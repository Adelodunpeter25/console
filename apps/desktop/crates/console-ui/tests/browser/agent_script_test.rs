//! Tests for the agent script wrapper and result capping.

use console_ui::browser::agent_script::{MAX_RESULT_CHARS, cap_result, wrap_script};

#[test]
fn wrap_script_embeds_request_id_and_escapes_source() {
    let wrapped = wrap_script("req-1", "document.title + \"x\"\n// end");
    assert!(wrapped.contains("\"req-1\""));
    assert!(wrapped.contains("document.title + \\\"x\\\"\\n// end"));
    assert!(!wrapped.contains("__REQUEST_ID__"));
    assert!(!wrapped.contains("__SOURCE__"));
    assert!(!wrapped.contains("__MAX_CHARS__"));
}

#[test]
fn cap_result_keeps_short_text_and_marks_truncation() {
    assert_eq!(cap_result("hello".to_string()), "hello");
    let long = "a".repeat(MAX_RESULT_CHARS + 10);
    let capped = cap_result(long);
    assert!(capped.contains("truncated"));
    assert!(capped.starts_with(&"a".repeat(MAX_RESULT_CHARS)));
}
