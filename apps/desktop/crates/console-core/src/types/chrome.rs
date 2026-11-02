//! Chrome import wire types (the thin client side of the server service).

use serde::{Deserialize, Serialize};

/// One Chrome profile folder plus its human-readable name.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct ChromeProfile {
    pub id: String,
    pub name: String,
}

impl Default for ChromeProfile {
    fn default() -> Self {
        Self {
            id: String::new(),
            name: String::new(),
        }
    }
}

/// One decrypted cookie from the server, ready for webview injection.
/// Values never touch disk here.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct ChromeCookie {
    pub domain: String,
    pub path: String,
    pub name: String,
    pub value: String,
    /// Unix ms, or 0 for a session cookie.
    pub expires_at_ms: i64,
    pub secure: bool,
    pub http_only: bool,
}

impl Default for ChromeCookie {
    fn default() -> Self {
        Self {
            domain: String::new(),
            path: String::new(),
            name: String::new(),
            value: String::new(),
            expires_at_ms: 0,
            secure: false,
            http_only: false,
        }
    }
}
