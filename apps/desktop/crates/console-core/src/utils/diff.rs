//! Line-level diff computation for file-edit tool calls.
//!
//! Uses the [`similar`](https://crates.io/crates/similar) crate (Myers
//! algorithm) for the core diff computation — the same engine used by
//! gitoxide and rustc. This is O(ND) where D is the number of differences,
//! which is significantly faster than a naive O(NM) LCS for large files
//! with small changes (the common case for `editFile`).

use similar::{Algorithm, DiffTag, capture_diff_slices};

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum DiffLineKind {
    Context,
    Added,
    Removed,
}

#[derive(Clone, Debug)]
pub struct DiffLine {
    pub kind: DiffLineKind,
    pub text: String,
    /// 1-based old-file line number for context/removed lines.
    pub old_no: Option<usize>,
    /// 1-based new-file line number for context/added lines.
    pub new_no: Option<usize>,
}

#[derive(Clone, Debug, Default)]
pub struct DiffResult {
    pub lines: Vec<DiffLine>,
    pub added: usize,
    pub removed: usize,
}

impl DiffResult {
    pub fn is_empty(&self) -> bool {
        self.lines.is_empty()
    }
}

/// Compute a line-level diff between `old` and `new`.
///
/// Returns a flat list of `DiffLine`s in file order (not the standard
/// unified-diff hunk format), which is easier to render in a scrollable
/// UI block. Context lines are included to give the viewer anchors.
///
/// The `context` parameter controls how many unchanged lines of context to
/// keep around each changed region. Pass `0` to show only added/removed lines.
pub fn diff_lines(old: &str, new: &str, context: usize) -> DiffResult {
    let old_lines: Vec<&str> = if old.is_empty() {
        Vec::new()
    } else {
        old.split('\n').collect()
    };
    let new_lines: Vec<&str> = if new.is_empty() {
        Vec::new()
    } else {
        new.split('\n').collect()
    };

    // Myers algorithm — O(ND), fast for small changes in large files.
    let ops = capture_diff_slices(Algorithm::Myers, &old_lines, &new_lines);

    let mut added = 0usize;
    let mut removed = 0usize;
    let mut lines: Vec<DiffLine> = Vec::new();

    for op in ops {
        let old_range = op.old_range();
        let new_range = op.new_range();
        match op.tag() {
            DiffTag::Equal => {
                for offset in 0..old_range.len() {
                    lines.push(DiffLine {
                        kind: DiffLineKind::Context,
                        text: old_lines[old_range.start + offset].to_owned(),
                        old_no: Some(old_range.start + offset + 1),
                        new_no: Some(new_range.start + offset + 1),
                    });
                }
            }
            DiffTag::Delete => {
                for offset in 0..old_range.len() {
                    lines.push(DiffLine {
                        kind: DiffLineKind::Removed,
                        text: old_lines[old_range.start + offset].to_owned(),
                        old_no: Some(old_range.start + offset + 1),
                        new_no: None,
                    });
                    removed += 1;
                }
            }
            DiffTag::Insert => {
                for offset in 0..new_range.len() {
                    lines.push(DiffLine {
                        kind: DiffLineKind::Added,
                        text: new_lines[new_range.start + offset].to_owned(),
                        old_no: None,
                        new_no: Some(new_range.start + offset + 1),
                    });
                    added += 1;
                }
            }
            DiffTag::Replace => {
                for offset in 0..old_range.len() {
                    lines.push(DiffLine {
                        kind: DiffLineKind::Removed,
                        text: old_lines[old_range.start + offset].to_owned(),
                        old_no: Some(old_range.start + offset + 1),
                        new_no: None,
                    });
                    removed += 1;
                }
                for offset in 0..new_range.len() {
                    lines.push(DiffLine {
                        kind: DiffLineKind::Added,
                        text: new_lines[new_range.start + offset].to_owned(),
                        old_no: None,
                        new_no: Some(new_range.start + offset + 1),
                    });
                    added += 1;
                }
            }
        }
    }

    // Apply context window: keep only `context` lines of context around changes.
    if context > 0 {
        lines = apply_context_window(lines, context);
    } else {
        lines.retain(|l| l.kind != DiffLineKind::Context);
    }

    DiffResult {
        lines,
        added,
        removed,
    }
}

fn apply_context_window(lines: Vec<DiffLine>, context: usize) -> Vec<DiffLine> {
    let changed: Vec<usize> = lines
        .iter()
        .enumerate()
        .filter(|(_, l)| l.kind != DiffLineKind::Context)
        .map(|(i, _)| i)
        .collect();

    if changed.is_empty() {
        return lines;
    }

    let keep: Vec<bool> = {
        let mut keep = vec![false; lines.len()];
        for &idx in &changed {
            let start = idx.saturating_sub(context);
            let end = (idx + context + 1).min(lines.len());
            for k in start..end {
                keep[k] = true;
            }
        }
        keep
    };

    lines
        .into_iter()
        .enumerate()
        .filter_map(|(i, line)| {
            if keep[i] {
                Some(line)
            } else {
                None
            }
        })
        .collect()
}

