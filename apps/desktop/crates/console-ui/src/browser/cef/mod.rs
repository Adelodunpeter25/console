//! Chromium (CEF) browser backend for the browser tab. The Devices tab does
//! not use it: it runs on the system WKWebView (see
//! `browser::host::SystemWebviewHost`) because stock CEF has no H.264.

pub mod agent_browser;
#[cfg(target_os = "macos")]
pub mod application;
#[cfg(target_os = "macos")]
pub mod client;
#[cfg(target_os = "macos")]
pub mod host;
#[cfg(target_os = "macos")]
pub mod runtime;
