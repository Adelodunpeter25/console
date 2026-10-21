//! Engine-agnostic page driver.
//!
//! The driver is plain JavaScript injected at document start into whichever
//! webview is running (WKWebView today, Chromium via the in-page fallback). It
//! owns the page model: the accessibility snapshot, per-document refs, and the
//! actions that operate on those refs. Keeping it out of the host means the
//! same code runs on every engine, and the snapshot format stays the shared
//! contract between the driver and the agent.
//!
//! Installation and calls are separate on purpose: the install script is a
//! `WKUserScript`/CEF init script, so a call is a plain function invocation.
//! That avoids `eval`, which strict Content-Security-Policy pages block.

use serde_json::json;

const INSTALL: &str = include_str!("driver.js");

const CALL: &str = r#"(function () {
  var api = window.__consoleDriverApi;
  if (!api) {
    try {
      window.ipc.postMessage(JSON.stringify({
        type: 'script_result',
        requestId: __REQUEST_ID__,
        ok: false,
        error: 'The page driver is not installed yet. Wait for the page to finish loading, then try again.'
      }));
    } catch (_) {}
    return;
  }
  api.run(__REQUEST_ID__, __REQUEST__);
})()"#;

/// Script that installs the driver in a document. Injected at document start
/// (WKWebView) or load end (CEF), once per document.
pub fn init_script() -> &'static str {
    INSTALL
}

/// Build one driver call. `request_id` ties the IPC result back to the caller;
/// the driver posts the same `script_result` shape as the agent script wrapper.
pub fn request_script(
    request_id: &str,
    op: &str,
    element_ref: Option<&str>,
    text: Option<&str>,
    submit: bool,
    selector: Option<&str>,
) -> String {
    let request = json!({
        "op": op,
        "ref": element_ref.map(str::trim).unwrap_or(""),
        "text": text.unwrap_or(""),
        "submit": submit,
        "selector": selector.map(str::trim).unwrap_or(""),
    });
    let request_id = serde_json::to_string(request_id).unwrap_or_else(|_| "\"\"".to_string());
    CALL.replace("__REQUEST_ID__", &request_id)
        .replace("__REQUEST__", &request.to_string())
}
