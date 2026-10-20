//! Per-server device credentials on disk.
//!
//! Pairing is a property of a *server*, not of the app: the same install can
//! be paired to several Console servers, and a token issued by one must never
//! be sent to another. The store is therefore keyed by the server's
//! normalized base URL.
//!
//! The file holds a long-lived signed device token, so it is written with
//! mode 0600 and replaced atomically (write to a temp file, then rename).

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};

/// One server's paired device credential.
#[derive(Clone, Debug, Serialize, Deserialize, Default, PartialEq, Eq)]
pub struct StoredServerAuth {
    pub token: String,
    pub device_id: String,
    pub device_name: String,
    pub expires_at: i64,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
struct Document {
    #[serde(default)]
    servers: HashMap<String, StoredServerAuth>,
}

/// Normalize a server URL into a store key: trimmed, without a trailing
/// slash, lowercased. A trailing slash is a spelling difference, not a
/// different server.
fn normalize(url: &str) -> String {
    url.trim().trim_end_matches('/').to_lowercase()
}

fn file_in(directory: &Path) -> PathBuf {
    directory.join("server-auth.json")
}

fn read(directory: &Path) -> Document {
    fs::read(file_in(directory))
        .ok()
        .and_then(|bytes| serde_json::from_slice(&bytes).ok())
        .unwrap_or_default()
}

fn write(directory: &Path, document: &Document) -> std::io::Result<()> {
    fs::create_dir_all(directory)?;
    let target = file_in(directory);
    let temporary = target.with_extension(format!("json.{}.tmp", std::process::id()));
    let bytes = serde_json::to_vec(document).map_err(std::io::Error::other)?;
    fs::write(&temporary, bytes)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&temporary, fs::Permissions::from_mode(0o600))?;
    }
    fs::rename(temporary, target)
}

/// The credential stored for [server_url], if any.
pub fn load_for(directory: &Path, server_url: &str) -> Option<StoredServerAuth> {
    read(directory)
        .servers
        .get(&normalize(server_url))
        .cloned()
        .filter(|auth| !auth.token.is_empty())
}

/// Just the token, for the transport to install.
pub fn token_for(directory: &Path, server_url: &str) -> Option<String> {
    load_for(directory, server_url).map(|auth| auth.token)
}

/// Store a credential for one server, leaving other servers untouched.
pub fn save(directory: &Path, server_url: &str, auth: StoredServerAuth) -> std::io::Result<()> {
    let mut document = read(directory);
    document.servers.insert(normalize(server_url), auth);
    write(directory, &document)
}

/// Forget one server's credential.
pub fn clear(directory: &Path, server_url: &str) -> std::io::Result<()> {
    let mut document = read(directory);
    document.servers.remove(&normalize(server_url));
    write(directory, &document)
}
