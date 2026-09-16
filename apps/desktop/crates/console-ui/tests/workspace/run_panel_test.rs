//! Unit tests for the Run tab helpers: shortcut canonicalization and
//! dispatch state, shortcut display, status labels, and the output cap.

use console_core::{ProjectScript, ScriptRunStatus, canonicalize_shortcut, compute_shortcut_state};
use console_ui::run::{
    MAX_RUN_OUTPUT_BYTES, cap_run_output, display_script_shortcut, push_run_output,
    script_row_status_label,
};

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
fn canonicalize_orders_modifiers_and_lowercases_key() {
    assert_eq!(canonicalize_shortcut("cmd-r").as_deref(), Some("cmd-r"));
    assert_eq!(
        canonicalize_shortcut("shift-cmd-R").as_deref(),
        Some("cmd-shift-r")
    );
    assert_eq!(
        canonicalize_shortcut("cmd-shift-b").as_deref(),
        Some("cmd-shift-b")
    );
}

#[test]
fn canonicalize_rejects_bare_keys_and_unknown_segments() {
    assert_eq!(canonicalize_shortcut("r"), None);
    assert_eq!(canonicalize_shortcut(""), None);
    assert_eq!(canonicalize_shortcut("cmd-foo-r"), None);
    assert_eq!(canonicalize_shortcut("cmd-"), None);
}

#[test]
fn shortcut_state_unique_shortcuts_map_one_to_one() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("build", Some("cmd-shift-b")),
    ];
    let state = compute_shortcut_state(&scripts);
    assert!(state.conflicts.is_empty());
    assert_eq!(state.bindings.get("cmd-r").map(String::as_str), Some("dev"));
    assert_eq!(
        state.bindings.get("cmd-shift-b").map(String::as_str),
        Some("build")
    );
}

#[test]
fn shortcut_state_matches_regardless_of_author_order_or_case() {
    let scripts = vec![script("dev", Some("shift-cmd-R"))];
    let state = compute_shortcut_state(&scripts);
    assert_eq!(
        state.bindings.get("cmd-shift-r").map(String::as_str),
        Some("dev")
    );
}

#[test]
fn shortcut_state_detects_two_way_conflicts() {
    let scripts = vec![script("dev", Some("cmd-r")), script("build", Some("cmd-r"))];
    let state = compute_shortcut_state(&scripts);
    assert!(state.bindings.is_empty());
    let ids = state.conflicts.get("cmd-r").expect("conflict recorded");
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
    let state = compute_shortcut_state(&scripts);
    assert_eq!(state.conflicts.get("cmd-r").map(|s| s.len()), Some(3));
}

#[test]
fn shortcut_state_skips_scripts_without_shortcuts() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("lint", None),
        script("test", Some("cmd-shift-b")),
    ];
    let state = compute_shortcut_state(&scripts);
    assert!(state.conflicts.is_empty());
    assert_eq!(state.bindings.len(), 2);
    assert!(state.bindings.contains_key("cmd-r"));
    assert!(state.bindings.contains_key("cmd-shift-b"));
}

#[test]
fn shortcut_state_mixed_conflict_keeps_unique_shortcuts_live() {
    let scripts = vec![
        script("dev", Some("cmd-r")),
        script("build", Some("cmd-r")),
        script("test", Some("cmd-shift-b")),
    ];
    let state = compute_shortcut_state(&scripts);
    assert!(state.conflicts.contains_key("cmd-r"));
    assert!(!state.conflicts.contains_key("cmd-shift-b"));
    // Only the duplicated shortcut is disabled — the unique one still binds.
    assert_eq!(
        state.bindings.get("cmd-shift-b").map(String::as_str),
        Some("test")
    );
    assert!(!state.bindings.contains_key("cmd-r"));
}

#[test]
fn test_shortcut_display_uses_macos_glyphs() {
    assert_eq!(display_script_shortcut("cmd-r"), "⌘R");
    assert_eq!(display_script_shortcut("cmd-shift-b"), "⇧⌘B");
    assert_eq!(display_script_shortcut("ctrl-alt-l"), "⌃⌥L");
    assert_eq!(display_script_shortcut("cmd-o"), "⌘O");
}

#[test]
fn test_shortcut_display_tolerates_case_and_opt_spelling() {
    assert_eq!(display_script_shortcut("CMD-R"), "⌘R");
    assert_eq!(display_script_shortcut("opt-1"), "⌥1");
}

#[test]
fn test_status_labels_cover_every_row_state() {
    assert_eq!(script_row_status_label(None, false), "Not run yet");
    assert_eq!(script_row_status_label(None, true), "Starting…");
    assert_eq!(
        script_row_status_label(Some(ScriptRunStatus::Running), false),
        "Running…"
    );
    assert_eq!(
        script_row_status_label(Some(ScriptRunStatus::Succeeded), false),
        "Succeeded"
    );
    assert_eq!(
        script_row_status_label(Some(ScriptRunStatus::Failed), false),
        "Failed"
    );
    assert_eq!(
        script_row_status_label(Some(ScriptRunStatus::Stopped), false),
        "Stopped"
    );
}

#[test]
fn test_output_cap_keeps_the_tail() {
    let big = "x".repeat(MAX_RUN_OUTPUT_BYTES + 100);
    let capped = cap_run_output(big);
    assert_eq!(capped.len(), MAX_RUN_OUTPUT_BYTES);
    assert!(capped.chars().all(|c| c == 'x'));

    let mut output = String::from("old");
    push_run_output(&mut output, "new");
    assert_eq!(output, "oldnew");
}

#[test]
fn test_output_cap_respects_utf8_boundaries() {
    let mut output = "é".repeat(MAX_RUN_OUTPUT_BYTES);
    push_run_output(&mut output, "!");
    assert!(output.len() <= MAX_RUN_OUTPUT_BYTES);
    assert!(output.len() > MAX_RUN_OUTPUT_BYTES - 4);
    assert!(output.ends_with('!'));
    assert!(output.is_char_boundary(0));
}
