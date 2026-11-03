//! The built-in browser's in-process history source and cookie injection.
//!
//! History: the app loads the persisted store at startup and after import;
//! the browser view reads suggestions from here at render, so no history
//! state needs to be plumbed through panes and tabs.
//!
//! Cookies: WKWebView cookie injection through `WKHTTPCookieStore`. Console
//! never writes decrypted cookie values to disk — they arrive from the
//! import route and cross straight into the cookie store.

use std::sync::{Arc, LazyLock, RwLock};

use console_core::browser_history::{BrowserHistory, BrowserHistoryEntry};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct HistorySuggestion {
    pub url: String,
    pub title: String,
}

pub static HISTORY: LazyLock<RwLock<Arc<BrowserHistory>>> =
    LazyLock::new(|| RwLock::new(Arc::new(BrowserHistory::default())));

/// Replace the in-memory copy (startup load, after an import).
pub fn init_history(history: BrowserHistory) {
    if let Ok(mut slot) = HISTORY.write() {
        *slot = Arc::new(history);
    }
}

/// Re-read the persisted store from disk.
pub fn reload_from_disk() {
    init_history(console_core::browser_history::load());
}

pub fn history_len() -> usize {
    HISTORY.read().map(|h| h.len()).unwrap_or(0)
}

/// Record a title-only update for an existing entry (no count change).
pub fn record_title(url: &str, title: &str) {
    let loaded = HISTORY.read().ok().map(|h| h.as_ref().clone());
    let Some(mut history) = loaded else { return };
    history.set_title(url, title);
    console_core::browser_history::save(&history);
    if let Ok(mut slot) = HISTORY.write() {
        *slot = Arc::new(history);
    }
}

/// Top omnibox suggestions for a query.
pub fn suggest(query: &str, limit: usize) -> Vec<HistorySuggestion> {
    let loaded = HISTORY.read().ok();
    loaded
        .map(|h| h.suggest(query, limit))
        .unwrap_or_default()
        .into_iter()
        .map(|entry| HistorySuggestion {
            url: entry.url,
            title: entry.title,
        })
        .collect()
}

/// Record one visit: merged in memory and persisted. Cheap enough to call
/// per navigation.
pub fn record_visit(url: &str, title: &str) {
    let loaded = HISTORY.read().ok().map(|h| h.as_ref().clone());
    let Some(mut history) = loaded else { return };
    history.record_visit(url, title);
    console_core::browser_history::save(&history);
    if let Ok(mut slot) = HISTORY.write() {
        *slot = Arc::new(history);
    }
}

/// Merge imported Chrome history entries into the store, then reload.
pub fn merge_history(entries: &[BrowserHistoryEntry]) {
    let mut merged = console_core::browser_history::load();
    merged.merge_chrome(entries);
    console_core::browser_history::save(&merged);
    init_history(merged);
}

// --- Cookie injection ------------------------------------------------------

#[cfg(target_os = "macos")]
pub fn inject_cookies(cookies: &[console_core::types::ChromeCookie]) -> Result<usize, String> {
    cookies::inject_cookies(cookies)
}

#[cfg(not(target_os = "macos"))]
pub fn inject_cookies(_cookies: &[console_core::types::ChromeCookie]) -> Result<usize, String> {
    Err("Chrome cookie import is supported on macOS".to_string())
}

#[cfg(target_os = "macos")]
pub mod cookies {
    use console_core::types::ChromeCookie;
    use objc2::rc::Retained;
    use objc2::runtime::AnyObject;
    use objc2_foundation::MainThreadMarker;
    use objc2_foundation::ns_string;
    use objc2_foundation::{NSDate, NSDictionary, NSHTTPCookie, NSString};
    use objc2_web_kit::WKWebsiteDataStore;

    /// Inject cookies into the shared default webview data store — the same
    /// store every `WebviewHost` was built against (wry without an explicit
    /// store uses `defaultDataStore`). Main thread only. Returns how many
    /// cookies went to the store ("injected" ≠ "accepted": acceptance is
    /// verified by the settled read-back log that follows).
    pub fn inject_cookies(input: &[ChromeCookie]) -> Result<usize, String> {
        use block2::RcBlock;

        let Some(mtm) = MainThreadMarker::new() else {
            return Err("cookie injection must run on the main thread".to_string());
        };
        log::info!("cookie inject: input={} cookies", input.len());
        let mut injected = 0usize;
        let mut skipped = 0usize;
        unsafe {
            let store = WKWebsiteDataStore::defaultDataStore(mtm).httpCookieStore();
            // One synthetic control cookie, independent of Chrome's data: if
            // even this never lands, the failure is in how these cookies are
            // built or set, not in the imported values.
            let probe = ChromeCookie {
                domain: ".console-cookie-probe.invalid".to_string(),
                path: "/".to_string(),
                name: "probe".to_string(),
                value: "1".to_string(),
                expires_at_ms: (now_unix_ms_placeholder() + 3_600_000) as i64,
                secure: false,
                http_only: false,
            };
            let mut all: Vec<ChromeCookie> = vec![probe];
            all.extend(input.iter().cloned());
            let mut first_back = true;
            for cookie in &all {
                let Some(cookie_obj) = build_cookie(cookie) else {
                    skipped += 1;
                    log::warn!(
                        "cookie build skipped: name_empty={} value_empty={} domain_empty={}",
                        cookie.name.is_empty(),
                        cookie.value.is_empty(),
                        cookie.domain.is_empty()
                    );
                    continue;
                };
                if first_back {
                    first_back = false;
                    // What WebKit parsed back from the properties we set —
                    // empty fields here mean the property dictionary is
                    // being ignored/misread. Values never logged.
                    log::info!(
                        "cookie build check: name={} domain={} path={} secure={} http_only={} expires={:?}",
                        cookie_obj.name().to_string(),
                        cookie_obj.domain().to_string(),
                        cookie_obj.path().to_string(),
                        cookie_obj.isSecure(),
                        cookie_obj.isHTTPOnly(),
                        cookie_obj.expiresDate().map(|d| d.timeIntervalSince1970() as i64),
                    );
                }
                let name = cookie.name.clone();
                let domain = cookie.domain.clone();
                // The handler firing is the acceptance signal; a rejected
                // cookie stays silent here, and the settled read-back that
                // the caller schedules counts what actually landed. Never
                // log values.
                let handler = RcBlock::new(move || {
                    log::debug!("chrome cookies: accepted {}.{}", domain, name);
                });
                store.setCookie_completionHandler(&cookie_obj, Some(&handler));
                injected += 1;
            }
        }
        log::info!("cookie inject: handed {injected} to WebKit, skipped {skipped}");
        Ok(injected)
    }

