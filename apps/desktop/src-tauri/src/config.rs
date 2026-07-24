use std::sync::RwLock;

use once_cell::sync::Lazy;

static SERVER_URL: Lazy<RwLock<String>> = Lazy::new(|| RwLock::new("http://localhost:3000".to_string()));

pub fn get_server_url() -> String {
    SERVER_URL.read().unwrap().clone()
}

pub fn set_server_url(url: &str) {
    let normalized = url.trim_end_matches('/').to_string();
    *SERVER_URL.write().unwrap() = normalized;
}

pub fn api_base() -> String {
    format!("{}/api", get_server_url())
}

pub fn health_url() -> String {
    format!("{}/health", get_server_url())
}
