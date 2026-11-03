use console_ui::markdown::selection::format_cite_quote;

#[test]
fn blank_selection_cites_nothing() {
    assert_eq!(format_cite_quote(""), None);
    assert_eq!(format_cite_quote("   \n \n "), None);
}

#[test]
fn single_line_gets_quote_prefix_and_reply_gap() {
    assert_eq!(
        format_cite_quote("hello world"),
        Some("> hello world\n\n".into())
    );
}

#[test]
fn every_line_is_quoted() {
    assert_eq!(
        format_cite_quote("first\nsecond\nthird"),
        Some("> first\n> second\n> third\n\n".into())
    );
}

#[test]
fn trims_surrounding_blank_lines_but_keeps_interior_ones() {
    assert_eq!(
        format_cite_quote("\n\npara one\n\npara two\n\n"),
        Some("> para one\n\n> para two\n\n".into())
    );
}

#[test]
fn trailing_whitespace_is_trimmed_per_line() {
    assert_eq!(
        format_cite_quote("word   \nmore\t"),
        Some("> word\n> more\n\n".into())
    );
}

#[test]
fn multi_byte_characters_survive() {
    assert_eq!(
        format_cite_quote("héllo → 世界"),
        Some("> héllo → 世界\n\n".into())
    );
}
