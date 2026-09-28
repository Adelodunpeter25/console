//! Tests for the floating composer's selection types.
//!
//! The card itself needs a gpui window to exercise; these cover the pure
//! decisions that decide what a launch actually creates.

use console_core::SlashCommandInfo;
use console_ui::common::{AutocompleteItem, AutocompleteView, BranchChoice};

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

#[test]
fn autocomplete_content_key_is_stable_for_equal_content() {
    // The card is handed a freshly built popup every frame and skips the write
    // when the key matches, so equal content must hash equal — otherwise the
    // app re-renders forever and runs out of memory.
    let one = view_of(&[("console", "one"), ("src/main.rs", "two")]);
    let same = view_of(&[("console", "one"), ("src/main.rs", "two")]);
    assert_eq!(one.content_key(), same.content_key());
}

#[test]
fn autocomplete_content_key_ignores_the_popup_itself() {
    // Two independent builds of the same rows must compare equal even though
    // they are distinct `AutocompleteView` values.
    let a = view_of(&[("console", "one")]);
    let b = view_of(&[("console", "one")]);
    assert_eq!(a.content_key(), b.content_key());
}

#[test]
fn autocomplete_content_key_changes_with_content() {
    let base = view_of(&[("console", "one")]);
    assert_ne!(
        base.content_key(),
        view_of(&[("other", "one")]).content_key()
    );
    // Same count, different paths: a new query often returns the same number
    // of rows, so a length-only key would miss the swap.
    assert_ne!(
        view_of(&[("a", "one"), ("b", "two")]).content_key(),
        view_of(&[("a", "one"), ("c", "two")]).content_key()
    );
    // Highlight and loading also change what the user sees.
    assert_ne!(
        base.content_key(),
        view_highlighted(&[("a", "one")], 1).content_key()
    );
    assert_ne!(
        base.content_key(),
        view_loading(&[("a", "one")]).content_key()
    );
}

#[test]
fn empty_autocomplete_pops_share_a_key() {
    // `None` is the steady state before typing; it must compare equal to
    // itself or opening the card would ping a notify every frame.
    let empty = AutocompleteView::new(Vec::new(), 0, false);
    assert_eq!(empty.content_key(), empty.content_key());
}

/// Build a popup of command rows. The `ComposerInput` isn't needed —
/// `content_key` only reads the items and the highlight/loading flags.
fn view_of(rows: &[(&str, &str)]) -> AutocompleteView {
    view_with(rows, 0, false)
}

fn view_highlighted(rows: &[(&str, &str)], highlighted: usize) -> AutocompleteView {
    view_with(rows, highlighted, false)
}

fn view_loading(rows: &[(&str, &str)]) -> AutocompleteView {
    view_with(rows, 0, true)
}

fn view_with(rows: &[(&str, &str)], highlighted: usize, loading: bool) -> AutocompleteView {
    let items = rows
        .iter()
        .map(|(name, description)| {
            AutocompleteItem::Command(SlashCommandInfo {
                name: (*name).to_owned(),
                description: (*description).to_owned(),
                builtin: false,
            })
        })
        .collect();
    AutocompleteView::new(items, highlighted, loading)
}
