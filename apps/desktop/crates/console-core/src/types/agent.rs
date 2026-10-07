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
    AskQuestionRequest as ProtoAskQuestionRequest,
    AssistantContentPart as ProtoContentPart, BrowserActionRequest as ProtoBrowserActionRequest,
    ErrorPayload as ProtoErrorPayload, ImageAttachment as ProtoImageAttachment,
    PermissionRequest as ProtoPermissionRequest,
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
    #[serde(default)]
    pub id: Option<String>,
    pub content: Vec<AssistantContentPart>,
    #[serde(default)]
    pub stop_reason: Option<String>,
    #[serde(default)]
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

impl AssistantMessage {
    /// Decode a turn-close payload from the canonical wire shape. Same
    /// normalization as the transcript path (empty id/stop_reason fall back
    /// to None); the turn carries no timestamp on the wire.
    pub fn from_proto_assistant(msg: &ProtoAssistantMessage) -> Self {
        AssistantMessage {
            id: opt_string(msg.id.clone()),
            content: msg
                .content
                .iter()
                .filter_map(content_part_from_proto)
                .collect(),
            stop_reason: opt_string(msg.stop_reason.clone()),
            created_at: None,
        }
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
        #[serde(default)]
        attachments: Option<Vec<ImageAttachment>>,
        #[serde(default, skip_serializing_if = "Option::is_none", rename = "contextFiles")]
        context_files: Option<Vec<String>>,
        #[serde(default, rename = "createdAt")]
        created_at: Option<i64>,
    },
    #[serde(rename = "assistant")]
    Assistant {
        #[serde(default)]
        id: Option<String>,
        content: Vec<AssistantContentPart>,
        #[serde(default, rename = "stopReason")]
        stop_reason: Option<String>,
        #[serde(default, rename = "createdAt")]
        created_at: Option<i64>,
    },
    #[serde(rename = "toolResult")]
    ToolResult {
        #[serde(default)]
        results: Vec<ToolResult>,
        #[serde(default, rename = "createdAt")]
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
    #[serde(default)]
    pub arguments: serde_json::Value,
    #[serde(default)]
    pub thought_signature: Option<String>,
}

impl ToolCall {
    /// Decode from the canonical wire shape (arguments arrive as raw JSON
    /// bytes carrying UTF-8). Infallible: every field has a default.
    pub fn from_proto(msg: &ProtoToolCall) -> Self {
        ToolCall {
            id: msg.id.clone(),
            name: msg.name.clone(),
            arguments: decode_json_bytes(&msg.arguments),
            thought_signature: msg.thought_signature.clone(),
        }
    }
}

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/event.proto): the id+name preview inside stream parts.
/// Args arrive separately with the toolExecutionStart frame.
pub use console_proto::ToolCallPreview;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ToolResult {
    pub tool_call_id: String,
    #[serde(default)]
    pub tool_name: Option<String>,
    #[serde(default)]
    pub content: serde_json::Value,
    #[serde(default)]
    pub is_error: Option<bool>,
}

impl ToolResult {
    /// Decode from the canonical wire shape (content arrives as raw JSON
    /// bytes carrying UTF-8). Infallible: every field has a default.
    pub fn from_proto(msg: &ProtoToolResult) -> Self {
        ToolResult {
            tool_call_id: msg.tool_call_id.clone(),
            tool_name: msg.tool_name.clone(),
            content: decode_json_bytes(&msg.content),
            is_error: msg.is_error,
        }
    }
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
    /// Client-only UI state nothing populates; kept for the card shape,
    /// always None from the wire.
    pub requires_upgrade: Option<bool>,
}

