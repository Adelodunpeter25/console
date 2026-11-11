//! Unit tests for the transcript scroll anchor.
//!
//! The anchor names a message, not a row: a row index counts rows of the
//! window that happens to be loaded, and that window moves as older pages
//! are prepended and shrinks when a chat is reopened. These tests pin the
//! resolution rules the app relies on when it restores a position.

use console_core::AgentMessage;
use console_ui::chat::{anchor_row, message_id_at, resolve_anchor_row, row_for_message_id};

fn user(id: Option<&str>) -> AgentMessage {
    AgentMessage::User {
        id: id.map(str::to_string),
        content: "hello".to_string(),
        attachments: None,
        context_files: None,
        annotations: None,
        created_at: None,
        approval_mode: None,
    }
}

fn assistant(id: Option<&str>) -> AgentMessage {
    AgentMessage::Assistant {
        id: id.map(str::to_string),
        content: Vec::new(),
        stop_reason: None,
        created_at: None,
        tok_per_sec: None,
    }
}

fn tool_result() -> AgentMessage {
    AgentMessage::ToolResult {
        results: Vec::new(),
        created_at: None,
    }
}

#[test]
fn message_id_is_read_from_the_row() {
    let messages = vec![user(Some("u1")), assistant(Some("a1")), tool_result()];
    assert_eq!(message_id_at(&messages, 0).as_deref(), Some("u1"));
    assert_eq!(message_id_at(&messages, 1).as_deref(), Some("a1"));
    // A tool result belongs to the tool call above it and has no id.
    assert_eq!(message_id_at(&messages, 2), None);
    assert_eq!(message_id_at(&messages, 9), None);
}

#[test]
fn a_message_without_an_id_has_none() {
    let messages = vec![user(None), assistant(None)];
    assert_eq!(message_id_at(&messages, 0), None);
}

#[test]
fn row_is_found_by_message_id() {
    let messages = vec![user(Some("u1")), assistant(Some("a1")), tool_result()];
    assert_eq!(row_for_message_id(&messages, "u1"), Some(0));
    assert_eq!(row_for_message_id(&messages, "a1"), Some(1));
    assert_eq!(row_for_message_id(&messages, "missing"), None);
}

#[test]
fn an_anchor_resolves_to_the_row_of_its_message() {
    let messages = vec![user(Some("u1")), assistant(Some("a1")), tool_result()];
    // The window the anchor was taken from had the message on row 0; the
    // window it is restored into has it on row 1.
    assert_eq!(resolve_anchor_row(&messages, Some("a1"), 0), Some(1));
}

#[test]
fn an_anchor_whose_message_is_gone_cannot_be_resolved() {
    let messages = vec![user(Some("u1")), assistant(Some("a1"))];
    // The message fell outside the fetched window, so there is no honest
    // row to land on. The caller falls back to the tail.
    assert_eq!(resolve_anchor_row(&messages, Some("gone"), 0), None);
}

#[test]
fn an_anchor_without_a_message_id_falls_back_to_the_row_index() {
    let messages = vec![user(None), assistant(None), user(None)];
    assert_eq!(resolve_anchor_row(&messages, None, 2), Some(2));
    // Clamped to the last row when the index is past the end.
    assert_eq!(resolve_anchor_row(&messages, None, 99), Some(2));
    assert_eq!(resolve_anchor_row(&messages, None, 0), Some(0));
}

#[test]
fn the_streaming_placeholder_is_not_a_anchor_row() {
    // While a run streams the list has one more row than it has messages:
    // the trailing "Working…" indicator. An index pointing at it would point
    // past the end once the row is gone.
    assert_eq!(anchor_row(3, 3), 2);
    assert_eq!(anchor_row(3, 1), 1);
    assert_eq!(anchor_row(0, 0), 0);
}
