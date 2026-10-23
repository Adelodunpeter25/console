//! File-path link helpers: telling files from URLs and resolving them.

use console_shared::markdown::file_links::{
    is_file_link, is_http_url, is_known_bare_name, resolve_file_link, strip_line_suffix,
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
