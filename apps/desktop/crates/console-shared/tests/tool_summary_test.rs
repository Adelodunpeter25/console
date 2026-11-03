//! One-line summaries of the console and computer tools: the calls a script makes.

use console_shared::tool::{argument_summary, script_calls};
use serde_json::json;

#[test]
fn lists_distinct_calls_in_first_use_order() {
    assert_eq!(
        script_calls("console", "console.sessions.list(); console.sessions.read({session_id:'a'}); console.sessions.list()").as_deref(),
        Some("sessions.list, sessions.read")
    );
}

#[test]
fn handles_single_calls_and_top_level_methods() {
    assert_eq!(script_calls("console", "console.models()").as_deref(), Some("models"));
    assert_eq!(script_calls("console", "console.ports.list()").as_deref(), Some("ports.list"));
    assert_eq!(script_calls("computer", "computer.click({x:1})").as_deref(), Some("click"));
}

#[test]
fn a_mention_inside_a_string_or_comment_is_not_a_call() {
    assert_eq!(script_calls("console", r#"print("console.sessions.list()")"#), None);
    assert_eq!(script_calls("console", "print('console.sessions.list()')"), None);
    assert_eq!(script_calls("console", "print(`console.sessions.list()`)"), None);
    assert_eq!(script_calls("console", "// console.sessions.list()\nvar x = 1"), None);
    assert_eq!(script_calls("console", "/* console.sessions.list() */ var x = 1"), None);
    // ...but a real call after the string is still found.
    assert_eq!(
        script_calls("console", r#"print("console.x()"); console.ports.list()"#).as_deref(),
        Some("ports.list")
    );
}

#[test]
fn escaped_quotes_do_not_end_a_string_early() {
    assert_eq!(
        script_calls("console", r#"var s = "say \"console.a.b()\" now"; console.ports.list()"#).as_deref(),
        Some("ports.list")
    );
}

#[test]
fn only_calls_count_not_property_reads() {
    assert_eq!(script_calls("console", "var f = console.sessions.list"), None);
    assert_eq!(script_calls("console", "console.sessions.list ()").as_deref(), Some("sessions.list"));
}

#[test]
fn the_root_must_stand_alone() {
    // `myconsole.x()` and `foo.console.x()` are not calls on the console object.
    assert_eq!(script_calls("console", "myconsole.sessions.list()"), None);
    assert_eq!(script_calls("console", "foo.console.sessions.list()"), None);
    assert_eq!(script_calls("console", "$console.models()"), None);
}

#[test]
fn pure_javascript_has_no_summary() {
    assert_eq!(script_calls("console", "var x = 1 + 2; x"), None);
    assert_eq!(script_calls("console", ""), None);
}

#[test]
fn long_lists_are_truncated_to_one_line() {
    let code: String = (0..30).map(|i| format!("console.method{i}.run();")).collect();
    let out = script_calls("console", &code).unwrap();
    assert!(out.chars().count() <= 73, "too long: {}", out.chars().count());
    assert!(out.ends_with('…'));
    assert!(!out.contains('\n'));
}

#[test]
fn unicode_and_odd_input_never_panics() {
    for code in ["console.日本語.x()", "console.", "console.(", "\"unterminated", "/* unterminated", "console.a.b(((", "\u{1F525}console.x()"] {
        let _ = script_calls("console", code);
    }
}

#[test]
fn argument_summary_uses_the_calls_for_console_and_computer() {
    let args = json!({"code": "var r = console.sessions.list({cwd:'/tmp'}); console.sessions.read({session_id: r[0].id})"});
    assert_eq!(argument_summary("console", &args, None).as_deref(), Some("sessions.list, sessions.read"));
    let args = json!({"code": "computer.list_apps()"});
    assert_eq!(argument_summary("computer", &args, None).as_deref(), Some("list_apps"));
    // Other tools are unchanged by the new rule.
    assert_eq!(argument_summary("bash", &json!({"command": "ls -la"}), None).as_deref(), Some("ls -la"));
    // A console call with no code, or only plain JavaScript, shows nothing.
    assert_eq!(argument_summary("console", &json!({}), None), None);
    assert_eq!(argument_summary("console", &json!({"code": "1+1"}), None), None);
}
