use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Automation {
    pub id: String,
    pub session_id: String,
    pub prompt: String,
    pub kind: String,
    #[serde(default)]
    pub interval_seconds: i64,
    #[serde(default)]
    pub run_at: i64,
    pub next_run_at: i64,
    pub status: String,
    #[serde(default)]
    pub last_fired_at: i64,
    #[serde(default)]
    pub last_error: String,
    pub created_at: i64,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AutomationEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    #[serde(default)]
    pub automation: Option<Automation>,
    #[serde(default)]
    pub id: Option<String>,
    #[serde(default)]
    pub session_id: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WebhookSubscription {
    pub id: String,
    pub session_id: String,
    pub topic: String,
    #[serde(default)]
    pub secret: String,
    #[serde(default)]
    pub filter: Option<serde_json::Value>,
    pub prompt: String,
    pub once: bool,
    pub status: String,
    #[serde(default)]
    pub expires_at: i64,
    #[serde(default)]
    pub last_fired_at: i64,
    #[serde(default)]
    pub fire_count: i64,
    #[serde(default)]
    pub last_error: String,
    pub created_at: i64,
    #[serde(default)]
    pub webhook_path: String,
    #[serde(default)]
    pub topic_path: String,
    #[serde(default)]
    pub webhook_url: Option<String>,
    #[serde(default)]
    pub webhook_url_with_token: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WebhookEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    #[serde(default)]
    pub subscription: Option<WebhookSubscription>,
    #[serde(default)]
    pub id: Option<String>,
    #[serde(default)]
    pub session_id: Option<String>,
}
