//! Path and folder display utilities.

/// Extract the last path component from a directory path and capitalize its
/// first ASCII character for display (e.g. `"/Users/foo/my-project"` -> `"My-project"`).
/// Falls back to `"Workspace"` if the path is empty or has no valid component.
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
