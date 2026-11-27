//! Git badges for the Files tree: per-file codes, folder aggregation, and the
//! label and role each client draws them with.
//!
//! These rules are shared by the desktop and Android, so they are pinned here
//! rather than in either client.

use console_shared::git_badges::{ChangedFile, GitBadge, GitBadgeRole, aggregate_git_badges};

fn change<'a>(path: &'a str, status: &'a str) -> ChangedFile<'a> {
    ChangedFile { path, status }
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

#[test]
fn every_badge_carries_a_label_and_a_role() {
    assert_eq!(GitBadge::Untracked.label(), "U");
    assert_eq!(GitBadge::Added.label(), "A");
    assert_eq!(GitBadge::Renamed.label(), "R");
    assert_eq!(GitBadge::Modified.label(), "M");
    assert_eq!(GitBadge::Deleted.label(), "D");
    assert_eq!(GitBadge::Conflict.label(), "!");
}

#[test]
fn roles_say_how_loudly_a_badge_reports_itself() {
    assert_eq!(GitBadge::Untracked.role(), GitBadgeRole::Success);
    assert_eq!(GitBadge::Added.role(), GitBadgeRole::Success);
    assert_eq!(GitBadge::Renamed.role(), GitBadgeRole::Muted);
    assert_eq!(GitBadge::Modified.role(), GitBadgeRole::Warning);
    assert_eq!(GitBadge::Deleted.role(), GitBadgeRole::Danger);
    assert_eq!(GitBadge::Conflict.role(), GitBadgeRole::Danger);
}
