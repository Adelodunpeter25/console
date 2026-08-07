use serde::{Deserialize, Serialize};

/// Native notification payload pushed by the backend
/// (mirrors packages/types/src/notifications.ts).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NotificationEvent {
    #[serde(rename = "type")]
    pub kind_type: String,
    pub kind: String,
    pub session_id: String,
    pub title: String,
    pub body: String,
}
