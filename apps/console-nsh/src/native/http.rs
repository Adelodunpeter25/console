//! The transport wiring: one place builds an `HttpTransport` from a base URL.
//!
//! Every transport here carries the paired device credential stored for
//! that server (the shared `server-auth.json` the desktop app writes too),
//! so a paired app authenticates every call and an unpaired one degrades to
//! the same unauthenticated requests the desktop makes before pairing.

use std::time::Duration;

use console_core::HttpTransport;
use console_core::utils::server_auth_store;
use console_shared::path::storage_directory;

/// A transport pointed at the backend, with the server's stored device
/// token when one is on disk.
pub fn transport(base_url: &str) -> HttpTransport {
    let token = server_auth_store::token_for(&storage_directory(), base_url);
    HttpTransport::new_with_auth(Some(base_url.to_owned()), token)
}

/// A full `ConsoleClient` for the backend — the same shape the desktop's
/// state constructs, auth included.
pub fn client(base_url: &str) -> console_core::ConsoleClient {
    console_core::ConsoleClient::new_with_auth(
        Some(base_url.to_owned()),
        server_auth_store::token_for(&storage_directory(), base_url),
    )
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
