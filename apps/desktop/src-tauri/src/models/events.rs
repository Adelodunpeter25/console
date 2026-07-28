use serde::{Deserialize, Serialize};

use super::agent::AssistantMessage;
use super::tool::{PermissionRequest, ToolCall, ToolResult};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AskQuestionRequest {
    pub request_id: String,
    pub question: String,
    pub options: Vec<String>,
    pub is_multi_select: Option<bool>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
#[serde(tag = "type")]
pub enum AgentSessionEvent {
    #[serde(rename = "sessionStart")]
    SessionStart,
    #[serde(rename = "turnStart")]
    TurnStart { prompt: String },
    #[serde(rename = "modelStreamStart", rename_all = "camelCase")]
    ModelStreamStart { turn_id: String },
    #[serde(rename = "modelStreamPart")]
    ModelStreamPart {
        part: ModelStreamPartData,
    },
    #[serde(rename = "modelStreamEnd", rename_all = "camelCase")]
    ModelStreamEnd { turn_id: String, turn: AssistantMessage },
    #[serde(rename = "toolExecutionStart")]
    ToolExecutionStart { calls: Vec<ToolCall> },
    #[serde(rename = "permissionRequest")]
    PermissionRequest { request: PermissionRequest },
    #[serde(rename = "askQuestion")]
    AskQuestion { request: AskQuestionRequest },
    #[serde(rename = "toolExecutionResult")]
    ToolExecutionResult { result: ToolResult },
    #[serde(rename = "toolExecutionEnd")]
    ToolExecutionEnd { results: Vec<ToolResult> },
    #[serde(rename = "compaction", rename_all = "camelCase")]
    Compaction {
        summary: String,
        original_message_count: u64,
    },
    #[serde(rename = "turnEnd", rename_all = "camelCase")]
    TurnEnd { turn_id: String },
    #[serde(rename = "sessionEnd")]
    SessionEnd,
    #[serde(rename = "error")]
    Error {
        error: AgentSessionError,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelStreamPartData {
    pub text: Option<String>,
    pub thinking: Option<String>,
    pub tool_call: Option<ToolCall>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentSessionError {
    pub message: String,
    pub data: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SseEventFrame {
    pub event: String,
    pub data: AgentSessionEvent,
}
