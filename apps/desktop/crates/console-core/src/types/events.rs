use super::agent::{
    AskQuestionRequest, BrowserActionRequest, ImageAttachment, PermissionRequest,
    QueuedPrompt, TodoItem,
};
use super::model::ThinkingLevel;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RunPromptDto {
    pub prompt: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context_files: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
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
        part: console_proto::ModelStreamPart,
    },
    ModelStreamEnd {
        #[serde(rename = "turnId")]
        turn_id: String,
        #[serde(default)]
        turn: Option<console_proto::AgentAssistantMessage>,
    },
    ToolExecutionStart {
        calls: Vec<console_proto::ToolCall>,
    },
    PermissionRequest {
        request: PermissionRequest,
    },
    AskQuestion {
        request: AskQuestionRequest,
    },
    ToolExecutionResult {
        result: console_proto::ToolResult,
    },
    ToolExecutionEnd {
        results: Vec<console_proto::ToolResult>,
    },
    TodoUpdate {
        items: Vec<TodoItem>,
        action: String,
    },
    Compaction {
        summary: String,
        original_message_count: usize,
    },
    BrowserAction {
        request: BrowserActionRequest,
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
    SessionTitleUpdated {
        title: String,
    },
    Error {
        error: ServerErrorPayload,
    },
    /// The session's queued next-turn prompt changed (queued, replaced, or cleared).
    QueueUpdated {
        #[serde(rename = "queuedPrompt")]
        queued_prompt: Option<QueuedPrompt>,
    },
    /// Estimated context-window occupancy after a turn.
    ContextUpdate {
        context: super::ContextSnapshot,
    },
}

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/event.proto). The oneof holds scalars directly so the
/// wire stays {"part":{"text":"..."}} — one arm per delta, exactly what the
/// server emits. Match on [`ModelStreamPartKind`] to discriminate.
pub use console_proto::ModelStreamPart;
/// Re-exported for match arms on the stream-part oneof.
pub use console_proto::model_stream_part::Part as ModelStreamPartKind;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerErrorPayload {
    pub message: String,
    pub data: Option<serde_json::Value>,
}
