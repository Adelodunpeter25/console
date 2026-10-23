//! Per-file diff extraction for whole-file write tool calls.
//!
//! A `batchWrite` targets several files at once, and the Go server records one
//! change per entry in apps/server-go/internal/run/file_changes.go. These
//! tests pin the desktop side to that behaviour: every written file yields its
//! own diff, in call order.

use console_shared::diff::{
    DiffLineKind, DiffResult, diff_lines, extract_write_args, extract_write_files, file_call_diffs,
    parse_unified_diff,
};
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

    assert_eq!(
        result.added, 2,
        "a trailing newline is not a line of content"
    );
    assert!(
        !result
            .lines
            .iter()
            .any(|l| l.kind == DiffLineKind::Added && l.text.is_empty()),
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

// ── parse_unified_diff ─────────────────────────────────────────────────────
//
// The Android parser this replaces treated any line starting `---`/`+++` as a
// file header, so a removed line whose *content* began with `--` (an SQL
// comment, a YAML `---` marker) vanished and every later line number slipped.
// Hunk tracking has to be structural, not prefix-based.

fn kinds(result: &DiffResult) -> Vec<(DiffLineKind, String)> {
    result
        .lines
        .iter()
        .map(|l| (l.kind.clone(), l.text.clone()))
        .collect()
}

#[test]
fn removed_line_that_looks_like_a_file_header_is_kept() {
    let raw = "diff --git a/q.sql b/q.sql\nindex 1..2 100644\n--- a/q.sql\n+++ b/q.sql\n\
               @@ -1,3 +1,2 @@\n keep\n--- drop\n-last\n\\ No newline at end of file\n+end\n";
    let parsed = parse_unified_diff(raw);
    assert_eq!(
        kinds(&parsed),
        vec![
            (DiffLineKind::Context, "keep".into()),
            (DiffLineKind::Removed, "-- drop".into()),
            (DiffLineKind::Removed, "last".into()),
            (DiffLineKind::Added, "end".into()),
        ]
    );
    assert_eq!((parsed.added, parsed.removed), (1, 2));
}

#[test]
fn line_numbers_stay_correct_after_a_header_lookalike() {
    let raw = "@@ -1,3 +1,2 @@\n keep\n--- drop\n-last\n+end\n";
    let lines = parse_unified_diff(raw).lines;
    assert_eq!((lines[1].old_no, lines[1].new_no), (Some(2), None));
    assert_eq!((lines[2].old_no, lines[2].new_no), (Some(3), None));
    assert_eq!((lines[3].old_no, lines[3].new_no), (None, Some(2)));
}

#[test]
fn added_line_that_looks_like_a_file_header_is_kept() {
    let raw = "@@ -1 +1,2 @@\n a\n+++ not a header\n";
    let parsed = parse_unified_diff(raw);
    assert_eq!(parsed.added, 1);
    assert_eq!(parsed.lines[1].text, "++ not a header");
}

#[test]
fn no_newline_marker_is_not_a_row() {
    let raw = "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n";
    assert_eq!(parse_unified_diff(raw).lines.len(), 2);
}

#[test]
fn trailing_newline_adds_no_phantom_row() {
    let with = parse_unified_diff("@@ -1 +1 @@\n-a\n+b\n");
    let without = parse_unified_diff("@@ -1 +1 @@\n-a\n+b");
    assert_eq!(with.lines.len(), 2);
    assert_eq!(with.lines.len(), without.lines.len());
}

#[test]
fn short_and_long_hunk_headers_both_set_the_counters() {
    // `-N`/`+N` (no length) is what difflib writes for a one-line range.
    let short = parse_unified_diff("@@ -7 +9 @@\n-x\n+y\n");
    assert_eq!(short.lines[0].old_no, Some(7));
    assert_eq!(short.lines[1].new_no, Some(9));
    let long = parse_unified_diff("@@ -7,3 +9,4 @@ fn ctx\n a\n");
    assert_eq!((long.lines[0].old_no, long.lines[0].new_no), (Some(7), Some(9)));
    // A zero-length old range: `-0,0`.
    let new_file = parse_unified_diff("@@ -0,0 +1,2 @@\n+a\n+b\n");
    assert_eq!(new_file.lines[0].new_no, Some(1));
    assert_eq!(new_file.added, 2);
}

#[test]
fn several_hunks_restart_the_counters() {
    let raw = "@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -20,2 +20,2 @@\n y\n-z\n+Z\n";
    let lines = parse_unified_diff(raw).lines;
    assert_eq!(lines[3].old_no, Some(20));
    assert_eq!(lines[5].new_no, Some(21));
}

#[test]
fn text_before_the_first_hunk_is_ignored() {
    let raw = "diff --git a/f b/f\nnew file mode 100644\nindex 0..1\n--- /dev/null\n+++ b/f\n@@ -0,0 +1 @@\n+hi\n";
    let parsed = parse_unified_diff(raw);
    assert_eq!(parsed.lines.len(), 1);
    assert_eq!(parsed.lines[0].text, "hi");
}

#[test]
fn empty_input_is_an_empty_diff() {
    assert!(parse_unified_diff("").is_empty());
}

// ── diff_lines on large inputs ─────────────────────────────────────────────

#[test]
fn one_edit_in_a_large_file_is_one_line_each_way() {
    // The Kotlin LCS bailed out above 4M cells and reported the whole file as
    // removed and re-added: a 2,100-line file with one edit read "+2100 −2100".
    let old: String = (0..2100).map(|i| format!("line {i}\n")).collect();
    let mut lines: Vec<String> = (0..2100).map(|i| format!("line {i}")).collect();
    lines[1000] = "CHANGED".into();
    let new = lines.join("\n") + "\n";
    let result = diff_lines(&old, &new, 3);
    assert_eq!((result.added, result.removed), (1, 1));
    // Three lines of context either side of the change, nothing more.
    assert_eq!(result.lines.len(), 2 + 6);
}

#[test]
fn context_window_zero_keeps_only_the_changes() {
    let result = diff_lines("a\nb\nc\n", "a\nB\nc\n", 0);
    assert_eq!(result.lines.len(), 2);
    assert!(result.lines.iter().all(|l| l.kind != DiffLineKind::Context));
}

#[test]
fn context_window_trims_distant_unchanged_lines() {
    let old: String = (0..20).map(|i| format!("l{i}\n")).collect();
    let new = old.replace("l10\n", "X\n");
    let result = diff_lines(&old, &new, 2);
    // 2 before + removed + added + 2 after.
    assert_eq!(result.lines.len(), 6);
    assert_eq!(result.lines[0].old_no, Some(9));
}
