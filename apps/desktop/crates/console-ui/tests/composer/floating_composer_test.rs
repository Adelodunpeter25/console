//! Tests for the floating composer's selection types.
//!
//! The card itself needs a gpui window to exercise; these cover the pure
//! decisions that decide what a launch actually creates.

use console_core::SlashCommandInfo;
use console_ui::common::{
    AutocompleteItem, AutocompleteView, BranchChoice, PendingPaste, resolve_pending_pastes,
};

fn pending(id: usize, lines: usize, text: &str) -> PendingPaste {
    PendingPaste {
        path: format!("pending-paste:{id}"),
        label: format!("pasted {lines} lines"),
        text: text.to_string(),
    }
}

#[test]
fn uploaded_paste_swaps_placeholder_for_server_path() {
    let files = vec!["src/main.rs".to_string(), "pending-paste:1".to_string()];
    let resolved = vec![(
        pending(1, 30, "raw"),
        Some("/p/pastes/s/pasted-1.txt".into()),
    )];
    let (prompt, files) =
        resolve_pending_pastes("see main.rs and pasted 30 lines please", &files, &resolved);
    assert_eq!(prompt, "see main.rs and pasted 30 lines please");
    assert_eq!(files, vec!["src/main.rs", "/p/pastes/s/pasted-1.txt"]);
}

#[test]
fn failed_paste_goes_inline_and_keeps_later_pastes_aligned() {
    let files = vec!["pending-paste:1".to_string(), "pending-paste:2".to_string()];
    let resolved = vec![
        (pending(1, 30, "RAW ONE"), None),
        (
            pending(2, 30, "RAW TWO"),
            Some("/p/pastes/s/pasted-2.txt".into()),
        ),
    ];
    let (prompt, files) =
        resolve_pending_pastes("pasted 30 lines then pasted 30 lines", &files, &resolved);
    assert_eq!(prompt, "RAW ONE then pasted 30 lines");
    assert_eq!(files, vec!["/p/pastes/s/pasted-2.txt"]);
}

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
fn project_menu_section_renders_no_project_row() {
    use console_core::ProjectInfo;
    use console_ui::common::{ProjectMenuExtras, project_menu_section};
    use console_ui::primitives::MenuItem;
    use std::rc::Rc;

    let projects = Rc::new(vec![ProjectInfo {
        id: "p1".into(),
        name: "Project 1".into(),
        path: "/path/to/p1".into(),
        created_at: 0,
        updated_at: 0,
    }]);

    let on_select: Rc<dyn Fn(String, &mut gpui::Window, &mut gpui::App)> = Rc::new(|_, _, _| {});
    let on_new: Rc<dyn Fn(&mut gpui::Window, &mut gpui::App)> = Rc::new(|_, _| {});
    let on_none: Rc<dyn Fn(&mut gpui::Window, &mut gpui::App)> = Rc::new(|_, _| {});

    let items = project_menu_section(
        projects,
        None,
        true,
        &on_select,
        Some(&ProjectMenuExtras {
            on_new_project: on_new,
            on_no_project: on_none,
            none_selected: true,
        }),
    );

    let no_project_item = items.iter().find(|item| match item {
        MenuItem::Entry { label, .. } => label == "No project",
        _ => false,
    });

    assert!(
        no_project_item.is_some(),
        "No project item should be present"
    );
    if let Some(MenuItem::Entry { selected, .. }) = no_project_item {
        assert!(
            *selected,
            "No project should be marked selected when none_selected is true"
        );
    }
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

// Only "New worktree" provisions a worktree. Picking an existing branch (even
// `main`) works in the project folder, so it must never create one.
#[test]
fn only_new_worktree_creates_a_worktree() {
    assert!(!BranchChoice::Project.creates_worktree());
    assert!(BranchChoice::NewWorktree.creates_worktree());
    assert!(!BranchChoice::FromBranch("main".into()).creates_worktree());
}

#[test]
fn picking_an_existing_branch_checks_it_out_in_the_project() {
    assert_eq!(
        BranchChoice::FromBranch("main".into()).checkout_target(),
        Some("main")
    );
    assert_eq!(BranchChoice::Project.checkout_target(), None);
    assert_eq!(BranchChoice::NewWorktree.checkout_target(), None);
}

#[test]
fn branch_menu_section_offers_no_remote_branches() {
    // Remote-tracking rows are worktree base choices, not checkouts —
    // `git switch` cannot check them out in place, so the shared section
    // (the footer and the card's FromBranch mode) must hide them.
    use console_core::GitBranchInfo;
    use console_ui::common::branch_menu_section;
    use console_ui::primitives::MenuItem;
    use std::rc::Rc;

    let branches = Rc::new(vec![
        GitBranchInfo {
            name: "main".into(),
            current: true,
            remote: false,
        },
        GitBranchInfo {
            name: "origin/develop".into(),
            current: false,
            remote: true,
        },
    ]);
    let is_selected: Rc<dyn Fn(&GitBranchInfo) -> bool> = Rc::new(|info: &GitBranchInfo| {
        BranchChoice::FromBranch(info.name.clone()) == BranchChoice::FromBranch("main".into())
    });
    let on_select: Rc<dyn Fn(String, &mut gpui::Window, &mut gpui::App)> = Rc::new(|_, _, _| {});

    let items = branch_menu_section(branches, &is_selected, &on_select, true, &[], None);
    let labels: Vec<&str> = items
        .iter()
        .filter_map(|item| match item {
            MenuItem::Entry { label, .. } => Some(label.as_str()),
            _ => None,
        })
        .collect();
    assert!(labels.contains(&"main"), "labels: {labels:?}");
    assert!(
        !labels.iter().any(|label| label.starts_with("origin/")),
        "remote rows must not be checkout targets: {labels:?}"
    );
}
