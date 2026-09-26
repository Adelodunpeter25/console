//! Per-file diff extraction for whole-file write tool calls.
//!
//! A `batchWrite` targets several files at once, and the Go server records one
//! change per entry in apps/server-go/internal/run/file_changes.go. These
//! tests pin the desktop side to that behaviour: every written file yields its
//! own diff, in call order.

use console_core::utils::diff::{diff_lines, extract_write_args, extract_write_files, file_call_diffs};
use serde_json::json;

#[test]
fn single_write_yields_one_file() {
    let args = json!({"path": "/repo/src/main.rs", "content": "fn main() {}\n"});

    let files = extract_write_files(&args);

    assert_eq!(files.len(), 1);
    assert_eq!(files[0].0, "/repo/src/main.rs");
    assert_eq!(files[0].1, "fn main() {}\n");
}

#[test]
fn batch_write_yields_every_file_in_order() {
    let args = json!({
        "files": [
            {"path": "/repo/a.txt", "content": "one\n"},
            {"path": "/repo/b.txt", "content": "two\n"},
            {"path": "/repo/c.txt", "content": "three\n"},
        ]
    });

    let files = extract_write_files(&args);

    // The regression this pins: the old extractor returned only the first
    // file, so a 3-file batch reported as a 1-file write.
    assert_eq!(files.len(), 3);
    assert_eq!(
        files.iter().map(|(p, _)| p.as_str()).collect::<Vec<_>>(),
        vec!["/repo/a.txt", "/repo/b.txt", "/repo/c.txt"]
    );
    assert_eq!(files[2].1, "three\n");
}

#[test]
fn single_write_args_still_returns_first_file() {
    // Kept as the display fallback for a single-path label.
    let args = json!({"path": "/repo/a.txt", "content": "x\n"});
    assert_eq!(extract_write_args(&args).unwrap().0, "/repo/a.txt");

    let batch = json!({"files": [{"path": "/repo/a.txt", "content": "x\n"}]});
    assert_eq!(extract_write_args(&batch).unwrap().0, "/repo/a.txt");

    let empty = json!({"files": []});
    assert!(extract_write_args(&empty).is_none());
}

#[test]
fn malformed_batch_entries_are_skipped_not_fatal() {
    let args = json!({
        "files": [
            {"path": "/repo/good.txt", "content": "ok\n"},
            {"content": "no path\n"},
            {"path": "/repo/no-content.txt"},
            "not an object",
        ]
    });

    let files = extract_write_files(&args);

    // One bad element must not blank the whole batch.
    assert_eq!(files.len(), 1);
    assert_eq!(files[0].0, "/repo/good.txt");
}

#[test]
fn path_aliases_are_accepted() {
    for key in ["path", "filePath", "targetFile"] {
        let args = json!({key: "/repo/aliased.txt", "content": "x\n"});
        assert_eq!(
            extract_write_files(&args).len(),
            1,
            "alias {key} should resolve a path"
        );
    }
}

#[test]
fn non_object_arguments_yield_nothing() {
    assert!(extract_write_files(&json!(null)).is_empty());
    assert!(extract_write_files(&json!("string")).is_empty());
    assert!(extract_write_files(&json!([1, 2, 3])).is_empty());
    assert!(extract_write_files(&json!({})).is_empty());
    assert!(extract_write_files(&json!({"path": "/repo/x.txt"})).is_empty());
}

#[test]
fn batch_write_builds_a_diff_per_file() {
    let args = json!({
        "files": [
            {"path": "/repo/a.txt", "content": "line1\nline2\n"},
            {"path": "/repo/b.txt", "content": "other\n"},
        ]
    });

    let diffs = file_call_diffs("batchWrite", &args);

    assert_eq!(diffs.len(), 2);
    assert_eq!(diffs[0].path, "/repo/a.txt");
    assert_eq!(diffs[1].path, "/repo/b.txt");
    // Written content has no known "before", so every line is an addition.
    assert_eq!(diffs[0].diff.added, 2);
    assert_eq!(diffs[0].diff.removed, 0);
    assert_eq!(diffs[1].diff.added, 1);
}

#[test]
fn batch_write_underscore_alias_behaves_identically() {
    let args = json!({
        "files": [
            {"path": "/repo/a.txt", "content": "a\n"},
            {"path": "/repo/b.txt", "content": "b\n"},
        ]
    });

    assert_eq!(
        file_call_diffs("batch_write", &args).len(),
        2,
        "the server accepts both spellings, so the desktop must too"
    );
}

#[test]
fn edit_file_still_produces_exactly_one_diff_with_both_sides() {
    let args = json!({
        "path": "/repo/lib.rs",
        "oldContent": "a\nb\nc\n",
        "newContent": "a\nB\nc\n",
    });

    let diffs = file_call_diffs("editFile", &args);

    assert_eq!(diffs.len(), 1);
    assert_eq!(diffs[0].path, "/repo/lib.rs");
    assert_eq!(diffs[0].diff.added, 1);
    assert_eq!(diffs[0].diff.removed, 1);
}

#[test]
fn non_file_tools_have_no_diffs() {
    assert!(file_call_diffs("bash", &json!({"command": "ls"})).is_empty());
    assert!(file_call_diffs("readFile", &json!({"path": "/repo/a.txt"})).is_empty());
}

#[test]
fn trailing_newline_does_not_add_a_phantom_line() {
    // "a\nb\n" has two lines, not three. The Go emitter drops the empty
    // element split('\n') leaves behind, so the desktop must too or the two
    // disagree on every file's addition count.
    let result = diff_lines("", "line1\nline2\n", 3);

    assert_eq!(result.added, 2, "a trailing newline is not a line of content");
    assert!(
        !result
            .lines
            .iter()
            .any(|l| l.kind == console_core::utils::diff::DiffLineKind::Added && l.text.is_empty()),
        "no blank added line should be rendered"
    );
}

#[test]
fn genuine_blank_lines_still_count() {
    // "a\n\n" is two lines: "a" and an intentional empty line. Only the
    // element produced *by* a trailing newline is dropped, never a real blank.
    let result = diff_lines("", "a\n\n", 3);

    assert_eq!(result.added, 2);
    assert_eq!(result.lines[1].text, "");
}

#[test]
fn content_without_trailing_newline_is_unchanged() {
    let result = diff_lines("", "a\nb", 3);
    assert_eq!(result.added, 2);
}

#[test]
fn edit_side_also_drops_only_the_trailing_phantom() {
    // Both sides are split the same way, so an unchanged trailing newline
    // matches as an equal line and does not register as a change.
    let result = diff_lines("a\nb\n", "a\nb\n", 3);
    assert_eq!(result.added, 0);
    assert_eq!(result.removed, 0);

    let result = diff_lines("a\nb\n", "a\nc\n", 3);
    assert_eq!(result.added, 1);
    assert_eq!(result.removed, 1);
}
