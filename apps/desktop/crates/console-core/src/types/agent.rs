use super::model::ThinkingLevel;
use serde::{Deserialize, Serialize};

// Canonical wire types from the shared protobuf schema
// (proto/console/v1/message.proto). These hand-written message types are the
// transcript RENDER models: the UI constructs them optimistically, mutates
// them while streaming, and matches on them everywhere. They decode from the
// wire through from_proto constructors below — never derive wire serde.
// Annotations exist only for replay markup server-side and are dropped here;
// the desktop never reads them.
pub use console_proto::{
    AgentMessage as ProtoAgentMessage, AgentAssistantMessage as ProtoAssistantMessage,
    AssistantContentPart as ProtoContentPart, ImageAttachment as ProtoImageAttachment,
    ToolCall as ProtoToolCall, ToolResult as ProtoToolResult,
};
pub use console_proto::agent_message::Message as ProtoMessageEvent;
pub use console_proto::assistant_content_part::Part as ProtoPartEvent;

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ImageAttachment {
    pub data: String,
    pub mime_type: String,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RectDimensions {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BrowserElementAnnotation {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_id: Option<String>,
    pub url: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub component_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub source_location: Option<String>,
    pub selector: String,
    pub html_snippet: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub dimensions: Option<RectDimensions>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub role: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub accessible_name: Option<String>,
    #[serde(default, skip_serializing_if = "std::collections::BTreeMap::is_empty")]
    pub computed_styles: std::collections::BTreeMap<String, String>,
    pub user_comment: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub screenshot_base64: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AssistantMessage {
    pub id: Option<String>,
    pub content: Vec<AssistantContentPart>,
    pub stop_reason: Option<String>,
    pub created_at: Option<i64>,
}

/// Decode helpers from the canonical wire shape into the render models.
fn decode_json_bytes(bytes: &[u8]) -> serde_json::Value {
    serde_json::from_slice(bytes).unwrap_or(serde_json::Value::Null)
}

fn opt_string(s: String) -> Option<String> {
    if s.is_empty() {
        None
    } else {
        Some(s)
    }
}

impl AgentMessage {
    pub fn from_proto(msg: ProtoAgentMessage) -> Option<Self> {
        match msg.message? {
            ProtoMessageEvent::User(u) => Some(AgentMessage::User {
                id: msg.id.clone(),
                content: u.content.clone(),
                attachments: if u.attachments.is_empty() {
                    None
                } else {
                    Some(
                        u.attachments
                            .iter()
                            .map(|a| ImageAttachment {
                                data: a.data.clone(),
                                mime_type: a.mime_type.clone(),
                            })
                            .collect(),
                    )
                },
                context_files: if u.context_files.is_empty() {
                    None
                } else {
                    Some(u.context_files.clone())
                },
                created_at: msg.created_at,
            }),
            ProtoMessageEvent::Assistant(a) => Some(AgentMessage::Assistant {
                id: opt_string(a.id.clone()),
                content: a.content.iter().filter_map(content_part_from_proto).collect(),
                stop_reason: opt_string(a.stop_reason.clone()),
                created_at: msg.created_at,
            }),
            ProtoMessageEvent::ToolResult(t) => Some(AgentMessage::ToolResult {
                results: t
                    .results
                    .iter()
                    .map(|r| ToolResult {
                        tool_call_id: r.tool_call_id.clone(),
                        tool_name: r.tool_name.clone(),
                        content: decode_json_bytes(&r.content),
                        is_error: r.is_error,
                    })
                    .collect(),
                created_at: msg.created_at,
            }),
        }
    }
}

fn content_part_from_proto(part: &ProtoContentPart) -> Option<AssistantContentPart> {
    match part.part.as_ref()? {
        ProtoPartEvent::Text(t) => Some(AssistantContentPart::Text {
            text: t.text.clone(),
            thought_signature: t.thought_signature.clone(),
        }),
        ProtoPartEvent::Thinking(t) => Some(AssistantContentPart::Thinking { text: t.text.clone() }),
        ProtoPartEvent::ToolCall(c) => Some(AssistantContentPart::ToolCall {
            call: ToolCall {
                id: c.id.clone(),
                name: c.name.clone(),
                arguments: decode_json_bytes(&c.arguments),
                thought_signature: c.thought_signature.clone(),
            },
        }),
        ProtoPartEvent::Image(i) => Some(AssistantContentPart::Image {
            data: i.data.clone(),
            mime_type: i.mime_type.clone(),
        }),
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "role", rename_all = "camelCase")]
pub enum AgentMessage {
    #[serde(rename = "user")]
    User {
        /// Server-assigned message id. The server has always sent it; the
        /// composer needs it to re-stage a recalled prompt's image
        /// attachments from the transcript.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        id: Option<String>,
        content: String,
        attachments: Option<Vec<ImageAttachment>>,
        #[serde(default, skip_serializing_if = "Option::is_none", rename = "contextFiles")]
        context_files: Option<Vec<String>>,
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
pub struct BrowserActionRequest {
    pub request_id: String,
    pub action: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub script: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub selector: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tab_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub url_contains: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub text: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub timeout_ms: Option<u64>,
    #[serde(default, rename = "ref", skip_serializing_if = "Option::is_none")]
    pub element_ref: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub submit: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BrowserActionResult {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub result: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    /// Base64 PNG attached to the tool result (screenshot action).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub image_base64: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ResolveBrowserActionDto {
    pub request_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub result: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    /// Base64 PNG attached to the tool result (screenshot action).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub image_base64: Option<String>,
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

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/session.proto). Todo statuses stay plain strings
/// (pending/in_progress/completed); ids narrow to i32 and stay JSON numbers.
pub use console_proto::TodoItem;

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct SubagentActivityItem {
    pub turn_index: usize,
    pub tool_call_id: String,
    pub tool_name: String,
    #[serde(default)]
    pub summary: Option<String>,
    #[serde(default)]
    pub args: Option<serde_json::Value>,
    pub status: String, // "running", "completed", "error"
    #[serde(default)]
    pub error: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct SubagentInfo {
    pub subagent_id: String,
    pub parent_tool_call_id: String,
    pub name: String,
    pub role: String,
    pub prompt: String,
    pub max_turns: usize,
    pub current_turn: usize,
    pub status: String, // "running", "completed", "aborted", "error"
    #[serde(default)]
    pub summary: Option<String>,
    #[serde(default)]
    pub error: Option<String>,
    #[serde(default)]
    pub activities: Vec<SubagentActivityItem>,
}

/// A prompt staged to run automatically once the session's active turn settles.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct QueuedPrompt {
    pub id: String,
    pub session_id: String,
    pub prompt: String,
    #[serde(default)]
    pub context_files: Option<Vec<String>>,
    #[serde(default)]
    pub attachments: Option<Vec<ImageAttachment>>,
    #[serde(default)]
    pub model_id: Option<String>,
    #[serde(default)]
    pub provider: Option<String>,
    #[serde(default)]
    pub approval_mode: Option<String>,
    #[serde(default)]
    pub thinking_level: Option<ThinkingLevel>,
    pub created_at: String,
}
