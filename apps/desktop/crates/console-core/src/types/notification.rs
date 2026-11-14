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

/// Whether showing a chat needs a read report: it is unread, or it is flagged
/// as needing attention (an errored run the user has now seen). A chat that is
/// already read needs none, which is what keeps reopening a quiet chat, or
/// returning to the window while on one, free of any request.
pub fn needs_read_report(session: &crate::types::session::SessionHeader) -> bool {
    session.unread || session.status == "needs_attention"
}

/// Whether a freshly fetched session list means the chat in front of the user
/// must be reported read. The list is the server's word, so this reads the fresh
/// copy and never the app's older local one: a run that ended while the window
/// was in the background is unread on the server before the app has heard of
/// it. Only the active chat in a focused window counts; every other chat keeps
/// its mark until it is opened.
///
/// This looks at the server's `unread` flag only, never at `needs_attention`.
/// A chat waiting on a pending question stays `needs_attention` on the server
/// even once read, because its run is still active; treating that status as
/// "needs a report" here would report, refresh, see it again, and loop for as
/// long as the question is open.
pub fn fresh_list_needs_read(
    fresh: &[crate::types::session::SessionHeader],
    active_session_id: Option<&str>,
    window_active: bool,
) -> bool {
    if !window_active {
        return false;
    }
    let Some(active) = active_session_id else {
        return false;
    };
    fresh
        .iter()
        .find(|session| session.id == active)
        .is_some_and(|session| session.unread)
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
