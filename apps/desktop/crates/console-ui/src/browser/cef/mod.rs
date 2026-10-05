//! Chromium (CEF) browser backend.
//!
//! Gated behind the `cef-browser` cargo feature so default builds keep using
//! wry/WKWebView with zero CEF download or link cost. This slice only
//! provides process lifecycle and runtime init/pump/shutdown; no browser is
//! created yet (that is Slice 3).

#[cfg(target_os = "macos")]
pub mod runtime;
