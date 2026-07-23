use serde::{Deserialize, Serialize};

use super::agent::AgentMessage;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionHeader {
    pub id: String,
    pub title: String,
    pub cwd: String,
    pub project_id: Option<String>,
    pub model_id: String,
    pub provider: String,
    pub created_at: u64,
    pub updated_at: u64,
    pub message_count: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDetailResponse {
    pub header: SessionHeader,
    pub messages: Vec<AgentMessage>,
}
