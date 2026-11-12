/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/notification.proto). `subtitle` omits when empty, as
/// the old omitempty did: an absent key means blank, i.e. a single-line
/// banner.
pub use console_proto::NotificationEvent;

/// Grouping key so session banners stack under one group in Notification Center.
pub const NOTIFICATION_THREAD_ID: &str = "console-sessions";

/// Stable per-session identifier so repeat events **replace** instead of stacking.
pub fn notification_ident(session_id: &str) -> String {
    format!("console-session-{session_id}")
}

/// Inverse of [`notification_ident`]: `console-session-<id>` -> `<id>`.
/// Returns `None` for wrong prefixes and empty ids.
pub fn parse_session_id_from_ident(ident: &str) -> Option<String> {
    ident
        .strip_prefix("console-session-")
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
}

/// How many chats the Dock badge counts: the ones that finished unseen, plus
/// the ones waiting on the user (a pending question, or an errored run not yet
/// opened). Read from the session headers, so it survives a restart and agrees
/// with the sidebar. Settled chats are filed away and never count, and a
/// running chat shows its spinner, not a badge.
pub fn badge_count(sessions: &[crate::types::session::SessionHeader]) -> usize {
    sessions
        .iter()
        .filter(|session| session.deleted_at.is_none() && session.settled_at.is_none())
        .filter(|session| session.status != "working" && session.status != "running")
        .filter(|session| session.unread || session.status == "needs_attention")
        .count()
}

/// Dock badge text for a count of sessions with unopened banners.
/// Zero clears the badge (`None`); otherwise the count as a string.
pub fn dock_badge_label(outstanding: usize) -> Option<String> {
    if outstanding == 0 {
        None
    } else {
        Some(outstanding.to_string())
    }
}

/// Empty titles fall back to `Console` so banners never post blank.
pub fn normalize_notification_title(title: &str) -> String {
    if title.is_empty() {
        "Console".to_string()
    } else {
        title.to_string()
    }
}

/// Banner decision for an incoming event given the currently viewed session.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum NotificationDecision {
    /// Post (or replace) the banner for this session.
    Notify,
    /// User is already viewing it: silent in-app update + clear stale banner.
    SuppressViewing,
    /// No session id: nothing to post or clear.
    SkipEmpty,
}

/// Mirrors `notifications.rs`: viewing same session suppresses, empty skips.
pub fn decide_notification(
    viewing_session_id: Option<&str>,
    event_session_id: &str,
) -> NotificationDecision {
    if event_session_id.is_empty() {
        NotificationDecision::SkipEmpty
    } else if viewing_session_id == Some(event_session_id) {
        NotificationDecision::SuppressViewing
    } else {
        NotificationDecision::Notify
    }
}
