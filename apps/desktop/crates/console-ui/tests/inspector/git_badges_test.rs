//! Git badges for the Files tree: per-file codes and folder aggregation.

use console_core::types::GitFileEntry;
use console_ui::{GitBadge, aggregate_git_badges};

fn change(path: &str, status: &str) -> GitFileEntry {
    GitFileEntry {
        path: path.into(),
        status: status.into(),
        staged: false,
        additions: None,
        deletions: None,
    }
}

#[test]
fn codes_map_to_badges_and_unknown_codes_to_none() {
    assert_eq!(GitBadge::from_code("M"), Some(GitBadge::Modified));
    assert_eq!(GitBadge::from_code("?"), Some(GitBadge::Untracked));
    assert_eq!(GitBadge::from_code("U"), Some(GitBadge::Conflict));
    assert_eq!(GitBadge::from_code("!"), None);
    assert_eq!(GitBadge::from_code(""), None);
}

#[test]
fn every_ancestor_of_a_changed_file_carries_its_badge() {
    let badges = aggregate_git_badges(&[change("/repo/apps/desktop/src/main.rs", "M")]);
    for path in [
        "/repo/apps/desktop/src/main.rs",
        "/repo/apps/desktop/src",
        "/repo/apps/desktop",
        "/repo/apps",
        "/repo",
    ] {
        assert_eq!(badges.get(path), Some(&GitBadge::Modified), "{path}");
    }
    assert!(
        !badges.contains_key("/"),
        "the filesystem root is not a folder in the tree"
    );
}

#[test]
fn a_folder_shows_the_strongest_badge_beneath_it() {
    let badges = aggregate_git_badges(&[
        change("/r/a/new.rs", "?"),
        change("/r/a/edit.rs", "M"),
        change("/r/a/gone.rs", "D"),
    ]);
    assert_eq!(badges["/r/a"], GitBadge::Deleted);
    assert_eq!(badges["/r"], GitBadge::Deleted);
    // Each file keeps its own.
    assert_eq!(badges["/r/a/new.rs"], GitBadge::Untracked);
    assert_eq!(badges["/r/a/edit.rs"], GitBadge::Modified);
}

#[test]
fn result_does_not_depend_on_change_order() {
    let forward = [
        change("/r/a/x.rs", "?"),
        change("/r/a/b/y.rs", "M"),
        change("/r/c/z.rs", "D"),
    ];
    let mut reversed = forward.to_vec();
    reversed.reverse();
    assert_eq!(
        aggregate_git_badges(&forward),
        aggregate_git_badges(&reversed)
    );
}

#[test]
fn a_stronger_change_deep_down_still_reaches_the_top() {
    // The shallow file is added first and marks the shared ancestors weakly;
    // the deeper deletion must still raise all of them.
    let badges = aggregate_git_badges(&[change("/r/a/x.rs", "?"), change("/r/a/b/c/y.rs", "D")]);
    assert_eq!(badges["/r/a/b/c"], GitBadge::Deleted);
    assert_eq!(badges["/r/a/b"], GitBadge::Deleted);
    assert_eq!(badges["/r/a"], GitBadge::Deleted);
    assert_eq!(badges["/r"], GitBadge::Deleted);
}

#[test]
fn unknown_status_codes_are_ignored_entirely() {
    let badges = aggregate_git_badges(&[change("/r/a/x.rs", "!")]);
    assert!(badges.is_empty());
}

#[test]
fn no_changes_means_no_badges() {
    assert!(aggregate_git_badges(&[]).is_empty());
}
