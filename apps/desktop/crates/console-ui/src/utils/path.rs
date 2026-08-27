//! Path and folder display utilities.

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
