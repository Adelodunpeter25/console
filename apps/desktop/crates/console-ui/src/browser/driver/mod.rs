//! Engine-agnostic page driver.
//!
//! The driver is plain JavaScript injected at document start into whichever
//! webview is running (WKWebView today, Chromium via the in-page fallback). It
//! owns the page model: the accessibility snapshot, per-document refs, the
//! actions that operate on those refs, plus script evaluation, structured
//! queries, content reading and wait probes. Keeping it out of the host means
//! the same code runs on every engine, and the snapshot format stays the
//! shared contract between the driver and the agent.
//!
//! Installation and calls are separate on purpose: the install script is a
//! `WKUserScript`/CEF init script, so a call is a plain function invocation.
//! That avoids `eval` for the page model; only `eval`/`run_js` itself needs it.

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

/// One driver request. Constructors keep the wire shape in one place so the
/// call sites stay readable as the action surface grows.
pub struct Request<'a> {
    pub op: &'a str,
    pub element_ref: Option<&'a str>,
    pub text: Option<&'a str>,
    pub submit: bool,
    pub selector: Option<&'a str>,
    pub script: Option<&'a str>,
    pub url_contains: Option<&'a str>,
}

impl<'a> Request<'a> {
    pub fn action(
        op: &'a str,
        element_ref: Option<&'a str>,
        text: Option<&'a str>,
        submit: bool,
        selector: Option<&'a str>,
    ) -> Self {
        Self {
            op,
            element_ref,
            text,
            submit,
            selector,
            script: None,
            url_contains: None,
        }
    }

    pub fn eval(script: &'a str) -> Self {
        Self {
            op: "eval",
            element_ref: None,
            text: None,
            submit: false,
            selector: None,
            script: Some(script),
            url_contains: None,
        }
    }

    pub fn content(selector: Option<&'a str>) -> Self {
        Self {
            op: "content",
            element_ref: None,
            text: None,
            submit: false,
            selector,
            script: None,
            url_contains: None,
        }
    }

    pub fn query(selector: &'a str) -> Self {
        Self {
            op: "query",
            element_ref: None,
            text: None,
            submit: false,
            selector: Some(selector),
            script: None,
            url_contains: None,
        }
    }

    pub fn wait(
        selector: Option<&'a str>,
        url_contains: Option<&'a str>,
        text: Option<&'a str>,
    ) -> Self {
        Self {
            op: "wait",
            element_ref: None,
            text,
            submit: false,
            selector,
            script: None,
            url_contains,
        }
    }
}

/// Script that installs the driver in a document. Injected at document start
/// (WKWebView) or load end (CEF), once per document.
pub fn init_script() -> &'static str {
    INSTALL
}

/// Build one driver call. `request_id` ties the IPC result back to the caller;
/// the driver posts the same `script_result` shape as the agent script wrapper.
pub fn request_script(request_id: &str, request: &Request<'_>) -> String {
    let body = json!({
        "op": request.op,
        "ref": request.element_ref.map(str::trim).unwrap_or(""),
        "text": request.text.unwrap_or(""),
        "submit": request.submit,
        "selector": request.selector.map(str::trim).unwrap_or(""),
        "script": request.script.unwrap_or(""),
        "urlContains": request.url_contains.unwrap_or(""),
    });
    let request_id = serde_json::to_string(request_id).unwrap_or_else(|_| "\"\"".to_string());
    CALL.replace("__REQUEST_ID__", &request_id)
        .replace("__REQUEST__", &body.to_string())
}
