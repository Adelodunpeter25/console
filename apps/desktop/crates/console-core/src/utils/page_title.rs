//! Resolve the human title of a web page, for showing in place of a bare URL.
//!
//! The pure extraction is shared with Android (`console-shared`); only the
//! network fetch lives here.

use std::time::Duration;

pub use console_shared::page_title::{MAX_BODY_BYTES, extract_html_title};

/// Fetch `url` and return its title. Reads only the start of the body and
/// skips anything that is not HTML.
pub async fn fetch_page_title(url: &str, timeout: Duration) -> Option<String> {
    let client = reqwest::Client::builder()
        .timeout(timeout)
        .user_agent("Mozilla/5.0 (compatible; Console/1.0)")
        .build()
        .ok()?;
    let mut response = client.get(url).send().await.ok()?;
    if !response.status().is_success() {
        return None;
    }
    let is_html = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .is_none_or(|value| value.to_ascii_lowercase().contains("html"));
    if !is_html {
        return None;
    }
    let mut body = Vec::new();
    while body.len() < MAX_BODY_BYTES {
        let Some(chunk) = response.chunk().await.ok()? else {
            break;
        };
        body.extend_from_slice(&chunk);
        if String::from_utf8_lossy(&body)
            .to_ascii_lowercase()
            .contains("</title")
        {
            break;
        }
    }
    extract_html_title(&String::from_utf8_lossy(&body))
}
