// The sidebar keys on `sort_timestamp()`, i.e. last activity. Reading a
// session no longer bumps updated_at on the server, so selecting a row does
// not reshuffle the list.
use console_core::{SessionHeader, SessionHeaderExt};

#[test]
fn sort_timestamp_is_last_activity() {
    let header = SessionHeader {
        created_at: 1_700_000_000_000,
        updated_at: 1_700_000_999_999,
        ..Default::default()
    };
    assert_eq!(header.sort_timestamp(), header.updated_at);
}

#[test]
fn sort_timestamp_falls_back_to_creation_time() {
    let header = SessionHeader {
        created_at: 9_000,
        updated_at: 0,
        ..Default::default()
    };
    assert_eq!(header.sort_timestamp(), header.created_at);
}
