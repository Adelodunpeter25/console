// The sidebar keys on `sort_timestamp()`. It used to key on last activity,
// which let opening or running a session reshuffle the list.
use console_core::{SessionHeader, SessionHeaderExt};

#[test]
fn sort_timestamp_is_creation_time() {
    let header = SessionHeader {
        created_at: 1_700_000_000_000,
        updated_at: 1_700_000_999_999,
        ..Default::default()
    };
    assert_eq!(header.sort_timestamp(), header.created_at);
    assert_ne!(header.sort_timestamp(), header.updated_at);
}

#[test]
fn sort_timestamp_ignores_later_activity() {
    let before = SessionHeader {
        created_at: 500,
        updated_at: 600,
        ..Default::default()
    };
    let after = SessionHeader {
        created_at: 500,
        updated_at: 9_000,
        ..Default::default()
    };
    assert_eq!(before.sort_timestamp(), after.sort_timestamp());
}
