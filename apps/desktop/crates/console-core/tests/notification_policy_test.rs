//! Pure notification-policy tests (no macOS center touched).
//!
//! Covers the helpers behind the objc2 banner path: stable per-session
//! identifiers (replace-not-stack), ident parsing (click routing),
//! title fallback, and the viewing-suppression decision.

use console_core::types::notification::{
    NOTIFICATION_THREAD_ID, NotificationDecision, decide_notification,
    normalize_notification_title, notification_ident, parse_session_id_from_ident,
};

#[test]
fn test_ident_is_stable_per_session() {
    let a = notification_ident("abc-123");
    let b = notification_ident("abc-123");
    assert_eq!(a, b);
    assert_eq!(a, "console-session-abc-123");
    // Different sessions must not share an identifier (else banners replace each other).
    assert_ne!(a, notification_ident("other-session"));
}

#[test]
fn test_parse_round_trip() {
    let sid = "sess-42";
    let ident = notification_ident(sid);
    assert_eq!(parse_session_id_from_ident(&ident).as_deref(), Some(sid));
}

#[test]
fn test_parse_rejects_bad_idents() {
    assert_eq!(parse_session_id_from_ident(""), None);
    assert_eq!(parse_session_id_from_ident("console-session-"), None);
    assert_eq!(parse_session_id_from_ident("other-prefix-abc"), None);
    assert_eq!(parse_session_id_from_ident("console-session"), None);
}

#[test]
fn test_title_fallback() {
    assert_eq!(normalize_notification_title(""), "Console");
    assert_eq!(
        normalize_notification_title("Console · Task Complete"),
        "Console · Task Complete"
    );
}

#[test]
fn test_decide_viewing_suppresses() {
    assert_eq!(
        decide_notification(Some("s1"), "s1"),
        NotificationDecision::SuppressViewing
    );
}

#[test]
fn test_decide_background_notifies() {
    assert_eq!(
        decide_notification(None, "s1"),
        NotificationDecision::Notify
    );
    assert_eq!(
        decide_notification(Some("other"), "s1"),
        NotificationDecision::Notify
    );
}

#[test]
fn test_decide_empty_skips() {
    assert_eq!(
        decide_notification(None, ""),
        NotificationDecision::SkipEmpty
    );
    assert_eq!(
        decide_notification(Some("s1"), ""),
        NotificationDecision::SkipEmpty
    );
}

#[test]
fn test_thread_groups_sessions() {
    // Single group keeps all session banners together in Notification Center.
    assert_eq!(NOTIFICATION_THREAD_ID, "console-sessions");
}
