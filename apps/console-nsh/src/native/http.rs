//! The transport wiring: one place builds an `HttpTransport` from a base URL.

use std::time::Duration;

use console_core::HttpTransport;

/// A transport pointed at the backend.
pub fn transport(base_url: &str) -> HttpTransport {
    HttpTransport::new(Some(base_url.to_owned()))
}

/// Fetches `{base_url}/health` and returns the response body, or rejects
/// with the reason when the server is unreachable (a timeout keeps the TSX
/// promise from pending forever).
pub async fn health(base_url: &str) -> Result<String, String> {
    let transport = transport(base_url);
    let url = transport.url("/health").await;
    match tokio::time::timeout(Duration::from_secs(5), transport.client().get(url).send()).await {
        Ok(Ok(response)) => response
            .text()
            .await
            .map_err(|err| format!("bad response: {err}")),
        Ok(Err(err)) => Err(format!("unreachable: {err}")),
        Err(_) => Err("unreachable: timed out".to_owned()),
    }
}
