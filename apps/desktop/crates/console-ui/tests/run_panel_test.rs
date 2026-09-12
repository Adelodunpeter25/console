//! Unit tests for the Run panel display helpers.

use console_core::{compute_shortcut_state, ProjectScript};
use console_ui::run::{display_script_shortcut, MAX_RUN_OUTPUT_BYTES, push_run_output};

fn script(id: &str, shortcut: Option<&str>) -> ProjectScript {
    ProjectScript {
        id: id.into(),
        label: id.into(),
        command: format!("echo {id}"),
        shortcut: shortcut.map(str::to_string),
        persistent: false,
    }
}

#[test]
fn shortcut_state_unique_shortcuts_map_one_to_one() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("build", Some("cmd-shift-b")),
    ];
    let map = compute_shortcut_state(&scripts).expect("no conflicts");
    assert_eq!(map.get("cmd-r").map(String::as_str), Some("dev"));
    assert_eq!(map.get("cmd-shift-b").map(String::as_str), Some("build"));
}

#[test]
fn shortcut_state_detects_two_way_conflicts() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("build", Some("cmd-r")),
    ];
    let conflicts = compute_shortcut_state(&scripts).expect_err("must conflict");
    let ids = conflicts.get("cmd-r").expect("conflict recorded");
    assert_eq!(ids.len(), 2);
    assert!(ids.contains("dev"));
    assert!(ids.contains("build"));
}

#[test]
fn shortcut_state_three_way_conflict_includes_all_scripts() {
    let scripts = vec![
        script("a", Some("cmd-r")),
        script("b", Some("cmd-r")),
        script("c", Some("cmd-r")),
    ];
    let conflicts = compute_shortcut_state(&scripts).expect_err("must conflict");
    assert_eq!(conflicts.get("cmd-r").map(|s| s.len()), Some(3));
}

#[test]
fn shortcut_state_skips_scripts_without_shortcuts() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("lint", None),
        script("test", Some("cmd-shift-b")),
    ];
    let map = compute_shortcut_state(&scripts).expect("no conflicts");
    assert_eq!(map.len(), 2);
    assert!(map.contains_key("cmd-r"));
    assert!(map.contains_key("cmd-shift-b"));
}

#[test]
fn shortcut_state_mixed_conflict_keeps_unique_shortcuts_active() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("build", Some("cmd-r")),
        script("test", Some("cmd-shift-b")),
    ];
    let conflicts = compute_shortcut_state(&scripts).expect_err("must conflict");
    assert!(conflicts.contains_key("cmd-r"));
    assert!(!conflicts.contains_key("cmd-shift-b"));
}

#[test]
fn display_shortcut_cmd_letter() {
    assert_eq!(display_script_shortcut("cmd-r"), "⌘R");
}

#[test]
fn display_shortcut_cmd_shift() {
    assert_eq!(display_script_shortcut("cmd-shift-b"), "⇧⌘B");
}

#[test]
fn display_shortcut_ctrl_alt() {
    assert_eq!(display_script_shortcut("ctrl-alt-l"), "⌃⌥L");
}

#[test]
fn display_shortcut_single_word() {
    // The display helper uppercases the entire key, so "space" becomes
    // "SPACE". Documenting current behavior; revisit if we ever add a
    // special-case for space.
    assert_eq!(display_script_shortcut("cmd-space"), "⌘SPACE");
}

#[test]
fn display_shortcut_alone_is_uppercased() {
    // "cmd" with no key segment has the modifier treated as the key and no
    // prefix attached (the modifier list is consumed before lookup). This
    // isn't a real-world shortcut.
    assert_eq!(display_script_shortcut("cmd"), "CMD");
}

#[test]
fn output_tail_cap_keeps_tail() {
    let mut buf = String::new();
    for _ in 0..10 {
        buf.push_str(&"x".repeat(MAX_RUN_OUTPUT_BYTES / 4));
    }
    // Way over the cap now — push_run_output must trim to the tail.
    buf.push_str("recent-tail-marker");
    push_run_output(&mut buf, "more");
    assert!(
        buf.ends_with("more"),
        "tail should always include the most recent write"
    );
    assert!(
        buf.len() <= MAX_RUN_OUTPUT_BYTES + "more".len(),
        "tail must be bounded by MAX_RUN_OUTPUT_BYTES plus the latest fragment"
    );
}
