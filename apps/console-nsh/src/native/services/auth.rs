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

use std::fs;
use std::path::PathBuf;

use console_core::utils::server_auth_store;
use console_core::services::{ServerAuthService, ServerDevice};
use console_shared::path::storage_directory;

use crate::native::http;

fn state_file() -> PathBuf {
    storage_directory().join("state.json")
}

/// The stored credential for `base_url`, if the app paired with it before.
fn stored(base_url: &str) -> Option<server_auth_store::StoredServerAuth> {
    server_auth_store::load_for(&storage_directory(), base_url)
}

/// Whether a pairing credential is on disk for this server.
pub fn paired(base_url: &str) -> bool {
    stored(base_url).is_some()
}

/// Adds or updates the server as the active environment in `state.json`.
pub fn adopt_server(base_url: &str, server_name: Option<&str>) -> Result<(), String> {
    let normalized = base_url.trim().trim_end_matches('/').to_string();
    if normalized.is_empty() {
        return Ok(());
    }

    let path = state_file();
    let mut doc: serde_json::Value = fs::read(&path)
        .ok()
        .and_then(|bytes| serde_json::from_slice(&bytes).ok())
        .unwrap_or_else(|| serde_json::json!({ "version": 1 }));

    if !doc.is_object() {
        doc = serde_json::json!({ "version": 1 });
    }

    let envs_val = doc
        .as_object_mut()
        .map(|obj| obj.entry("environments").or_insert_with(|| serde_json::json!({})))
        .and_then(|v| v.as_object_mut());

    let mut target_id = None;
    if let Some(envs_obj) = envs_val {
        let env_list = envs_obj
            .entry("environments")
            .or_insert_with(|| serde_json::json!([]))
            .as_array_mut();

        if let Some(list) = env_list {
            for item in list.iter_mut() {
                if let Some(url) = item.get("url").and_then(|u| u.as_str()) {
                    if url.trim().trim_end_matches('/') == normalized {
                        if let Some(name) = server_name {
                            if !name.trim().is_empty() {
                                item["name"] = serde_json::json!(name);
                            }
                        }
                        if let Some(id) = item.get("id").and_then(|i| i.as_str()) {
                            target_id = Some(id.to_string());
                        }
                        break;
                    }
                }
            }

            if target_id.is_none() {
                let id = format!(
                    "env-{}",
                    std::time::SystemTime::now()
                        .duration_since(std::time::UNIX_EPOCH)
                        .unwrap_or_default()
                        .as_millis()
                );
                list.push(serde_json::json!({
                    "id": id,
                    "name": server_name.unwrap_or("Server"),
                    "url": normalized,
                }));
                target_id = Some(id);
            }
        }

        if let Some(id) = target_id {
            envs_obj.insert("active_id".to_string(), serde_json::json!(id));
        }
    }

    let directory = path.parent().unwrap_or_else(|| std::path::Path::new("."));
    fs::create_dir_all(directory).map_err(|e| e.to_string())?;
    let temp_path = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let bytes = serde_json::to_vec(&doc).map_err(|e| e.to_string())?;
    fs::write(&temp_path, bytes).map_err(|e| e.to_string())?;
    fs::rename(temp_path, path).map_err(|e| e.to_string())?;

    Ok(())
}

/// Resolves the active backend server URL from `CONSOLE_BACKEND_URL`,
/// `state.json`, or `server-auth.json`.
pub fn resolve_active_server_url() -> String {
    if let Ok(from_env) = std::env::var("CONSOLE_BACKEND_URL") {
        let trimmed = from_env.trim().trim_end_matches('/');
        if !trimmed.is_empty() {
            return trimmed.to_string();
        }
    }

    let dir = storage_directory();
    let state_path = dir.join("state.json");
    if let Ok(bytes) = fs::read(&state_path) {
        if let Ok(doc) = serde_json::from_slice::<serde_json::Value>(&bytes) {
            if let Some(envs) = doc.get("environments") {
                let active_id = envs.get("active_id").and_then(|v| v.as_str());
                let items = envs.get("environments").and_then(|v| v.as_array());

                if let Some(items) = items {
                    if let Some(active_id) = active_id {
                        if let Some(active_env) = items.iter().find(|e| e.get("id").and_then(|i| i.as_str()) == Some(active_id)) {
                            if let Some(url) = active_env.get("url").and_then(|u| u.as_str()) {
                                let trimmed = url.trim().trim_end_matches('/');
                                if trimmed.starts_with("http://") || trimmed.starts_with("https://") {
                                    return trimmed.to_string();
                                }
                            }
                        }
                    }

                    for item in items {
                        if let Some(url) = item.get("url").and_then(|u| u.as_str()) {
                            let trimmed = url.trim().trim_end_matches('/');
                            if (trimmed.starts_with("http://") || trimmed.starts_with("https://")) && paired(trimmed) {
                                return trimmed.to_string();
                            }
                        }
                    }
                }
            }
        }
    }

    let auth_path = dir.join("server-auth.json");
    if let Ok(bytes) = fs::read(&auth_path) {
        if let Ok(doc) = serde_json::from_slice::<serde_json::Value>(&bytes) {
            if let Some(servers) = doc.get("servers").and_then(|v| v.as_object()) {
                for key in servers.keys() {
                    let trimmed = key.trim().trim_end_matches('/');
                    if (trimmed.contains("127.0.0.1") || trimmed.contains("localhost")) && (trimmed.starts_with("http://") || trimmed.starts_with("https://")) {
                        return trimmed.to_string();
                    }
                }
                for key in servers.keys() {
                    let trimmed = key.trim().trim_end_matches('/');
                    if trimmed.starts_with("http://") || trimmed.starts_with("https://") {
                        return trimmed.to_string();
                    }
                }
            }
        }
    }

    "http://localhost:3000".to_string()
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

    let server_name = data.server.as_ref().map(|s| s.name.as_str());
    let _ = adopt_server(&base_url, server_name);

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

/// Splits one pasted pairing line into its host and code — the desktop's
/// settings form behaviour, where one line fills both fields. Accepts the
/// whole pairing URL (`http://host:3000/console/pair#v=1&server=..&code=…`),
/// `host:3000#code`, or a bare address (empty code).
pub fn split_pairing_line(text: &str) -> (String, String) {
    console_core::utils::split_pairing_line(text)
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
