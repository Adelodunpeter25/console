//! Unit tests for the user-message fold heuristic: long prompts fold to
//! 8 lines with a Show more/less toggle (parity with mobile's 4-line fold).

use console_ui::chat::message_bubble::estimated_text_lines;

#[test]
fn short_text_never_folds() {
    assert_eq!(estimated_text_lines(""), 1);
    assert_eq!(estimated_text_lines("hello"), 1);
    assert_eq!(estimated_text_lines("one\ntwo\nthree"), 3);
}

#[test]
fn hard_breaks_count_even_when_blank() {
    // "a\n\nb" renders three visual lines, not two.
    assert_eq!(estimated_text_lines("a\n\nb"), 3);
}

#[test]
fn long_single_line_wraps() {
    // 64 chars per estimated line: 65 chars spill onto a second line.
    let line = "x".repeat(64);
    assert_eq!(estimated_text_lines(&line), 1);
    assert_eq!(estimated_text_lines(&format!("{line}y")), 2);
}

#[test]
fn eight_lines_is_the_fold_boundary() {
    let eight = (1..=8)
        .map(|i| format!("line {i}"))
        .collect::<Vec<_>>()
        .join("\n");
    assert_eq!(estimated_text_lines(&eight), 8);
    assert_eq!(estimated_text_lines(&format!("{eight}\nline 9")), 9);
}
