//! File-path link helpers for clickable transcript paths.
//!
//! The parser linkifies bare paths (`apps/server/index.ts`, `./foo.rs:12:3`,
//! `file://...`) with their own text as the destination. This module decides
//! whether a clicked link is a file (vs a browser URL), strips the `:line:col`
//! suffix for phase 1, and resolves relatives against the session cwd with a
//! pane project-path fallback.

/// True for `http(s)://` destinations, which keep opening in the browser.
pub fn is_http_url(link: &str) -> bool {
    let link = link.trim_start();
    link.len() >= 7
        && (link[..7].eq_ignore_ascii_case("http://")
            || (link.len() >= 8 && link[..8].eq_ignore_ascii_case("https://")))
}

/// True when `link` should open as a workspace file instead of a browser URL.
/// Covers `file://` URLs plus anything that looks like a path with a file
/// extension (bare linkified paths and `[label](path)` markdown destinations).
pub fn is_file_link(link: &str) -> bool {
    let link = link.trim();
    if link.is_empty() || is_http_url(link) {
        return false;
    }
    if link.starts_with("file://") {
        return true;
    }
    // Strip the :line:col suffix before checking for an extension.
    let path = strip_line_suffix(link);
    let file = path.rsplit('/').next().unwrap_or(path);
    let file = file.rsplit('\\').next().unwrap_or(file);
    match file.rfind('.') {
        Some(dot) if dot + 1 < file.len() => {
            let ext = &file[dot + 1..];
            !ext.is_empty()
                && ext.len() <= 10
                && ext.chars().all(|c| c.is_ascii_alphanumeric())
        }
        _ => false,
    }
}

/// Strip a trailing `:line` / `:line:col` suffix (`foo.ts:12:3` → `foo.ts`).
/// Phase 1 opens the file only; line jumping waits on viewer scroll support.
pub fn strip_line_suffix(link: &str) -> &str {
    let bytes = link.as_bytes();
    let mut end = link.len();
    // Optional :col
    let mut i = end;
    while i > 0 && bytes[i - 1].is_ascii_digit() {
        i -= 1;
    }
    if i < end && i > 0 && bytes[i - 1] == b':' && i != end {
        end = i - 1;
        // Optional :line (when both present, this is the line; else the single number)
        let mut j = end;
        while j > 0 && bytes[j - 1].is_ascii_digit() {
            j -= 1;
        }
        if j < end && j > 0 && bytes[j - 1] == b':' && j != end {
            end = j - 1;
        }
        link[..end].trim_end()
    } else {
        link.trim_end()
    }
}

/// Resolve a file link to an absolute path for `open_file_tab`.
/// Absolute paths pass through; relatives join the session cwd first, then the
/// pane project path. `~` expands against `$HOME`.
pub fn resolve_file_link(link: &str, cwd: Option<&str>, project_path: Option<&str>) -> String {
    let mut raw = link.trim().to_owned();
    if raw.starts_with("file://") {
        raw = raw.trim_start_matches("file://").to_owned();
        if let Some(stripped) = raw.strip_prefix("localhost") {
            raw = stripped.to_owned();
        }
    }
    let mut path = strip_line_suffix(&raw).trim().to_owned();
    // Percent-decode minimal: %20 → space (common from file:// URLs).
    if path.contains('%') {
        path = percent_decode(&path);
    }
    if path.starts_with("~/") {
        if let Ok(home) = std::env::var("HOME") {
            return format!("{}{}", home, &path[1..]);
        }
        return path;
    }
    if path.starts_with('/') || path.starts_with('\\') {
        return path;
    }
    // Strip ./ prefix for clean joins.
    let relative = path.strip_prefix("./").unwrap_or(&path);
    if let Some(cwd) = cwd.filter(|c| !c.is_empty()) {
        return join_path(cwd, relative);
    }
    if let Some(root) = project_path.filter(|p| !p.is_empty()) {
        return join_path(root, relative);
    }
    relative.to_owned()
}

fn join_path(base: &str, relative: &str) -> String {
    let base = base.trim_end_matches('/');
    let relative = relative.trim_start_matches('/');
    // Resolve ../ segments lexically without touching the filesystem.
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

fn percent_decode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    let mut chars = s.chars();
    while let Some(c) = chars.next() {
        if c == '%' {
            let hi = chars.next().and_then(|c| c.to_digit(16));
            let lo = chars.next().and_then(|c| c.to_digit(16));
            match (hi, lo) {
                (Some(h), Some(l)) => out.push(char::from_u32(h * 16 + l).unwrap_or('\u{FFFD}')),
                _ => {
                    out.push('%');
                    if let Some(h) = hi {
                        out.push(char::from_u32(h).unwrap_or('\u{FFFD}'));
                    }
                }
            }
        } else {
            out.push(c);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn http_stays_browser() {
        assert!(is_http_url("https://example.com/x"));
        assert!(!is_file_link("https://example.com/x"));
    }

    #[test]
    fn detects_paths() {
        assert!(is_file_link("apps/server/index.ts"));
        assert!(is_file_link("./src/foo.rs:12:3"));
        assert!(is_file_link("file:///tmp/a.md"));
        assert!(!is_file_link("just words"));
    }

    #[test]
    fn strips_lines() {
        assert_eq!(strip_line_suffix("a/b.ts:12:3"), "a/b.ts");
        assert_eq!(strip_line_suffix("a/b.ts:12"), "a/b.ts");
        assert_eq!(strip_line_suffix("a/b.ts"), "a/b.ts");
    }

    #[test]
    fn resolves_against_cwd() {
        assert_eq!(
            resolve_file_link("apps/a.ts", Some("/proj"), None),
            "/proj/apps/a.ts"
        );
        assert_eq!(resolve_file_link("/abs/a.ts", Some("/proj"), None), "/abs/a.ts");
        assert_eq!(
            resolve_file_link("a.ts:12", Some("/proj"), None),
            "/proj/a.ts"
        );
    }
}
