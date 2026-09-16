//! Path and folder display utilities.

/// Basename of a path, handling both separator styles.
pub fn base_name(path: &str) -> &str {
    path.rsplit(['/', '\\']).next().unwrap_or(path)
}

/// Parent directory of a path, handling both separator styles.
/// Returns `""` for bare filenames.
pub fn parent_dir(path: &str) -> &str {
    match path.rsplit_once(['/', '\\']) {
        Some((parent, _)) => parent,
        None => "",
    }
}

/// Shorten a parent dir for narrow rows: strip a shared workspace prefix and
/// keep the trailing segments that actually disambiguate same-name files.
/// `apps/desktop/crates/console-ui/src/markdown` → `…/console-ui/src/markdown`.
pub fn short_parent_dir(path: &str) -> String {
    let parent = parent_dir(path);
    if parent.is_empty() {
        return String::new();
    }
    const SKIP_PREFIXES: [&str; 2] = ["apps/", "packages/"];
    let mut rest = parent;
    for prefix in SKIP_PREFIXES {
        if let Some(stripped) = rest.strip_prefix(prefix) {
            rest = stripped;
            break;
        }
    }
    const KEEP_SEGMENTS: usize = 3;
    let segments: Vec<&str> = rest.split(['/', '\\']).filter(|s| !s.is_empty()).collect();
    if segments.len() <= KEEP_SEGMENTS || rest.len() <= 28 {
        return rest.replace('\\', "/");
    }
    format!(
        "…/{}",
        segments[segments.len() - KEEP_SEGMENTS..].join("/")
    )
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