    fn now_unix_ms_placeholder() -> u64 {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_millis() as u64)
            .unwrap_or(0)
    }

    /// Read the shared cookie jar back and log its size plus the top
    /// domains. Callers delay this a couple of seconds after injection so
    /// the async setCookie calls have propagated; a count far below the
    /// injected total means WebKit rejected the cookies, and the domain
    /// split shows which survived.
    pub fn log_cookie_jar() {
        use block2::RcBlock;
        use std::ptr::NonNull;
        use objc2_foundation::NSArray;

        let Some(mtm) = MainThreadMarker::new() else {
            return;
        };
        unsafe {
            let store = WKWebsiteDataStore::defaultDataStore(mtm).httpCookieStore();
            let read = RcBlock::new(|cookies: NonNull<NSArray<NSHTTPCookie>>| {
                let cookies = cookies.as_ref();
                let total = cookies.count();
                let mut by_domain: std::collections::HashMap<String, usize> = Default::default();
                for cookie in cookies.iter() {
                    *by_domain.entry(cookie.domain().to_string()).or_insert(0) += 1;
                }
                let mut top: Vec<(String, usize)> = by_domain.into_iter().collect();
                top.sort_by(|a, b| b.1.cmp(&a.1).then(a.0.cmp(&b.0)));
                let top: Vec<String> =
                    top.iter().take(8).map(|(d, n)| format!("{d}x{n}")).collect();
                log::info!("chrome cookies: jar holds {total} cookies; top: {}", top.join(", "));
            });
            store.getAllCookies(&read);
        }
    }

    /// Foundation has no documented property key for HttpOnly; the private
    ///-by-convention "NSHTTPCookieIsHttpOnly" key is what WebKit and
    /// Chromium both honor, so the flag survives for every consumer.
    fn build_cookie(cookie: &ChromeCookie) -> Option<Retained<NSHTTPCookie>> {
        if cookie.name.is_empty() || cookie.value.is_empty() || cookie.domain.is_empty() {
            return None;
        }

        let mut keys: Vec<&NSString> = vec![
            ns_string!("NSHTTPCookieName"),
            ns_string!("NSHTTPCookieValue"),
            ns_string!("NSHTTPCookieDomain"),
            ns_string!("NSHTTPCookiePath"),
        ];
        let name = NSString::from_str(&cookie.name);
        let value = NSString::from_str(&cookie.value);
        let domain = NSString::from_str(&cookie.domain);
        let path = NSString::from_str(if cookie.path.is_empty() { "/" } else { cookie.path.as_str() });
        let mut objects: Vec<Retained<AnyObject>> = vec![
            // SAFETY: NSString/NSDate widen to AnyObject, no data moved.
            unsafe { into_any(name.clone()) },
            unsafe { into_any(value.clone()) },
            unsafe { into_any(domain.clone()) },
            unsafe { into_any(path.clone()) },
        ];

        if cookie.secure {
            keys.push(ns_string!("NSHTTPCookieSecure"));
            objects.push(truth());
        }
        if cookie.http_only {
            keys.push(ns_string!("NSHTTPCookieIsHttpOnly"));
            objects.push(truth());
        }
        if cookie.expires_at_ms > 0 {
            // NSDate wants seconds; the ms → s conversion keeps the same
            // instant, and zero stays a session cookie.
            let date = NSDate::dateWithTimeIntervalSince1970(cookie.expires_at_ms as f64 / 1_000.0);
            keys.push(ns_string!("NSHTTPCookieExpires"));
            objects.push(unsafe { into_any(date) });
        }
        let refs: Vec<&AnyObject> = objects.iter().map(|o| &**o).collect();

        let properties = NSDictionary::<NSString, AnyObject>::from_slices(&keys, &refs);
        unsafe { NSHTTPCookie::cookieWithProperties(&properties) }.into()
    }

    /// Bridge a typed Foundation object to the dictionary's `AnyObject`
    /// object type. Retains none: ownership moves.
    unsafe fn into_any<T: objc2::ClassType>(value: Retained<T>) -> Retained<AnyObject> {
        unsafe { Retained::cast_unchecked::<AnyObject>(value) }
    }

    fn truth() -> Retained<AnyObject> {
        // SAFETY: NSString is a subclass of NSObject; the AnyObject view is
        // the same object, nothing stored changes.
        unsafe { into_any(NSString::from_str("TRUE")) }
    }
}
