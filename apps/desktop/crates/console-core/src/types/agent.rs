use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ImageAttachment {
    pub data: String,
    pub mime_type: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AssistantMessage {
    pub id: Option<String>,
    pub content: Vec<AssistantContentPart>,
    pub stop_reason: Option<String>,
    pub created_at: Option<i64>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "role", rename_all = "camelCase")]
pub enum AgentMessage {
    #[serde(rename = "user")]
    User {
        content: String,
        attachments: Option<Vec<ImageAttachment>>,
        #[serde(rename = "createdAt")]
        created_at: Option<i64>,
    },
    #[serde(rename = "assistant")]
    Assistant {
        id: Option<String>,
        content: Vec<AssistantContentPart>,
        #[serde(rename = "stopReason")]
        stop_reason: Option<String>,
        #[serde(rename = "createdAt")]
        created_at: Option<i64>,
    },
    #[serde(rename = "toolResult")]
    ToolResult {
        results: Vec<ToolResult>,
        #[serde(rename = "createdAt")]
        created_at: Option<i64>,
    },
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum AssistantContentPart {
    #[serde(rename = "text")]
    Text {
        text: String,
        #[serde(rename = "thoughtSignature")]
        thought_signature: Option<String>,
    },
    #[serde(rename = "thinking")]
    Thinking { text: String },
    #[serde(rename = "toolCall")]
    ToolCall { call: ToolCall },
    #[serde(rename = "image")]
    Image {
        data: String,
        #[serde(rename = "mimeType")]
        mime_type: String,
    },
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolCall {
    pub id: String,
    pub name: String,
    pub arguments: serde_json::Value,
    pub thought_signature: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolCallPreview {
    pub id: String,
    pub name: String,
    #[serde(default)]
    pub arguments: Option<serde_json::Value>,
    pub thought_signature: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolResult {
    pub tool_call_id: String,
    pub tool_name: Option<String>,
    pub content: serde_json::Value,
    pub is_error: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PermissionRequest {
    pub request_id: String,
    pub tool_call_id: String,
    pub tool_name: String,
    pub args: serde_json::Value,
    pub tier: String,
    pub reason: Option<String>,
    pub requires_upgrade: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AskQuestionRequest {
    pub request_id: String,
    pub question: String,
    pub options: Option<Vec<String>>,
    pub is_multi_select: Option<bool>,
    pub skippable: Option<bool>,
    pub batch_id: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TodoItem {
    pub id: i64,
    pub content: String,
    pub status: String,
}
