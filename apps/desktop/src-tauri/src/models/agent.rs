use serde::{Deserialize, Serialize};

use super::tool::{ToolCall, ToolResult};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TextPart {
    #[serde(rename = "text")]
    Text { text: String },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum ThinkingPart {
    #[serde(rename = "thinking")]
    Thinking { text: String },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum ToolCallPart {
    #[serde(rename = "toolCall")]
    ToolCall { call: ToolCall },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum AssistantMessageContent {
    #[serde(rename = "text")]
    Text { text: String },
    #[serde(rename = "thinking")]
    Thinking { text: String },
    #[serde(rename = "toolCall")]
    ToolCall { call: ToolCall },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ImagePart {
    #[serde(rename = "type")]
    pub part_type: String,
    pub data: String,
    pub mime_type: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UserMessage {
    pub role: String,
    pub content: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub attachments: Option<Vec<ImagePart>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AssistantMessage {
    pub role: String,
    pub id: Option<String>,
    pub content: Vec<AssistantMessageContent>,
    pub stop_reason: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolResultMessage {
    pub role: String,
    pub results: Vec<ToolResult>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
#[serde(tag = "role")]
pub enum AgentMessage {
    #[serde(rename = "user")]
    User {
        content: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        attachments: Option<Vec<ImagePart>>,
        #[serde(rename = "createdAt", default, skip_serializing_if = "Option::is_none")]
        created_at: Option<f64>,
    },
    #[serde(rename = "assistant")]
    Assistant {
        id: Option<String>,
        content: Vec<AssistantMessageContent>,
        #[serde(rename = "stopReason")]
        stop_reason: Option<String>,
        #[serde(rename = "createdAt", default, skip_serializing_if = "Option::is_none")]
        created_at: Option<f64>,
    },
    #[serde(rename = "toolResult")]
    ToolResult {
        results: Vec<ToolResult>,
        #[serde(rename = "createdAt", default, skip_serializing_if = "Option::is_none")]
        created_at: Option<f64>,
    },
}
