use console_ui::input::is_large_paste;

#[test]
fn small_pastes_stay_inline() {
    assert!(!is_large_paste("one line"));
    assert!(!is_large_paste(&"x\n".repeat(10)));
    assert!(!is_large_paste(&format!("{}\n", "a".repeat(24))));
}

#[test]
fn line_threshold_collapses() {
    // 26 lines collapses even though byte count is small.
    let text = "a\n".repeat(26);
    assert!(is_large_paste(&text));
    // Exactly 25 lines fits.
    assert!(!is_large_paste(&"a\n".repeat(25)));
}

#[test]
fn byte_threshold_collapses() {
    // One long line over 4 KiB collapses.
    let text = "a".repeat(4 * 1024 + 1);
    assert!(is_large_paste(&text));
    // Just under the cap fits.
    assert!(!is_large_paste(&"a".repeat(4 * 1024)));
}

#[test]
fn trailing_newline_is_no_extra_line() {
    // 50 newlines but no trailing segment: 50 lines, over the threshold.
    assert!(is_large_paste(&"\n".repeat(50)));
    // A trailing newline closes the last line, not a new one.
    assert!(!is_large_paste(&"a\n".repeat(25)));
}
