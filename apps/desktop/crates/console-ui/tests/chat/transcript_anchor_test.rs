//! Unit tests for the transcript scroll anchor.
//!
//! The anchor names a turn, not a row: a row index counts rows of the
//! window that happens to be loaded, and that window moves as older pages
//! are prepended and shrinks when a chat is reopened.

use console_core::Turn;
use console_ui::chat::{anchor_row, message_id_at, resolve_anchor_row, row_for_message_id};

fn turn(id: &str) -> Turn {
    Turn {
        id: id.to_string(),
        created_at: None,
        user: None,
        wake_label: None,
        run: None,
        responses: Vec::new(),
    }
}

#[test]
fn turn_id_is_read_from_the_row() {
    let turns = vec![turn("t1"), turn("t2"), turn("")];
    assert_eq!(message_id_at(&turns, 0).as_deref(), Some("t1"));
    assert_eq!(message_id_at(&turns, 1).as_deref(), Some("t2"));
    assert_eq!(message_id_at(&turns, 2), None);
    assert_eq!(message_id_at(&turns, 9), None);
}

#[test]
fn row_is_found_by_turn_id() {
    let turns = vec![turn("t1"), turn("t2")];
    assert_eq!(row_for_message_id(&turns, "t1"), Some(0));
    assert_eq!(row_for_message_id(&turns, "t2"), Some(1));
    assert_eq!(row_for_message_id(&turns, "missing"), None);
}

#[test]
fn an_anchor_resolves_to_the_row_of_its_turn() {
    let turns = vec![turn("t1"), turn("t2")];
    assert_eq!(resolve_anchor_row(&turns, Some("t2"), 0), Some(1));
}

#[test]
fn an_anchor_whose_turn_is_gone_cannot_be_resolved() {
    let turns = vec![turn("t1"), turn("t2")];
    assert_eq!(resolve_anchor_row(&turns, Some("gone"), 0), None);
}

#[test]
fn an_anchor_without_a_turn_id_falls_back_to_the_row_index() {
    let turns = vec![turn(""), turn(""), turn("")];
    assert_eq!(resolve_anchor_row(&turns, None, 2), Some(2));
    assert_eq!(resolve_anchor_row(&turns, None, 99), Some(2));
    assert_eq!(resolve_anchor_row(&turns, None, 0), Some(0));
}

#[test]
fn the_streaming_placeholder_is_not_an_anchor_row() {
    assert_eq!(anchor_row(3, 3), 2);
    assert_eq!(anchor_row(3, 1), 1);
    assert_eq!(anchor_row(0, 0), 0);
}
