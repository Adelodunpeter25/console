use console_ui::browser::agent_driver::{
    click_commands, content_commands, normalize_ref, run_js_commands, screenshot_commands,
    session_name, snapshot_commands, type_commands, wait_commands,
};

fn cmd(parts: &[&str]) -> Vec<String> {
    parts.iter().map(|p| p.to_string()).collect()
}

#[test]
fn normalizes_refs_from_any_snapshot_style() {
    assert_eq!(normalize_ref("e12"), "@e12");
    assert_eq!(normalize_ref("@e12"), "@e12");
    assert_eq!(normalize_ref("ref=e12"), "@e12");
    assert_eq!(normalize_ref("  e3 "), "@e3");
}

#[test]
fn builds_snapshot_click_and_type() {
    assert_eq!(snapshot_commands(), vec![cmd(&["snapshot", "-i"])]);
    assert_eq!(click_commands("e5"), vec![cmd(&["click", "@e5"])]);
    assert_eq!(
        type_commands("e1", "hello world", false),
        vec![cmd(&["fill", "@e1", "hello world"])]
    );
    assert_eq!(
        type_commands("@e1", "q", true),
        vec![cmd(&["fill", "@e1", "q"]), cmd(&["press", "Enter"])]
    );
}

#[test]
fn script_text_is_one_argument_never_split_or_reinterpreted() {
    let script = "document.title; --cdp 1 && rm -rf /";
    assert_eq!(run_js_commands(script), vec![cmd(&["eval", script])]);
    assert_eq!(type_commands("e1", "--session x", false)[0][2], "--session x");
}

#[test]
fn content_defaults_to_the_whole_body() {
    assert_eq!(content_commands(None), vec![cmd(&["get", "text", "body"])]);
    assert_eq!(content_commands(Some("  ")), vec![cmd(&["get", "text", "body"])]);
    assert_eq!(content_commands(Some("main h1")), vec![cmd(&["get", "text", "main h1"])]);
}

#[test]
fn wait_builds_one_step_per_condition_with_a_timeout() {
    assert_eq!(
        wait_commands(Some("#done"), Some("/checkout"), Some("Thanks"), 5000),
        vec![
            cmd(&["wait", "#done", "--timeout", "5000"]),
            cmd(&["wait", "--text", "Thanks", "--timeout", "5000"]),
            cmd(&["wait", "--url", "**/checkout**", "--timeout", "5000"]),
        ]
    );
    assert_eq!(
        wait_commands(None, None, Some("x"), 100),
        vec![cmd(&["wait", "--text", "x", "--timeout", "100"])]
    );
    assert!(wait_commands(None, None, None, 100).is_empty());
}

#[test]
fn screenshot_saves_to_the_given_path() {
    assert_eq!(
        screenshot_commands("/tmp/a b.png"),
        vec![cmd(&["screenshot", "/tmp/a b.png"])]
    );
}

#[test]
fn session_name_is_stable_per_data_dir() {
    assert_eq!(session_name(None), "console");
    assert_eq!(session_name(Some("")), "console");
    let a = session_name(Some("/tmp/console-test-a"));
    assert_eq!(a, session_name(Some("/tmp/console-test-a")));
    assert_ne!(a, session_name(Some("/tmp/console-test-b")));
    assert!(a.starts_with("console-") && a.len() == "console-".len() + 8, "{a}");
    assert!(a.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'-'));
}
