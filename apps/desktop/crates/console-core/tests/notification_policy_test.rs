//! Pure notification-policy tests (no macOS center touched).
//!
//! Covers the helpers behind the objc2 banner path: stable per-session
//! identifiers (replace-not-stack), ident parsing (click routing),
//! title fallback, and the viewing-suppression decision.

use console_core::types::notification::{
    NOTIFICATION_THREAD_ID, NotificationDecision, badge_count, decide_notification,
    dock_badge_label, fresh_list_needs_read, needs_read_report, normalize_notification_title,
    notification_ident, parse_session_id_from_ident,
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

#[test]
fn test_dock_badge_label() {
    // Zero clears the badge; otherwise the outstanding-banner count.
    assert_eq!(dock_badge_label(0), None);
    assert_eq!(dock_badge_label(1).as_deref(), Some("1"));
    assert_eq!(dock_badge_label(12).as_deref(), Some("12"));
}

fn session(status: &str, unread: bool) -> console_core::SessionHeader {
    console_core::SessionHeader {
        status: status.to_string(),
        unread,
        ..Default::default()
    }
}

#[test]
fn badge_counts_unread_and_needs_attention_chats() {
    let sessions = vec![
        session("done", true),
        session("done", true),
        session("needs_attention", false),
        session("done", false),
        session("idle", false),
    ];
    assert_eq!(badge_count(&sessions), 3);
}

#[test]
fn badge_is_empty_when_everything_is_read() {
    assert_eq!(
        badge_count(&[session("done", false), session("idle", false)]),
        0
    );
    assert_eq!(badge_count(&[]), 0);
}

#[test]
fn badge_skips_running_settled_and_deleted_chats() {
    let mut settled = session("done", true);
    settled.settled_at = Some(1);
    let mut deleted = session("done", true);
    deleted.deleted_at = Some(1);
    let running = session("working", true);
    assert_eq!(badge_count(&[settled, deleted, running]), 0);
}

#[test]
fn an_unread_or_attention_chat_needs_a_read_report() {
    // The case that bit: a chat finished while the user was away, so it is
    // unread even though it was already open in its pane.
    assert!(needs_read_report(&session("done", true)));
    assert!(needs_read_report(&session("needs_attention", false)));
}

#[test]
fn a_read_chat_needs_no_report() {
    // Returning to the window, or reopening a chat from a banner, must not
    // send a request for a chat that is already read.
    assert!(!needs_read_report(&session("done", false)));
    assert!(!needs_read_report(&session("idle", false)));
    assert!(!needs_read_report(&session("working", false)));
}

fn named(id: &str, status: &str, unread: bool) -> console_core::SessionHeader {
    console_core::SessionHeader {
        id: id.to_string(),
        status: status.to_string(),
        unread,
        ..Default::default()
    }
}

#[test]
fn a_fresh_list_with_the_active_chat_unread_needs_a_read() {
    // The background-finish case: the server says unread, the app never knew.
    let fresh = vec![named("a", "done", true), named("b", "done", false)];
    assert!(fresh_list_needs_read(&fresh, Some("a"), true));
}

#[test]
fn only_the_active_chat_counts() {
    // Another chat's mark must survive: it has not been opened.
    let fresh = vec![named("a", "done", true), named("b", "done", true)];
    assert!(!fresh_list_needs_read(&fresh, Some("c"), true));
    assert!(fresh_list_needs_read(&fresh, Some("b"), true));
}

#[test]
fn an_unfocused_window_never_reads_anything() {
    let fresh = vec![named("a", "done", true)];
    assert!(!fresh_list_needs_read(&fresh, Some("a"), false));
}

#[test]
fn no_active_chat_or_a_read_active_chat_needs_no_read() {
    let fresh = vec![named("a", "done", false), named("b", "done", true)];
    assert!(!fresh_list_needs_read(&fresh, None, true));
    assert!(!fresh_list_needs_read(&fresh, Some("a"), true));
    assert!(!fresh_list_needs_read(&[], Some("a"), true));
}

#[test]
fn a_pending_question_on_the_active_chat_does_not_loop() {
    // The server keeps a chat `needs_attention` while its question is open,
    // even after it is read. The refresh rule must not treat that as unread,
    // or it would report, refresh, and report again for as long as the
    // question stays open.
    let fresh = vec![named("a", "needs_attention", false)];
    assert!(!fresh_list_needs_read(&fresh, Some("a"), true));
}

#[test]
fn an_errored_unread_active_chat_is_read_once() {
    // An errored run is `needs_attention` AND unread: the unread flag is what
    // triggers the read, and reading it clears both on the server.
    let fresh = vec![named("a", "needs_attention", true)];
    assert!(fresh_list_needs_read(&fresh, Some("a"), true));
}
