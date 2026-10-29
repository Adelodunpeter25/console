use console_ui::input::is_large_paste;

#[test]
fn small_pastes_stay_inline() {
    assert!(!is_large_paste("one line"));
    assert!(!is_large_paste(&"x\n".repeat(10)));
    assert!(!is_large_paste(&format!("{}\n", "a".repeat(199))));
}

#[test]
fn line_threshold_collapses() {
    // 201 lines collapses even though byte count is small.
    let text = "a\n".repeat(201);
    assert!(is_large_paste(&text));
    // Exactly 200 lines fits.
    assert!(!is_large_paste(&"a\n".repeat(200)));
}

#[test]
fn byte_threshold_collapses() {
    // One long line over 64 KiB collapses.
    let text = "a".repeat(64 * 1024 + 1);
    assert!(is_large_paste(&text));
    // Just under the cap fits.
    assert!(!is_large_paste(&"a".repeat(64 * 1024)));
}

#[test]
fn trailing_newline_is_no_extra_line() {
    // 300 newlines but no trailing segment: 300 lines, over the threshold.
    assert!(is_large_paste(&"\n".repeat(300)));
    // A trailing newline closes the last line, not a new one.
    assert!(!is_large_paste(&"a\n".repeat(200)));
}