/// Try to extract `oldContent` / `newContent` from an `editFile` tool-call
/// arguments JSON value. Returns `None` if the shape doesn't match.
pub fn extract_edit_args(arguments: &serde_json::Value) -> Option<(&str, &str)> {
    let obj = arguments.as_object()?;
    let old = obj.get("oldContent").and_then(|v| v.as_str())?;
    let new = obj.get("newContent").and_then(|v| v.as_str())?;
    Some((old, new))
}

/// Try to extract file content from a `writeFile` / `batchWrite` tool-call
/// arguments JSON value. For `writeFile` this returns a single (path, content).
/// For `batchWrite` this returns the first file. Returns `None` otherwise.
pub fn extract_write_args(arguments: &serde_json::Value) -> Option<(String, String)> {
    let obj = arguments.as_object()?;
    if let (Some(path), Some(content)) = (obj.get("path").and_then(|v| v.as_str()), obj.get("content").and_then(|v| v.as_str()))
    {
        return Some((path.to_owned(), content.to_owned()));
    }
    if let Some(files) = obj.get("files").and_then(|v| v.as_array()) {
        if let Some(first) = files.first()
            && let (Some(path), Some(content)) = (
                first.get("path").and_then(|v| v.as_str()),
                first.get("content").and_then(|v| v.as_str()),
            )
        {
            return Some((path.to_owned(), content.to_owned()));
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_added_and_removed_lines() {
        let result = diff_lines("a\nb\nc", "a\nx\nc", 3);
        assert_eq!(result.added, 1);
        assert_eq!(result.removed, 1);
        assert!(result.lines.iter().any(|l| l.text == "b" && l.kind == DiffLineKind::Removed));
        assert!(result.lines.iter().any(|l| l.text == "x" && l.kind == DiffLineKind::Added));
    }

    #[test]
    fn identical_strings_produce_no_changes() {
        let result = diff_lines("hello\nworld", "hello\nworld", 3);
        assert_eq!(result.added, 0);
        assert_eq!(result.removed, 0);
    }

    #[test]
    fn empty_old_treats_all_as_added() {
        let result = diff_lines("", "new\nfile", 3);
        assert_eq!(result.added, 2);
        assert_eq!(result.removed, 0);
    }

    #[test]
    fn empty_new_treats_all_as_removed() {
        let result = diff_lines("old\nfile", "", 3);
        assert_eq!(result.added, 0);
        assert_eq!(result.removed, 2);
    }

    #[test]
    fn context_window_filters_distant_unchanged_lines() {
        let old = "1\n2\n3\n4\n5\n6\n7\n8\n9\n10";
        let new = "1\n2\n3\n4\nX\n6\n7\n8\n9\n10";
        let result = diff_lines(old, new, 2);
        let texts: Vec<&str> = result.lines.iter().map(|l| l.text.as_str()).collect();
        assert!(texts.contains(&"X"));
        assert!(texts.contains(&"4")); // context before
        assert!(texts.contains(&"6")); // context after
        assert!(!texts.contains(&"1")); // too far
    }

    #[test]
    fn zero_context_shows_only_changes() {
        let result = diff_lines("a\nb\nc", "a\nx\nc", 0);
        assert!(result.lines.iter().all(|l| l.kind != DiffLineKind::Context));
        assert_eq!(result.lines.len(), 2); // only b removed + x added
    }

    #[test]
    fn line_numbers_are_tracked() {
        let result = diff_lines("a\nb\nc", "a\nb\nc\nd", 3);
        let added = result.lines.iter().find(|l| l.kind == DiffLineKind::Added).unwrap();
        assert_eq!(added.new_no, Some(4));
        assert_eq!(added.old_no, None);
    }

    #[test]
    fn handles_large_file_with_small_change() {
        // 1000 lines, one change in the middle — similar's Myers should
        // handle this efficiently (O(ND), not O(NM)).
        let old: String = (0..1000).map(|i| format!("line {i}")).collect::<Vec<_>>().join("\n");
        let new = old.replace("line 500", "line 500 (edited)");
        let result = diff_lines(&old, &new, 3);
        assert_eq!(result.added, 1);
        assert_eq!(result.removed, 1);
        // Context window should keep only ~7 lines (3 context + 1 removed + 1 added + 1 context boundary)
        assert!(result.lines.len() < 20);
    }

    #[test]
    fn extract_edit_args_works() {
        let args = serde_json::json!({"path": "foo.rs", "oldContent": "old", "newContent": "new"});
        let (old, new) = extract_edit_args(&args).unwrap();
        assert_eq!(old, "old");
        assert_eq!(new, "new");
    }

    #[test]
    fn extract_write_args_works() {
        let args = serde_json::json!({"path": "foo.rs", "content": "hello"});
        let (path, content) = extract_write_args(&args).unwrap();
        assert_eq!(path, "foo.rs");
        assert_eq!(content, "hello");
    }
}
