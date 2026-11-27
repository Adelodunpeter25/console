//! Unit tests for the persisted transcript scroll positions.
//!
//! The scroll positions live in the state document next to the layout and
//! the workspace tree. The section is optional, so a document written before
//! it existed still has to parse — that is the whole reason it does not move
//! the document version.

use console_app::persistence::store::{
    PersistedTranscriptScroll, PersistedTranscriptScrollState, StorageDocument,
};

fn scroll_state() -> PersistedTranscriptScrollState {
    let mut positions = std::collections::HashMap::new();
    positions.insert(
        "session-a".to_string(),
        PersistedTranscriptScroll {
            message_id: Some("msg-42".to_string()),
            offset_in_row: 12.5,
            at_tail: false,
        },
    );
    positions.insert(
        "session-b".to_string(),
        PersistedTranscriptScroll {
            message_id: None,
            offset_in_row: 0.0,
            at_tail: true,
        },
    );
    PersistedTranscriptScrollState { positions }
}

#[test]
fn scroll_positions_round_trip_through_the_document() {
    let document = StorageDocument {
        transcript_scroll: Some(scroll_state()),
        ..Default::default()
    };
    let json = serde_json::to_string(&document).expect("document serializes");
    let parsed: StorageDocument = serde_json::from_str(&json).expect("document parses");

    let state = parsed.transcript_scroll.expect("scroll section survives");
    let a = state.positions.get("session-a").expect("session-a kept");
    assert_eq!(a.message_id.as_deref(), Some("msg-42"));
    assert_eq!(a.offset_in_row, 12.5);
    assert!(!a.at_tail);

    let b = state.positions.get("session-b").expect("session-b kept");
    assert_eq!(b.message_id, None);
    assert!(b.at_tail);
}

#[test]
fn a_document_without_the_section_still_parses() {
    let json = r#"{"version":2,"layout":{"sidebar_visible":true}}"#;
    let parsed: StorageDocument = serde_json::from_str(json).expect("document parses");
    assert!(parsed.transcript_scroll.is_none());
    // The section is optional, so the document version does not move for it.
    assert_eq!(parsed.version, 2);
}

#[test]
fn a_position_without_a_message_id_is_read_back_as_none() {
    let json = r#"{"version":2,"transcript_scroll":{"positions":{"s":{"at_tail":true}}}}"#;
    let parsed: StorageDocument = serde_json::from_str(json).expect("document parses");
    let state = parsed.transcript_scroll.expect("scroll section survives");
    let position = state.positions.get("s").expect("position kept");
    assert_eq!(position.message_id, None);
    assert!(position.at_tail);
}
