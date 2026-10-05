//! Chromium (CEF) browser backend.
//!
//! Gated behind the `cef-browser` cargo feature so default builds keep using
//! wry/WKWebView with zero CEF download or link cost.

#[cfg(target_os = "macos")]
pub mod client;
#[cfg(target_os = "macos")]
pub mod host;
#[cfg(target_os = "macos")]
pub mod runtime;
