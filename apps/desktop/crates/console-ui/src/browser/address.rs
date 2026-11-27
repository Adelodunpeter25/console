//! Address bar input resolution, display URL formatting, and search routing.

use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::{Duration, Instant};

#[derive(Default)]
struct FaviconRegistry {
    cache: HashMap<String, Arc<gpui::Image>>,
    in_flight: HashSet<String>,
    failed: HashSet<String>,
}

static FAVICONS: OnceLock<Mutex<FaviconRegistry>> = OnceLock::new();

fn favicons() -> &'static Mutex<FaviconRegistry> {
    FAVICONS.get_or_init(Default::default)
}

/// How long a URL must stay on screen before anything is fetched for it. A URL
/// still streaming in changes every few milliseconds, so this keeps every
/// partial prefix from becoming a request.
const SETTLE_DELAY: Duration = Duration::from_millis(400);

static FIRST_SEEN: OnceLock<Mutex<HashMap<String, Instant>>> = OnceLock::new();

/// True once `key` has been asked about for at least [`SETTLE_DELAY`].
fn settled(key: &str) -> bool {
    let Ok(mut seen) = FIRST_SEEN.get_or_init(|| Mutex::new(HashMap::new())).lock() else {
        return false;
    };
    seen.entry(key.to_owned())
        .or_insert_with(Instant::now)
        .elapsed()
        >= SETTLE_DELAY
}

#[derive(Default)]
struct TitleRegistry {
    cache: HashMap<String, String>,
    in_flight: HashSet<String>,
    failed: HashSet<String>,
}

static TITLES: OnceLock<Mutex<TitleRegistry>> = OnceLock::new();

fn titles() -> &'static Mutex<TitleRegistry> {
    TITLES.get_or_init(Default::default)
}

static TITLE_GENERATION: AtomicUsize = AtomicUsize::new(0);

/// Bumps whenever a title arrives, so cached text built before it can be
/// rebuilt to show it.
pub fn page_title_generation() -> usize {
    TITLE_GENERATION.load(Ordering::Relaxed)
}

/// Where a page-title lookup stands.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum TitleState {
    Ready(String),
    /// Waiting to settle, or fetching; ask again on a later frame.
    Loading,
    /// No title will arrive; keep showing the URL.
    Unavailable,
}

/// The title of a public web page, fetched in the background on first ask.
pub fn page_title_state(url: &str) -> TitleState {
    if favicon_host(url).is_none() {
        return TitleState::Unavailable;
    }
    let Ok(mut reg) = titles().lock() else {
        return TitleState::Unavailable;
    };
    if let Some(title) = reg.cache.get(url) {
        return TitleState::Ready(title.clone());
    }
    if reg.failed.contains(url) {
        return TitleState::Unavailable;
    }
    if !settled(url) {
        return TitleState::Loading;
    }
    if reg.in_flight.insert(url.to_owned()) {
        drop(reg);
        let url = url.to_owned();
        tokio::spawn(async move {
            let title = console_core::fetch_page_title(&url, Duration::from_secs(5)).await;
            if let Ok(mut reg) = titles().lock() {
                reg.in_flight.remove(&url);
                match title {
                    Some(title) => {
                        reg.cache.insert(url, title);
                        TITLE_GENERATION.fetch_add(1, Ordering::Relaxed);
                    }
                    None => {
                        reg.failed.insert(url);
                    }
                }
            }
        });
    }
    TitleState::Loading
}

/// Where a favicon lookup stands.
#[derive(Clone)]
pub enum FaviconState {
    Ready(Arc<gpui::Image>),
    /// A fetch is running; ask again on a later frame.
    Loading,
    /// Local host, or the fetch failed. Callers show a fallback glyph.
    Unavailable,
}

/// The host worth fetching a favicon for, or `None` for local targets.
pub fn favicon_host(url_or_host: &str) -> Option<String> {
    let host = url_host(url_or_host);
    if host.is_empty()
        || host.eq_ignore_ascii_case("localhost")
        || host.starts_with("127.")
        || host.starts_with("0.0.0.0")
    {
        return None;
    }
    Some(host.to_string())
}

/// Retrieve a cached favicon `gpui::Image` for the given host or URL.
/// If not yet cached, initiates an asynchronous background fetch and returns `None`
/// until the image is decoded and ready in memory.
pub fn get_or_fetch_favicon(url_or_host: &str) -> Option<Arc<gpui::Image>> {
    match favicon_state(url_or_host) {
        FaviconState::Ready(image) => Some(image),
        _ => None,
    }
}

