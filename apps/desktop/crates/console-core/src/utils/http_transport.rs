use anyhow::{Context, Result};
use reqwest::header::{AUTHORIZATION, CONTENT_TYPE, HeaderMap, HeaderValue};
use std::sync::Arc;
use tokio::sync::RwLock;

#[derive(Clone)]
pub struct HttpTransport {
    base_url: Arc<RwLock<String>>,
    auth_token: Arc<RwLock<Option<String>>>,
    http: reqwest::Client,
}

impl HttpTransport {
    pub fn new(base_url: Option<String>) -> Self {
        let resolved_url = base_url
            .or_else(|| std::env::var("CONSOLE_BACKEND_URL").ok())
            .unwrap_or_else(|| "http://localhost:3000".to_string());

        Self {
            base_url: Arc::new(RwLock::new(resolved_url)),
            auth_token: Arc::new(RwLock::new(None)),
            http: reqwest::Client::builder().build().unwrap(),
        }
    }

    pub async fn set_base_url(&self, url: impl Into<String>) {
        let mut base = self.base_url.write().await;
        *base = url.into();
    }

    pub async fn base_url(&self) -> String {
        self.base_url.read().await.clone()
    }

    pub async fn set_auth_token(&self, token: Option<String>) {
        let mut auth = self.auth_token.write().await;
        *auth = token;
    }

    pub async fn build_headers(&self) -> HeaderMap {
        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
        if let Some(token) = self.auth_token.read().await.as_ref() {
            if let Ok(val) = HeaderValue::from_str(&format!("Bearer {}", token)) {
                headers.insert(AUTHORIZATION, val);
            }
        }
        headers
    }

    pub async fn url(&self, path: &str) -> String {
        let base = self.base_url.read().await;
        format!("{}{}", base.trim_end_matches('/'), path)
    }

    pub fn client(&self) -> &reqwest::Client {
        &self.http
    }
}

/// Probe an arbitrary backend URL for reachability — used by the settings
/// screen's environment editor before saving/activating a URL. A backend
/// answers `GET /api/projects`; any HTTP response within the timeout counts
/// as reachable (auth failures still prove the server is there).
pub async fn probe_backend(url: &str, timeout: std::time::Duration) -> Result<()> {
    let trimmed = url.trim_end_matches('/');
    anyhow::ensure!(!trimmed.is_empty(), "Backend URL is empty.");
    let base = if trimmed.starts_with("http://") || trimmed.starts_with("https://") {
        trimmed.to_string()
    } else {
        format!("http://{trimmed}")
    };

    let client = reqwest::Client::builder().build()?;
    let response = client
        .get(format!("{base}/api/projects"))
        .timeout(timeout)
        .send()
        .await
        .context("Could not reach the server. Is it running?")?;
    anyhow::ensure!(
        response.status().is_success() || response.status() == reqwest::StatusCode::UNAUTHORIZED,
        "Server responded with status {}",
        response.status()
    );
    Ok(())
}
