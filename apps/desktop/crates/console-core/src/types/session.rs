use super::agent::AgentMessage;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum SessionStatus {
    Idle,
    Working,
    Done,
    NeedsAttention,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionHeader {
    pub id: String,
    pub title: String,
    pub cwd: String,
    pub project_id: Option<String>,
    pub model_id: String,
    pub provider: String,
    pub created_at: i64,
    pub updated_at: i64,
    pub message_count: Option<usize>,
    pub status: Option<SessionStatus>,
    pub approval_mode: Option<String>,
    pub deleted_at: Option<i64>,
}

impl SessionHeader {
    pub fn display_title(&self) -> &str {
        let trimmed = self.title.trim();
        if trimmed.is_empty() {
            "New Chat"
        } else {
            trimmed
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDetailResponse {
    #[serde(alias = "session")]
    pub header: SessionHeader,
    pub messages: Vec<AgentMessage>,
    #[serde(default)]
    pub has_more: bool,
    #[serde(default)]
    pub next_cursor: Option<i64>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateSessionDto {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    pub project_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSessionDto {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionFileChange {
    pub path: String,
    pub status: String,
    pub additions: u64,
    pub deletions: u64,
    pub turn_index: u64,
    pub updated_at: i64,
}