/// Like [`get_or_fetch_favicon`], but says whether an icon is still coming or
/// will never arrive, so a caller can stop asking for repaints.
pub fn favicon_state(url_or_host: &str) -> FaviconState {
    let Some(host) = favicon_host(url_or_host) else {
        return FaviconState::Unavailable;
    };

    let Ok(mut reg) = favicons().lock() else {
        return FaviconState::Unavailable;
    };

    if let Some(img) = reg.cache.get(&host) {
        return FaviconState::Ready(img.clone());
    }
    if reg.failed.contains(&host) {
        return FaviconState::Unavailable;
    }

    if !settled(&host) {
        return FaviconState::Loading;
    }

    if reg.in_flight.insert(host.clone()) {
        drop(reg);
        let host_for_task = host.clone();
        let fav_url = format!(
            "https://t1.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=http://{}&size=32",
            host
        );
        tokio::spawn(async move {
            let mut decoded_image = None;
            if let Some(bytes_vec) =
                console_core::fetch_url_bytes(&fav_url, std::time::Duration::from_secs(4)).await
            {
                let format = if bytes_vec.starts_with(&[0x89, 0x50, 0x4E, 0x47]) {
                    Some(gpui::ImageFormat::Png)
                } else if bytes_vec.starts_with(&[0xFF, 0xD8, 0xFF]) {
                    Some(gpui::ImageFormat::Jpeg)
                } else if bytes_vec.starts_with(&[0x00, 0x00, 0x01, 0x00]) {
                    Some(gpui::ImageFormat::Ico)
                } else if bytes_vec.starts_with(b"RIFF")
                    && bytes_vec.len() > 12
                    && &bytes_vec[8..12] == b"WEBP"
                {
                    Some(gpui::ImageFormat::Webp)
                } else {
                    gpui::ImageFormat::from_mime_type("image/png")
                };
                if let Some(format) = format {
                    decoded_image = Some(Arc::new(gpui::Image::from_bytes(format, bytes_vec)));
                }
            }
            if let Ok(mut reg) = favicons().lock() {
                reg.in_flight.remove(&host_for_task);
                if let Some(image) = decoded_image {
                    reg.cache.insert(host_for_task, image);
                } else {
                    reg.failed.insert(host_for_task);
                }
            }
        });
    }

    FaviconState::Loading
}

/// What the address input resolves to when the user submits it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum AddressTarget {
    Url(String),
    Search(String),
}

/// Safari-style omnibox resolution: explicit schemes pass through, host-like
/// text gets a scheme guessed for it, anything else becomes a web search.
pub fn resolve_address(raw: &str) -> Option<AddressTarget> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return None;
    }

    let has_scheme = trimmed.split_once(':').is_some_and(|(scheme, rest)| {
        !scheme.is_empty()
            && scheme
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '+' | '-' | '.'))
            && scheme
                .chars()
                .next()
                .is_some_and(|c| c.is_ascii_alphabetic())
            && (rest.starts_with("//") || matches!(scheme, "about" | "data" | "mailto" | "file"))
    });
    if has_scheme {
        return Some(AddressTarget::Url(trimmed.to_owned()));
    }

    if trimmed.contains(char::is_whitespace) {
        return Some(AddressTarget::Search(trimmed.to_owned()));
    }

    let authority = trimmed.split(['/', '?', '#']).next().unwrap_or(trimmed);
    let (host, port) = match authority.rsplit_once(':') {
        Some((host, port)) if !port.is_empty() && port.chars().all(|c| c.is_ascii_digit()) => {
            (host, true)
        }
        Some(_) => return Some(AddressTarget::Search(trimmed.to_owned())),
        None => (authority, false),
    };
    let is_ip = !host.is_empty()
        && host.chars().all(|c| c.is_ascii_digit() || c == '.')
        && host.split('.').count() == 4;
    let is_local = host.eq_ignore_ascii_case("localhost") || is_ip;
    let host_like = is_local
        || (host.contains('.')
            && !host.starts_with('.')
            && !host.ends_with('.')
            && host
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-')));

    if !host_like {
        return Some(AddressTarget::Search(trimmed.to_owned()));
    }
    // Dev servers rarely speak TLS; the public web rarely speaks anything else.
    let scheme = if is_local || (port && host.eq_ignore_ascii_case("localhost")) {
        "http"
    } else {
        "https"
    };
    Some(AddressTarget::Url(format!("{scheme}://{trimmed}")))
}

/// URL-encode a query string and build a search engine query URL.
pub fn search_url(query: &str) -> String {
    let mut encoded = String::with_capacity(query.len() * 3);
    for byte in query.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'.' | b'_' | b'~' => {
                encoded.push(byte as char)
            }
            b' ' => encoded.push('+'),
            _ => encoded.push_str(&format!("%{byte:02X}")),
        }
    }
    format!("https://www.google.com/search?q={encoded}")
}

/// Whether the given URL uses HTTPS (TLS encryption).
pub fn is_secure_url(url: &str) -> bool {
    url.starts_with("https://")
}

/// Host (domain) portion of a URL, without scheme, port, path, or query.
/// Used for tab titles and the Google favicon lookup.
pub fn url_host(url: &str) -> &str {
    let without_scheme = url.split_once("://").map(|(_, rest)| rest).unwrap_or(url);
    let authority = without_scheme
        .split(['/', '?', '#'])
        .next()
        .unwrap_or(without_scheme);
    // Strip userinfo and port.
    let host = authority.rsplit('@').next().unwrap_or(authority);
    host.rsplit_once(':')
        .map(|(h, _)| h)
        .unwrap_or(host)
        .trim_start_matches('[')
        .trim_end_matches(']')
}

/// Google favicon service URL for the given page URL (size 32, with fallback).
/// Uses the direct gstatic endpoint so clients receive HTTP 200 PNG bytes directly without redirects.
pub fn favicon_url(page_url: &str, size: u32) -> String {
    let host = url_host(page_url);
    format!(
        "https://t1.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=http://{}&size={}",
        host, size
    )
}

/// Default tab title for a URL: host + port (e.g. `localhost:3000`).
pub fn default_browser_title(url: &str) -> String {
    let without_scheme = url.split_once("://").map(|(_, rest)| rest).unwrap_or(url);
    let title = without_scheme
        .split(['/', '?', '#'])
        .next()
        .unwrap_or(without_scheme);
    if title.is_empty() {
        url.to_owned()
    } else {
        title.to_owned()
    }
}

/// The address bar hides `https://` the way Safari does; everything else —
/// including `http://` — stays visible because it is information.
pub fn display_url(url: &str) -> &str {
    url.strip_prefix("https://").unwrap_or(url)
}
