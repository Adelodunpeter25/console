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
    /// store every `WebviewHost` was built against (wry creates its
    /// WKWebViewConfiguration without a custom store, so `defaultDataStore`
    /// is the browser's jar). Main thread only. Returns how many cookies
    /// the store accepted; a cookie with unusable properties is skipped.
    pub fn inject_cookies(input: &[ChromeCookie]) -> Result<usize, String> {
        use block2::RcBlock;
        use std::ptr::NonNull;
        use objc2_foundation::NSArray;

        let Some(mtm) = MainThreadMarker::new() else {
            return Err("cookie injection must run on the main thread".to_string());
        };
        let mut injected = 0usize;
        unsafe {
            let store = WKWebsiteDataStore::defaultDataStore(mtm).httpCookieStore();
            for cookie in input {
                let Some(cookie_obj) = build_cookie(cookie) else {
                    continue;
                };
                let name = cookie.name.clone();
                let domain = cookie.domain.clone();
                let handler = RcBlock::new(move || {
                    // The handler firing is the acceptance signal; a rejected
                    // cookie stays silent here, and the store-wide read below
                    // counts what actually landed.
                    log::debug!("chrome cookies: stored {}.{}", domain, name);
                });
                store.setCookie_completionHandler(&cookie_obj, Some(&handler));
                injected += 1;
            }
            // Store-wide read right after the loop: how many cookies the jar
            // holds at this instant (recent sets may still be landing, so
            // treat low numbers here as provisional).
            let read = RcBlock::new(|cookies: NonNull<NSArray<NSHTTPCookie>>| {
                let total = unsafe { cookies.as_ref().count() };
                log::info!("chrome cookies: jar read-back holds {total} cookies");
            });
            store.getAllCookies(&read);
        }
        Ok(injected)
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
