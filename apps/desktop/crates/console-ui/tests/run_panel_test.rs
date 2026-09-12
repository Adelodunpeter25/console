//! Unit tests for the Run tab helpers: shortcut display, status labels,
//! and the retained-output cap.

use console_core::ScriptRunStatus;
use console_ui::run::{
    MAX_RUN_OUTPUT_BYTES, cap_run_output, display_script_shortcut, push_run_output,
    script_row_status_label,
};

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
