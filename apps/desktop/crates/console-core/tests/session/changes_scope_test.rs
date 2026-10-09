use console_core::types::{ChangesScope, SessionFileChange, filter_changes_for_scope};

fn change(path: &str, turn_index: u32) -> SessionFileChange {
    SessionFileChange {
        path: path.into(),
        turn_index,
        ..Default::default()
    }
}

fn paths(changes: &[SessionFileChange]) -> Vec<&str> {
    changes.iter().map(|c| c.path.as_str()).collect()
}

#[test]
fn previous_turn_skips_turns_without_changes() {
    // Turn 3 edited nothing, so "previous" before turn 4 is turn 2.
    let changes = vec![change("d.rs", 4), change("b.rs", 2), change("a.rs", 0)];
    let previous = filter_changes_for_scope(&changes, ChangesScope::PreviousTurn);
    assert_eq!(paths(&previous), vec!["b.rs"]);
}

#[test]
fn this_turn_is_latest_turn() {
    let changes = vec![change("d.rs", 4), change("e.rs", 4), change("b.rs", 2)];
    let this_turn = filter_changes_for_scope(&changes, ChangesScope::ThisTurn);
    assert_eq!(paths(&this_turn), vec!["d.rs", "e.rs"]);
}

#[test]
fn previous_turn_empty_with_single_turn() {
    let changes = vec![change("a.rs", 0), change("b.rs", 0)];
    assert!(filter_changes_for_scope(&changes, ChangesScope::PreviousTurn).is_empty());
}
