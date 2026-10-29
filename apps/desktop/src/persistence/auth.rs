//! Per-server device credentials: the desktop app's own store.
//!
//! The storage *format* lives in `console-core` (`utils::server_auth_store`)
//! so it can be tested next to the rest of the client's wire types; this
//! module only supplies the directory the desktop app already uses for its
//! state and adapts the call sites to it.

use console_core::utils::server_auth_store;
pub use console_core::utils::server_auth_store::StoredServerAuth;
use std::path::PathBuf;

fn directory() -> PathBuf {
    super::store::storage_directory()
}

/// The credential stored for [server_url], if any.
pub fn load_for(server_url: &str) -> Option<StoredServerAuth> {
    server_auth_store::load_for(&directory(), server_url)
}

/// Just the token, for the transport to install.
pub fn token_for(server_url: &str) -> Option<String> {
    server_auth_store::token_for(&directory(), server_url)
}

/// Store a credential for one server, leaving other servers untouched.
pub fn save(server_url: &str, auth: StoredServerAuth) -> std::io::Result<()> {
    server_auth_store::save(&directory(), server_url, auth)
}

/// Forget one server's credential.
pub fn clear(server_url: &str) -> std::io::Result<()> {
    server_auth_store::clear(&directory(), server_url)
}
