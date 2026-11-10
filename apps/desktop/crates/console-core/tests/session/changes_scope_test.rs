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

#[test]
fn all_turns_aggregates_additions_and_deletions_across_turns() {
    let changes = vec![
        SessionFileChange {
            path: "runtime.go".into(),
            turn_index: 2,
            status: "modified".into(),
            additions: 10,
            deletions: 5,
            ..Default::default()
        },
        SessionFileChange {
            path: "args.go".into(),
            turn_index: 2,
            status: "added".into(),
            additions: 20,
            deletions: 0,
            ..Default::default()
        },
        SessionFileChange {
            path: "runtime.go".into(),
            turn_index: 1,
            status: "added".into(),
            additions: 35,
            deletions: 12,
            ..Default::default()
        },
    ];

    let all = filter_changes_for_scope(&changes, ChangesScope::AllTurns);
    assert_eq!(paths(&all), vec!["runtime.go", "args.go"]);

    let runtime = &all[0];
    assert_eq!(runtime.path, "runtime.go");
    assert_eq!(runtime.additions, 45);
    assert_eq!(runtime.deletions, 17);
    assert_eq!(runtime.status, "added");

    let args = &all[1];
    assert_eq!(args.path, "args.go");
    assert_eq!(args.additions, 20);
    assert_eq!(args.deletions, 0);

    let total_additions: u32 = all.iter().map(|c| c.additions).sum();
    let total_deletions: u32 = all.iter().map(|c| c.deletions).sum();
    assert_eq!(total_additions, 65);
    assert_eq!(total_deletions, 17);
}
