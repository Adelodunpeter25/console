//! Chrome import client: profiles and a one-shot history+cookies fetch.
//!
//! The heavy lifting lives on the server (SQLite reads + cookie decryption);
//! this service is the thin REST pair for it. Cookie values are held in
//! memory only — the UI injects them straight into the WKWebView cookie
//! store and keeps nothing on disk.

use crate::types::{ApiResponse, BrowserHistoryEntry, ChromeCookie, ChromeProfile};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};

#[derive(Clone)]
pub struct ChromeService {
    transport: HttpTransport,
}

#[derive(Debug, Serialize)]
struct ChromeImportBody {
    #[serde(rename = "profileId")]
    profile_id: String,
    #[serde(rename = "wantHistory")]
    want_history: bool,
    #[serde(rename = "wantCookies")]
    want_cookies: bool,
}

#[derive(Debug, Deserialize)]
pub struct ChromeImportData {
    #[serde(default)]
    pub history: Vec<BrowserHistoryEntry>,
    #[serde(default)]
    pub cookies: Vec<ChromeCookie>,
}

/// Older servers answer the import routes with a router 404; surface that
/// as "update your server" instead of a decode failure.
fn is_unsupported_route(status: reqwest::StatusCode) -> bool {
    status == reqwest::StatusCode::NOT_FOUND || status == reqwest::StatusCode::NOT_IMPLEMENTED
}

impl ChromeService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// Chrome profiles with data on the server's machine. Empty when the
    /// server has none (Chrome missing, or not a Mac).
    pub async fn profiles(&self) -> Result<Vec<ChromeProfile>> {
        let url = self.transport.url("/api/chrome/profiles").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list Chrome profiles")?;
        let status = resp.status();
        if is_unsupported_route(status) {
            anyhow::bail!("This server does not support Chrome import yet — update it (console upgrade).");
        }
        let body: ApiResponse<Vec<ChromeProfile>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse Chrome profiles response")?;
        log::info!(
            "chrome profiles: status={status} success={} count={}",
            body.success,
            body.data.as_ref().map(Vec::len).unwrap_or(0)
        );
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to list Chrome profiles".into())
            ))
        }
    }

    /// Import a profile: most-visited history rows and decrypted, live
    /// cookies. 409 carries the keychain refusal (user denied access, or a
    /// headless daemon that cannot reach one).
    pub async fn import(
        &self,
        profile_id: &str,
        want_history: bool,
        want_cookies: bool,
    ) -> Result<ChromeImportData> {
        let url = self.transport.url("/api/chrome/import").await;
        log::info!(
            "chrome import: POST {url} profile={profile_id} want_history={want_history} want_cookies={want_cookies}"
        );
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&ChromeImportBody {
                profile_id: profile_id.to_string(),
                want_history,
                want_cookies,
            })
            .send()
            .await
            .context("Failed to import Chrome data")?;
        if is_unsupported_route(resp.status()) {
            anyhow::bail!("This server does not support Chrome import yet — update it (console upgrade).");
        }
        let status = resp.status();
        let body: ApiResponse<ChromeImportData> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse Chrome import response")?;
        log::info!(
            "chrome import: status={status} success={} history={} cookies={}",
            body.success,
            body.data.as_ref().map(|d| d.history.len()).unwrap_or(0),
            body.data.as_ref().map(|d| d.cookies.len()).unwrap_or(0)
        );
        if body.success {
            body.data.context("Chrome import data missing")
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Chrome import failed".into())
            ))
        }
    }
}
