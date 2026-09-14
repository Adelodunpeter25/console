use console_ui::{CodePosition, CodeSelection, build_file_lines};

#[test]
fn test_build_file_lines_empty() {
    let lines = build_file_lines("test.rs", "");
    assert_eq!(lines.len(), 0);
}

#[test]
fn test_build_file_lines_multiline() {
    let content = "fn main() {\n    println!(\"hello\");\n}\n";
    let lines = build_file_lines("main.rs", content);
    assert_eq!(lines.len(), 3);
    assert_eq!(lines[0].line_no, Some(1));
    assert_eq!(lines[0].text, "fn main() {");
    assert_eq!(lines[1].line_no, Some(2));
    assert_eq!(lines[1].text, "    println!(\"hello\");");
    assert_eq!(lines[2].line_no, Some(3));
    assert_eq!(lines[2].text, "}");
}

#[test]
fn test_code_selection_line_col_range() {
    let sel = CodeSelection::new(
        CodePosition { line: 1, col: 4 },
        CodePosition { line: 3, col: 2 },
    );

    // Line 0 is before selection
    assert_eq!(sel.line_col_range(0, 10), None);

    // Line 1 is the start line
    assert_eq!(sel.line_col_range(1, 10), Some((4, 10)));

    // Line 2 is completely inside selection
    assert_eq!(sel.line_col_range(2, 10), Some((0, 10)));

    // Line 3 is the end line
    assert_eq!(sel.line_col_range(3, 10), Some((0, 2)));

    // Line 4 is after selection
    assert_eq!(sel.line_col_range(4, 10), None);
}

#[test]
fn test_code_selection_reversed_drag() {
    // User drags from line 3 col 5 backwards up to line 1 col 2
    let sel = CodeSelection::new(
        CodePosition { line: 3, col: 5 },
        CodePosition { line: 1, col: 2 },
    );

    let start = sel.start();
    let end = sel.end();

    assert_eq!(start, CodePosition { line: 1, col: 2 });
    assert_eq!(end, CodePosition { line: 3, col: 5 });
    assert_eq!(sel.line_col_range(1, 10), Some((2, 10)));
}
