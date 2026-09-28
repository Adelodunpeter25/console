//! Tests for the floating composer's selection types.
//!
//! The card itself needs a gpui window to exercise; these cover the pure
//! decisions that decide what a launch actually creates.

use console_ui::common::BranchChoice;

#[test]
fn default_branch_choice_is_the_project_itself() {
    // Nothing selected means "don't provision anything" — the server only
    // creates a worktree when asked.
    assert_eq!(BranchChoice::default(), BranchChoice::Project);
}

#[test]
fn branch_labels_distinguish_the_three_modes() {
    assert_eq!(BranchChoice::Project.label(), "Project branch");
    assert_eq!(BranchChoice::NewWorktree.label(), "New worktree");
    assert_eq!(
        BranchChoice::FromBranch("main".into()).label(),
        "main",
        "an existing branch is shown by its own name"
    );
}

#[test]
fn existing_branch_choice_is_not_the_default() {
    // Guards the `show()` reset: an open must never inherit the last
    // launch's base branch.
    assert_ne!(
        BranchChoice::FromBranch("main".into()),
        BranchChoice::default()
    );
}
