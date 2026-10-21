//! Tests for the engine-agnostic driver script builder.
//!
//! The driver itself runs in the page, so these tests pin the Rust side of the
//! contract: the install script exists, a call embeds its request, user text
//! cannot break out of the call, and the error strings the agent relies on
//! stay present.

use console_ui::browser::driver::{init_script, request_script};

#[test]
fn install_script_exposes_the_driver_api_and_has_no_request_placeholder() {
    let js = init_script();
    assert!(js.contains("window.__consoleDriverApi = { run }"));
    assert!(!js.contains("__REQUEST__"));
    assert!(!js.contains("__REQUEST_ID__"));
}

#[test]
fn call_embeds_the_request_id_and_leaves_no_placeholder() {
    let js = request_script("req-1", "click", Some("e12"), None, false, None);
    assert!(!js.contains("__REQUEST_ID__"));
    assert!(!js.contains("__REQUEST__"));
    assert!(js.contains("api.run(\"req-1\""));
    assert!(js.contains("\"op\":\"click\""));
    assert!(js.contains("\"ref\":\"e12\""));
}

#[test]
fn call_escapes_quotes_and_newlines_in_text() {
    let js = request_script("req-2", "type", Some("e3"), Some("a\"b\nc"), true, None);
    assert!(js.contains("a\\\"b\\nc"));
    assert!(js.contains("\"submit\":true"));
}

#[test]
fn call_carries_the_scoped_snapshot_selector() {
    let js = request_script("req-3", "snapshot", None, None, false, Some("#main"));
    assert!(js.contains("\"selector\":\"#main\""));
}

#[test]
fn driver_keeps_its_error_contract_strings() {
    let js = init_script();
    for needle in [
        "Invalid selector",
        "is stale or unknown",
        "is covered by",
        "No option",
    ] {
        assert!(js.contains(needle), "driver lost {needle:?}");
    }
}