impl PermissionRequest {
    /// Decode from the canonical wire shape (args arrive as raw JSON bytes).
    pub fn from_proto(msg: &ProtoPermissionRequest) -> Self {
        PermissionRequest {
            request_id: msg.request_id.clone(),
            tool_call_id: msg.tool_call_id.clone(),
            tool_name: msg.tool_name.clone(),
            args: decode_json_bytes(&msg.args),
            tier: msg.tier.clone(),
            reason: msg.reason.clone(),
            requires_upgrade: None,
        }
    }
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

impl BrowserActionRequest {
    /// Decode from the canonical wire shape. Numeric/bool fields arrive
    /// absent-as-default and map back to None, matching the old omitempty.
    pub fn from_proto(msg: &ProtoBrowserActionRequest) -> Self {
        let none_if_zero = |v: i32| if v == 0 { None } else { Some(v as u64) };
        BrowserActionRequest {
            request_id: msg.request_id.clone(),
            action: msg.action.clone(),
            url: msg.url.clone(),
            script: msg.script.clone(),
            selector: msg.selector.clone(),
            tab_id: msg.tab_id.clone(),
            url_contains: msg.url_contains.clone(),
            text: msg.text.clone(),
            timeout_ms: none_if_zero(msg.timeout_ms),
            element_ref: msg.r#ref.clone(),
            submit: if msg.submit { Some(true) } else { None },
        }
    }
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

impl AskQuestionRequest {
    /// Decode from the canonical wire shape. Empty options and false
    /// is_multi_select map back to None, matching the old omitempty;
    /// skippable is always set server-side, so it stays Some.
    pub fn from_proto(msg: &ProtoAskQuestionRequest) -> Self {
        AskQuestionRequest {
            request_id: msg.request_id.clone(),
            question: msg.question.clone(),
            options: if msg.options.is_empty() {
                None
            } else {
                Some(msg.options.clone())
            },
            is_multi_select: if msg.is_multi_select {
                Some(true)
            } else {
                None
            },
            skippable: msg.skippable,
            batch_id: msg.batch_id.clone(),
        }
    }
}

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/session.proto). Todo statuses stay plain strings
/// (pending/in_progress/completed); ids narrow to i32 and stay JSON numbers.
pub use console_proto::TodoItem;

/// Canonical wire types from the shared protobuf schema
/// (proto/console/v1/session.proto). Activity args cross as raw JSON bytes;
/// counts narrow to i32 and stay JSON numbers; timestamps encode as strings.
pub use console_proto::{SubagentActivityItem, SubagentInfo};

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/session.proto). The queueUpdated SSE frame stays
/// hand-shaped until the event stream migrates, but its nested queuedPrompt
/// payload decodes through this same type.
///
/// Wire notes: created_at is the server's RFC3339 string (not millis);
/// thinking_level is desktop-optimistic (pane default on the local card) —
/// the server never populates it. The ThinkingLevel enum stays client-side
/// with conversion at the boundary (helpers below).
pub use console_proto::QueuedPrompt;

/// Boundary conversions between the render [`ImageAttachment`] and the wire
/// shape. Both are `{data, mimeType}`; only the container differs
/// (`Option<Vec>` locally, bare `Vec` on the wire where empty means absent).
pub fn queued_attachments_to_proto(
    items: Option<Vec<ImageAttachment>>,
) -> Vec<ProtoImageAttachment> {
    items.unwrap_or_default().into_iter().map(|a| ProtoImageAttachment {
        data: a.data,
        mime_type: a.mime_type,
    }).collect()
}

pub fn queued_attachments_from_proto(
    items: &[ProtoImageAttachment],
) -> Option<Vec<ImageAttachment>> {
    if items.is_empty() {
        None
    } else {
        Some(items.iter().map(|a| ImageAttachment {
            data: a.data.clone(),
            mime_type: a.mime_type.clone(),
        }).collect())
    }
}

/// Boundary conversions between [`ThinkingLevel`] and its wire string.
/// Unknown strings fall back to None, matching the old default.
pub fn queued_thinking_to_proto(level: Option<ThinkingLevel>) -> Option<String> {
    level.as_ref().map(|l| l.as_str().to_string())
}

pub fn queued_thinking_from_proto(raw: Option<&str>) -> Option<ThinkingLevel> {
    raw.and_then(ThinkingLevel::from_str)
}

/// Repeated strings cross as a bare `Vec` on the wire (empty means absent);
/// back to `None` when empty, preserving the old omit-when-empty requests.
pub fn queued_strings_from_proto(items: &[String]) -> Option<Vec<String>> {
    if items.is_empty() {
        None
    } else {
        Some(items.to_vec())
    }
}
