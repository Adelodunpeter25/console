//! Unit tests for shared path display helpers.

use console_ui::utils::{
    ParentDirDisplay, parent_dir, short_parent_dir, short_parent_dir_with,
};

#[test]
fn parent_dir_handles_both_separators_and_bare_names() {
    assert_eq!(parent_dir("a/b/c.rs"), "a/b");
    assert_eq!(parent_dir("a\\b\\c.rs"), "a\\b");
    assert_eq!(parent_dir("file.rs"), "");
}

#[test]
fn short_parent_dir_returns_empty_for_bare_filenames() {
    assert_eq!(short_parent_dir("file.rs"), "");
}

#[test]
fn short_parent_dir_keeps_short_parents_whole() {
    assert_eq!(short_parent_dir("src/file.rs"), "src");
    assert_eq!(short_parent_dir("apps/mobile/src/file.rs"), "apps/mobile/src");
}

#[test]
fn short_parent_dir_truncates_long_parents_to_trailing_segments() {
    assert_eq!(
        short_parent_dir("apps/desktop/crates/console-ui/src/markdown/file.rs"),
        "…/console-ui/src/markdown"
    );
}

#[test]
fn short_parent_dir_normalizes_backslashes() {
    assert_eq!(
        short_parent_dir("a\\very\\long\\parent\\directory\\chain\\here\\file.rs"),
        "…/directory/chain/here"
    );
}

#[test]
fn short_parent_dir_with_honors_custom_tuning() {
    let display = ParentDirDisplay {
        keep_segments: 1,
        max_len: 0,
    };
    assert_eq!(
        short_parent_dir_with("a/b/c/file.rs", &display),
        "…/c"
    );
}
