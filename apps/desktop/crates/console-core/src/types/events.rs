use super::agent::{
    AskQuestionRequest, AssistantMessage, ImageAttachment, PermissionRequest, TodoItem, ToolCall,
    ToolCallPreview, ToolResult,
};
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RunPromptDto {
    pub prompt: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub attachments: Option<Vec<ImageAttachment>>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AnswerQuestionDto {
    pub request_id: String,
    pub answer: serde_json::Value,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApproveToolPermissionDto {
    pub request_id: String,
    pub allow: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum AgentSessionEvent {
    SessionStart,
    TurnStart {
        prompt: String,
    },
    ModelStreamStart {
        #[serde(rename = "turnId")]
        turn_id: String,
    },
    ModelStreamPart {
        part: ModelStreamPartPayload,
    },
    ModelStreamEnd {
        #[serde(rename = "turnId")]
        turn_id: String,
        #[serde(default)]
        turn: Option<AssistantMessage>,
    },
    ToolExecutionStart {
        calls: Vec<ToolCall>,
    },
    PermissionRequest {
        request: PermissionRequest,
    },
    AskQuestion {
        request: AskQuestionRequest,
    },
    ToolExecutionResult {
        result: ToolResult,
    },
    ToolExecutionEnd {
        results: Vec<ToolResult>,
    },
    TodoUpdate {
        items: Vec<TodoItem>,
        action: String,
    },
    Compaction {
        summary: String,
        original_message_count: usize,
    },
    TurnEnd {
        #[serde(rename = "turnId")]
        turn_id: String,
    },
    SubagentStart {
        #[serde(rename = "subagentId")]
        subagent_id: String,
        #[serde(rename = "parentToolCallId")]
        parent_tool_call_id: String,
        name: String,
        role: String,
        prompt: String,
        #[serde(rename = "maxTurns")]
        max_turns: usize,
    },
    SubagentActivity {
        #[serde(rename = "subagentId")]
        subagent_id: String,
        #[serde(rename = "turnIndex")]
        turn_index: usize,
        #[serde(rename = "toolCallId")]
        tool_call_id: String,
        #[serde(rename = "toolName")]
        tool_name: String,
        #[serde(default)]
        args: Option<serde_json::Value>,
        status: String,
        #[serde(default)]
        error: Option<String>,
    },
    SubagentEnd {
        #[serde(rename = "subagentId")]
        subagent_id: String,
        status: String,
        #[serde(default)]
        summary: Option<String>,
        #[serde(default)]
        error: Option<String>,
        #[serde(rename = "totalTurns")]
        total_turns: usize,
    },
    SessionEnd,
    Error {
        error: ServerErrorPayload,
    },
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelStreamPartPayload {
    pub text: Option<String>,
    pub thinking: Option<String>,
    pub tool_call: Option<ToolCallPreview>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerErrorPayload {
    pub message: String,
    pub data: Option<serde_json::Value>,
}
