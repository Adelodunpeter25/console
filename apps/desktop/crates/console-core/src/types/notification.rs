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
