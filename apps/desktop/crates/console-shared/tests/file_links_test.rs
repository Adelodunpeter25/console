//! File-path link helpers: telling files from URLs and resolving them.

use console_shared::markdown::file_links::{
    closest_match, is_bare_file_name, is_file_link, is_http_url, is_known_bare_name,
    resolve_file_link, strip_line_suffix,
};

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
fn known_extensionless_names_are_files() {
    assert!(is_known_bare_name("Makefile"));
    assert!(is_file_link("LICENSE"));
    assert!(!is_file_link("hello"));
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
    assert_eq!(
        resolve_file_link("/abs/a.ts", Some("/proj"), None),
        "/abs/a.ts"
    );
    assert_eq!(
        resolve_file_link("a.ts:12", Some("/proj"), None),
        "/proj/a.ts"
    );
}

#[test]
fn bare_names_have_no_directory() {
    assert!(is_bare_file_name("file_tree.rs"));
    assert!(is_bare_file_name("file_tree.rs:12:3"));
    assert!(is_bare_file_name("Makefile"));
    assert!(!is_bare_file_name("src/file_tree.rs"));
    assert!(!is_bare_file_name("./file_tree.rs"));
    assert!(!is_bare_file_name("/abs/file_tree.rs"));
    assert!(!is_bare_file_name("~/file_tree.rs"));
    assert!(!is_bare_file_name("file:///tmp/a.md"));
    assert!(!is_bare_file_name(""));
}

#[test]
fn the_closest_match_to_the_cwd_wins() {
    let candidates = vec![
        "/p/apps/android/mod.rs".to_string(),
        "/p/apps/desktop/crates/console-ui/src/mod.rs".to_string(),
        "/p/apps/desktop/src/mod.rs".to_string(),
    ];
    // Same cwd subtree beats the others even though it is deeper.
    assert_eq!(
        closest_match(Some("/p/apps/desktop/crates/console-ui"), &candidates),
        Some("/p/apps/desktop/crates/console-ui/src/mod.rs")
    );
    assert_eq!(
        closest_match(Some("/p/apps/desktop"), &candidates),
        Some("/p/apps/desktop/src/mod.rs")
    );
}

#[test]
fn ties_prefer_shallower_then_alphabetical() {
    let candidates = vec![
        "/p/b/deep/er/x.rs".to_string(),
        "/p/z/x.rs".to_string(),
        "/p/a/x.rs".to_string(),
    ];
    assert_eq!(closest_match(Some("/p"), &candidates), Some("/p/a/x.rs"));
    assert_eq!(closest_match(None, &candidates), Some("/p/a/x.rs"));
}

#[test]
fn no_candidates_means_no_match() {
    assert_eq!(closest_match(Some("/p"), &[]), None);
}
