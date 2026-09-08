use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct NotificationEvent {
    #[serde(default)]
    pub r#type: String,
    #[serde(default)]
    pub kind: String,
    #[serde(rename = "sessionId", default)]
    pub session_id: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub body: String,
}

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
