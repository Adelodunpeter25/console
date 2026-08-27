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
pub fn parse_unified_diff(raw_diff: &str) -> DiffResult {
    let mut lines = Vec::new();
    let mut added = 0usize;
    let mut removed = 0usize;
    let mut old_line_no = 0usize;
    let mut new_line_no = 0usize;
    let mut in_hunk = false;

    for line in raw_diff.lines() {
        if line.starts_with("@@") {
            in_hunk = true;
            if let Some((old_start, new_start)) = parse_hunk_header(line) {
                old_line_no = old_start;
                new_line_no = new_start;
            }
            continue;
        }

        if !in_hunk {
            continue;
        }

        if let Some(rest) = line.strip_prefix('+') {
            lines.push(DiffLine {
                kind: DiffLineKind::Added,
                text: rest.to_string(),
                old_no: None,
                new_no: Some(new_line_no),
            });
            added += 1;
            new_line_no += 1;
        } else if let Some(rest) = line.strip_prefix('-') {
            lines.push(DiffLine {
                kind: DiffLineKind::Removed,
                text: rest.to_string(),
                old_no: Some(old_line_no),
                new_no: None,
            });
            removed += 1;
            old_line_no += 1;
        } else if let Some(rest) = line.strip_prefix(' ') {
            lines.push(DiffLine {
                kind: DiffLineKind::Context,
                text: rest.to_string(),
                old_no: Some(old_line_no),
                new_no: Some(new_line_no),
            });
            old_line_no += 1;
            new_line_no += 1;
        } else if line.starts_with('\\') {
            continue;
        } else {
            lines.push(DiffLine {
                kind: DiffLineKind::Context,
                text: line.to_string(),
                old_no: Some(old_line_no),
                new_no: Some(new_line_no),
            });
            old_line_no += 1;
            new_line_no += 1;
        }
    }

    DiffResult {
        lines,
        added,
        removed,
    }
}

fn parse_hunk_header(header: &str) -> Option<(usize, usize)> {
    let parts: Vec<&str> = header.split("@@").collect();
    if parts.len() < 2 {
        return None;
    }
    let range_part = parts[1].trim();
    let mut ranges = range_part.split_whitespace();
    let old_range = ranges.next()?;
    let new_range = ranges.next()?;

    let old_start = old_range
        .strip_prefix('-')?
        .split(',')
        .next()?
        .parse::<usize>()
        .ok()?;
    let new_start = new_range
        .strip_prefix('+')?
        .split(',')
        .next()?
        .parse::<usize>()
        .ok()?;

    Some((old_start, new_start))
}
