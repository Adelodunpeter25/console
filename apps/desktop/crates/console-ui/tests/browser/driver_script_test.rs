//! Tests for the engine-agnostic driver script builder.
//!
//! The driver itself runs in the page, so these tests pin the Rust side of the
//! contract: the install script exists, a call embeds its request, user text
//! cannot break out of the call, and the result markers the agent relies on
//! stay present.

use console_ui::browser::driver::{Request, init_script, request_script};

#[test]
fn install_script_exposes_the_driver_api_and_has_no_request_placeholder() {
    let js = init_script();
    assert!(js.contains("window.__consoleDriverApi = { run }"));
    assert!(!js.contains("__REQUEST__"));
    assert!(!js.contains("__REQUEST_ID__"));
}

#[test]
fn call_embeds_the_request_id_and_leaves_no_placeholder() {
    let request = Request::action("click", Some("e12"), None, false, None);
    let js = request_script("req-1", &request);
    assert!(!js.contains("__REQUEST_ID__"));
    assert!(!js.contains("__REQUEST__"));
    assert!(js.contains("api.run(\"req-1\""));
    assert!(js.contains("\"op\":\"click\""));
    assert!(js.contains("\"ref\":\"e12\""));
}

#[test]
fn call_escapes_quotes_and_newlines_in_text() {
    let request = Request::action("type", Some("e3"), Some("a\"b\nc"), true, None);
    let js = request_script("req-2", &request);
    assert!(js.contains("a\\\"b\\nc"));
    assert!(js.contains("\"submit\":true"));
}

#[test]
fn call_carries_the_scoped_snapshot_selector() {
    let request = Request::action("snapshot", None, None, false, Some("#main"));
    let js = request_script("req-3", &request);
    assert!(js.contains("\"selector\":\"#main\""));
}

#[test]
fn eval_content_and_wait_requests_carry_their_payloads() {
    let eval = request_script("req-4", &Request::eval("document.title"));
    assert!(eval.contains("\"op\":\"eval\""));
    assert!(eval.contains("\"script\":\"document.title\""));

    let content = request_script("req-5", &Request::content(Some("main")));
    assert!(content.contains("\"op\":\"content\""));
    assert!(content.contains("\"selector\":\"main\""));

    let query = request_script("req-6", &Request::query(".result-card"));
    assert!(query.contains("\"op\":\"query\""));
    assert!(query.contains("\"selector\":\".result-card\""));

    let wait = request_script(
        "req-7",
        &Request::wait(Some("#done"), Some("/app"), Some("Ready")),
    );
    assert!(wait.contains("\"op\":\"wait\""));
    assert!(wait.contains("\"selector\":\"#done\""));
    assert!(wait.contains("\"urlContains\":\"/app\""));
    assert!(wait.contains("\"text\":\"Ready\""));
}

#[test]
fn driver_resolves_click_type_targets_without_a_ref() {
    let js = init_script();
    // The selector fallback for click/type (used by the server's browser
    // kernel): first visible match, invalid and no-match messages.
    assert!(js.contains("elements.find((candidate) => isVisible(candidate))"));
    assert!(js.contains("No visible match for selector: "));
    let request = Request::action("click", None, None, false, Some("#go"));
    let payload = request_script("req-8", &request);
    assert!(payload.contains("\"op\":\"click\""));
    assert!(payload.contains("\"selector\":\"#go\""));
}

#[test]
fn driver_keeps_its_result_and_error_contract_strings() {
    let js = init_script();
    for needle in [
        "Invalid selector",
        "is stale or unknown",
        "is covered by",
        "No option",
        "WF_OK",
        "WF_HIDDEN",
        "WF_ERR",
        "MAX_RESULT_CHARS",
        "aria-hidden",
        "hidden from the accessibility tree",
        "No visible match",
        "url: location.href",
        "title: document.title",
    ] {
        assert!(js.contains(needle), "driver lost {needle:?}");
    }
}

#[test]
fn extract_and_scroll_requests_carry_their_json_spec() {
    let extract = request_script("req-9", &Request::extract(r##"{"scope":"#buybox","limit":5}"##));
    assert!(extract.contains("\"op\":\"extract\""));
    assert!(extract.contains("\"spec\":\"{\\\"scope\\\":\\\"#buybox\\\",\\\"limit\\\":5}\""));

    let scroll = request_script("req-10", &Request::scroll(r#"{"direction":"down"}"#));
    assert!(scroll.contains("\"op\":\"scroll\""));
    assert!(scroll.contains("\"spec\":\"{\\\"direction\\\":\\\"down\\\"}\""));

    // Other ops leave the spec slot empty.
    let click = request_script("req-11", &Request::action("click", Some("e1"), None, false, None));
    assert!(click.contains("\"spec\":\"\""));
}

#[test]
fn driver_resolves_text_and_role_locators_in_the_action_path() {
    let js = init_script();
    // The kernel's {text} and {role, name} targets arrive as prefixed
    // selectors, resolved before the CSS fallback. No-match and bad-role
    // messages must stay recognizable.
    assert!(js.contains("startsWith('text=')"), "text= locator parsing lost");
    assert!(js.contains("startsWith('role=')"), "role= locator parsing lost");
    assert!(js.contains("No visible match for text: "), "text no-match message lost");
    assert!(js.contains("No visible match for role "), "role no-match message lost");
    // Exact match wins over substring, first in document order.
    assert!(js.contains("name === wanted || text === wanted"), "exact-first text matching lost");
    // Locators stay visible/a11y-filtered like a snapshot would be.
    assert!(js.contains("const resolveLocator"), "locator resolver lost");
}

#[test]
fn driver_extract_waits_for_new_content_and_reports_unchanged() {
    let js = init_script();
    // waitForNew: in-page poll on the page signature, capped budget, and an
    // explicit unchanged flag for the paginate loop.
    assert!(js.contains("lastSig"), "extract signature store lost");
    assert!(js.contains("waitForNew"), "waitForNew handling lost");
    assert!(js.contains("payload.unchanged = true"), "unchanged flag lost");
    // Extraction is bounded like the rest of the driver.
    assert!(js.contains("MAX_EXTRACT_ITEMS"), "extract item cap lost");
    assert!(js.contains("MAX_EXTRACT_CHARS"), "extract byte cap lost");
    // The scope list's comma parts are alternatives, tried in order.
    assert!(js.contains("Invalid scope selector"), "scope error message lost");
    assert!(js.contains("No visible match for scope: "), "scope no-match message lost");
}
