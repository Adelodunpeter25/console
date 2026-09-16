//! Path and folder display utilities.

// NOTE: there is deliberately no `base_name` here — use
// `crate::primitives::base_name`, the single shared implementation.

/// Parent directory of a path, handling both separator styles.
/// Returns `""` for bare filenames.
pub fn parent_dir(path: &str) -> &str {
    match path.rsplit_once(['/', '\\']) {
        Some((parent, _)) => parent,
        None => "",
    }
}

/// How aggressively `short_parent_dir` shortens a parent directory.
pub struct ParentDirDisplay {
    /// Trailing segments kept when shortening (`…/a/b/c`).
    pub keep_segments: usize,
    /// Parents at or below this length are returned whole.
    pub max_len: usize,
}

impl Default for ParentDirDisplay {
    fn default() -> Self {
        Self {
            keep_segments: 3,
            max_len: 28,
        }
    }
}

/// Shorten a parent dir for narrow rows: keep the trailing segments that
/// actually disambiguate same-name files.
/// `apps/desktop/crates/console-ui/src/markdown` → `…/console-ui/src/markdown`.
pub fn short_parent_dir(path: &str) -> String {
    short_parent_dir_with(path, &ParentDirDisplay::default())
}

/// [`short_parent_dir`] with explicit tuning. `keep_segments` is at least 1.
pub fn short_parent_dir_with(path: &str, display: &ParentDirDisplay) -> String {
    let parent = parent_dir(path);
    if parent.is_empty() {
        return String::new();
    }
    let keep = display.keep_segments.max(1);
    let segments: Vec<&str> = parent
        .split(['/', '\\'])
        .filter(|s| !s.is_empty())
        .collect();
    if segments.len() <= keep || parent.len() <= display.max_len {
        return parent.replace('\\', "/");
    }
    format!("…/{}", segments[segments.len() - keep..].join("/"))
}

/// Lexically join `base` + `relative`, resolving `.` / `..` without touching
/// the filesystem. Shared by file-link resolution and run-activity display.
pub fn join_path_lexical(base: &str, relative: &str) -> String {
    let base = base.trim_end_matches('/');
    let relative = relative.trim_start_matches('/');
    let mut parts: Vec<&str> = base.split('/').collect();
    for seg in relative.split('/') {
        match seg {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            s => parts.push(s),
        }
    }
    parts.join("/")
}

/// Extract the last path component from a directory path and capitalize its
/// first ASCII character for display (e.g. `"/Users/foo/my-project"` -> `"My-project"`).
/// Falls back to `"Workspace"` if the path is empty or has no valid component.
pub fn format_folder_display_name(path: &str) -> String {
    let mut name = path
        .rsplit(['/', '\\'])
        .find(|s| !s.is_empty())
        .unwrap_or("Workspace")
        .to_string();
    if let Some(first) = name.get_mut(..1) {
        first.make_ascii_uppercase();
    }
    name
}
