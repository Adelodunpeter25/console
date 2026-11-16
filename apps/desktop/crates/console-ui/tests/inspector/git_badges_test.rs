//! The Files tree draws the shared badge rules in the desktop's own palette.
//! The rules themselves are pinned in console-shared, which the Android client
//! reaches over JNI; this file covers the two things that stay here — that
//! the tree can still reach them, and how a role becomes a color.

use console_core::types::GitFileEntry;
use console_ui::theme::Theme;
use console_ui::{GitBadge, GitBadgeRole, aggregate_git_badges, git_badge_color};

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
fn the_tree_reaches_the_shared_rules() {
    let badges = aggregate_git_badges(&[change("/repo/src/main.rs", "M")]);
    assert_eq!(badges.get("/repo/src"), Some(&GitBadge::Modified));
    assert_eq!(badges.get("/repo"), Some(&GitBadge::Modified));
}

#[test]
fn a_role_becomes_the_desktops_own_color() {
    let theme = Theme::dark();
    assert_eq!(git_badge_color(GitBadge::Added, &theme), theme.success);
    assert_eq!(git_badge_color(GitBadge::Modified, &theme), theme.warning);
    assert_eq!(git_badge_color(GitBadge::Deleted, &theme), theme.danger);
    assert_eq!(
        git_badge_color(GitBadge::Renamed, &theme),
        theme.text_tertiary
    );
}

#[test]
fn the_roles_come_from_the_shared_rules() {
    assert_eq!(GitBadge::Untracked.role(), GitBadgeRole::Success);
    assert_eq!(GitBadge::Conflict.role(), GitBadgeRole::Danger);
}
