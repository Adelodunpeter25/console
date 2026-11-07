//! Device pairing with the Console server, through console-core's
//! `ServerAuthService` — the desktop's `state/server_auth.rs` minus the
//! entity plumbing.
//!
//! Pairing is per server and the credential lives in the shared storage
//! directory (`console_shared::path::storage_directory`, the desktop's own
//! store), so a device paired once is authenticated for both apps. Every
//! bridge call builds its transport through `http::transport`, which reads
//! the stored token for the call's base URL — there is no second,
//! unauthenticated path to forget.

use console_core::utils::server_auth_store;
use console_core::services::{ServerAuthService, ServerDevice};
use console_shared::path::storage_directory;

use crate::native::http;

/// The stored credential for `base_url`, if the app paired with it before.
fn stored(base_url: &str) -> Option<server_auth_store::StoredServerAuth> {
    server_auth_store::load_for(&storage_directory(), base_url)
}

/// Whether a pairing credential is on disk for this server.
pub fn paired(base_url: &str) -> bool {
    stored(base_url).is_some()
}

/// Exchanges a one-time pairing grant (the code the server shows) for this
/// device's long-lived credential, persists it for the normalized server
/// URL, and returns the paired device and the server's identity as JSON.
pub async fn pair_with_code(
    host: &str,
    code: &str,
    device_name: &str,
) -> Result<String, String> {
    // The pairing call itself carries no token, so an unauthenticated
    // transport pointed anywhere is enough; the service normalizes the host
    // and answers with the server it paired against.
    let service = ServerAuthService::new(http::transport(host));
    let (data, base_url) = service
        .pair_with_code(host, code, device_name)
        .await
        .map_err(|err| err.to_string())?;
    let credential = server_auth_store::StoredServerAuth {
        token: data.token.clone(),
        device_id: data.device.as_ref().map(|d| d.id.clone()).unwrap_or_default(),
        device_name: data
            .device
            .as_ref()
            .map(|d| d.name.clone())
            .unwrap_or_else(|| device_name.to_string()),
        expires_at: data.expires_at,
    };
    server_auth_store::save(&storage_directory(), &base_url, credential)
        .map_err(|err| format!("failed to store credential: {err}"))?;
    serde_json::to_string(&serde_json::json!({
        "baseUrl": base_url,
        "device": data.device.as_ref().map(|d| serde_json::json!({
            "id": d.id, "name": d.name, "platform": d.platform,
        })),
        "server": data.server.as_ref().map(|s| serde_json::json!({
            "id": s.id, "name": s.name,
        })),
    }))
    .map_err(|err| err.to_string())
}

/// Forgets this server's credential (the desktop's unpair).
pub fn unpair(base_url: &str) -> Result<(), String> {
    server_auth_store::clear(&storage_directory(), base_url)
        .map_err(|err| format!("failed to clear credential: {err}"))
}

/// The devices paired with this server (an authenticated read; rejects
/// without a stored token, like the desktop's settings page does).
pub async fn devices(base_url: &str) -> Result<Vec<ServerDevice>, String> {
    if !paired(base_url) {
        return Err("not paired with this server".to_string());
    }
    let service = ServerAuthService::new(http::transport(base_url));
    service.devices().await.map_err(|err| err.to_string())
}
