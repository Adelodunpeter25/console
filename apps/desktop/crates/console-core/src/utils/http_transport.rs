use anyhow::{Context, Result};
use reqwest::header::{AUTHORIZATION, CONTENT_TYPE, HeaderMap, HeaderValue};
use serde::de::DeserializeOwned;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

#[derive(Clone)]
pub struct HttpTransport {
    base_url: Arc<RwLock<String>>,
    // One tunnel per node ID, kept so switching back to a server reuses its
    // live connection instead of building a new endpoint.
    tunnels: Arc<RwLock<HashMap<String, Arc<console_iroh::Tunnel>>>>,
    auth_token: Arc<RwLock<Option<String>>>,
    http: reqwest::Client,
}

impl HttpTransport {
    pub fn new(base_url: Option<String>) -> Self {
        Self::new_with_auth(base_url, None)
    }

    pub fn new_with_auth(base_url: Option<String>, auth_token: Option<String>) -> Self {
        let resolved_url = base_url
            .or_else(|| std::env::var("CONSOLE_BACKEND_URL").ok())
            .unwrap_or_else(|| "http://localhost:3000".to_string());

        Self {
            base_url: Arc::new(RwLock::new(resolved_url)),
            tunnels: Arc::new(RwLock::new(HashMap::new())),
            auth_token: Arc::new(RwLock::new(auth_token)),
            http: reqwest::Client::builder().build().unwrap(),
        }
    }

    /// Point the transport at a server. A `console-iroh://<node-id>` address
    /// is reached through a local tunnel: the base URL becomes the tunnel's
    /// loopback port, so HTTP and WebSocket callers need no changes. The
    /// tunnel is made without touching the network and iroh is dialled on the
    /// first request, so switching servers never waits on a connection. If the
    /// tunnel cannot be built the iroh address is kept, and requests fail
    /// with a clear connection error rather than reaching a stale server.
    pub async fn set_base_url(&self, url: impl Into<String>) {
        let url = url.into();
        let resolved = match console_shared::pairing::iroh_node_id(&url) {
            Some(node_id) => match self.tunnel_for(node_id).await {
                Ok(tunnel) => tunnel.base_url(),
                Err(err) => {
                    log::warn!("iroh tunnel to {node_id} failed: {err:#}");
                    url
                }
            },
            None => url,
        };
        *self.base_url.write().await = resolved;
    }

    async fn tunnel_for(&self, node_id: &str) -> Result<Arc<console_iroh::Tunnel>> {
        if let Some(tunnel) = self.tunnels.read().await.get(node_id) {
            return Ok(tunnel.clone());
        }
        let tunnel = Arc::new(console_iroh::Tunnel::new(node_id).await?);
        self.tunnels
            .write()
            .await
            .insert(node_id.to_string(), tunnel.clone());
        Ok(tunnel)
    }

    pub async fn base_url(&self) -> String {
        self.base_url.read().await.clone()
    }

    pub async fn set_auth_token(&self, token: Option<String>) {
        let mut auth = self.auth_token.write().await;
        *auth = token;
    }

    pub async fn auth_token(&self) -> Option<String> {
        self.auth_token.read().await.clone()
    }

    pub async fn build_headers(&self) -> HeaderMap {
        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
        if let Some(token) = self.auth_token.read().await.as_ref()
            && let Ok(val) = HeaderValue::from_str(&format!("Bearer {}", token))
        {
            headers.insert(AUTHORIZATION, val);
        }
        headers
    }

    /// Build a websocket handshake request carrying the same bearer token as
    /// HTTP requests. Sockets to `/api/*` are behind the auth middleware and a
    /// native client has no other way to present its credential.
    pub async fn ws_request(
        &self,
        url: &str,
    ) -> Result<tokio_tungstenite::tungstenite::handshake::client::Request> {
        use tokio_tungstenite::tungstenite::client::IntoClientRequest;
        let mut request = url.into_client_request()?;
        if let Some(token) = self.auth_token.read().await.as_ref()
            && !token.is_empty()
        {
            let value = HeaderValue::from_str(&format!("Bearer {}", token))
                .context("invalid auth token for websocket handshake")?;
            request.headers_mut().insert(AUTHORIZATION, value);
        }
        Ok(request)
    }

    pub async fn url(&self, path: &str) -> String {
        let base = self.base_url.read().await;
        format!("{}{}", base.trim_end_matches('/'), path)
    }

    pub fn client(&self) -> &reqwest::Client {
        &self.http
    }

    /// Decode a response without doing serde work on the UI executor.
    ///
    /// `reqwest::Response::json()` is lazy: its serde work runs wherever the
    /// future is polled. Desktop callers frequently poll service futures from
    /// GPUI's main executor, so the centralized path reads bytes first and
    /// moves the CPU-heavy decode to Tokio's blocking pool.
    pub async fn decode_json<T>(&self, response: reqwest::Response) -> Result<T>
    where
        T: DeserializeOwned + Send + 'static,
    {
        let bytes = response
            .bytes()
            .await
            .context("Failed to read response body")?;
        decode_json_bytes(bytes.to_vec()).await
    }
}

/// Decode already-buffered JSON on Tokio's blocking pool.
pub async fn decode_json_bytes<T>(bytes: Vec<u8>) -> Result<T>
where
    T: DeserializeOwned + Send + 'static,
{
    tokio::task::spawn_blocking(move || serde_json::from_slice::<T>(&bytes))
        .await
        .context("JSON decode worker failed")?
        .context("Failed to decode JSON response")
}

/// Probe an arbitrary backend URL for reachability — used by the settings
/// screen's environment editor before saving/activating a URL. A backend
/// answers `GET /api/projects`; any HTTP response within the timeout counts
/// as reachable (auth failures still prove the server is there).
pub async fn probe_backend(url: &str, timeout: std::time::Duration) -> Result<()> {
    let trimmed = url.trim_end_matches('/');
    anyhow::ensure!(!trimmed.is_empty(), "Backend URL is empty.");
    // An iroh address is probed through a short-lived tunnel.
    let _tunnel;
    let iroh_base;
    let trimmed = if let Some(node_id) = console_shared::pairing::iroh_node_id(trimmed) {
        _tunnel = console_iroh::Tunnel::connect(node_id)
            .await
            .context("Could not reach the server over iroh.")?;
        iroh_base = _tunnel.base_url();
        iroh_base.as_str()
    } else {
        trimmed
    };
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

/// Fetch raw bytes from a URL with a timeout.
pub async fn fetch_url_bytes(url: &str, timeout: std::time::Duration) -> Option<Vec<u8>> {
    let client = reqwest::Client::builder().timeout(timeout).build().ok()?;
    let response = client.get(url).send().await.ok()?;
    if response.status().is_success() {
        response.bytes().await.ok().map(|b| b.to_vec())
    } else {
        None
    }
}
