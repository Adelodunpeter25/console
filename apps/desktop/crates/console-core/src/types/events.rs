use super::agent::{ImageAttachment, QueuedPrompt, TodoItem};
use super::model::ThinkingLevel;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

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
#[serde(rename_all = "camelCase")]
pub struct BackgroundWork {
    pub id: String,
    pub session_id: String,
    #[serde(default)]
    pub parent_run_id: Option<String>,
    #[serde(default)]
    pub parent_call_id: Option<String>,
    pub kind: String,
    pub status: String,
    pub created_at: String,
    #[serde(default)]
    pub started_at: Option<String>,
    #[serde(default)]
    pub finished_at: Option<String>,
    #[serde(default)]
    pub summary: Option<String>,
    #[serde(default)]
    pub error: Option<String>,
    #[serde(default)]
    pub metadata: HashMap<String, String>,
}

impl BackgroundWork {
    /// A detached shell that is still running: what the chat banner and the
    /// Shells tab list.
    pub fn is_running_shell(&self) -> bool {
        self.kind == "shell" && self.status == "running"
    }

    /// The command line to show for a shell: its first command line, else the
    /// summary, else a short id.
    pub fn shell_command(&self) -> String {
        let first = |s: &str| s.lines().next().unwrap_or("").trim().to_string();
        self.metadata
            .get("command")
            .map(|c| first(c))
            .filter(|c| !c.is_empty())
            .or_else(|| {
                self.summary
                    .as_deref()
                    .map(first)
                    .filter(|c| !c.is_empty())
            })
            .unwrap_or_else(|| format!("Shell {}", self.id.chars().take(6).collect::<String>()))
    }
}

/// Paged output for a detached shell
/// (`GET /api/sessions/:id/background-work/:workId/output`).
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BackgroundWorkOutput {
    #[serde(default)]
    pub stdout: String,
    #[serde(default)]
    pub stderr: String,
    #[serde(default)]
    pub next_cursor: i64,
    #[serde(default)]
    pub truncated: bool,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub exit_code: Option<i32>,
    #[serde(default)]
    pub timed_out: bool,
}

impl BackgroundWorkOutput {
    pub fn is_running(&self) -> bool {
        self.status == "running"
    }

    /// stdout followed by stderr, as one block of text.
    pub fn combined(&self) -> String {
        let mut out = self.stdout.clone();
        if !self.stderr.trim().is_empty() {
            if !out.is_empty() && !out.ends_with('\n') {
                out.push('\n');
            }
            out.push_str(&self.stderr);
        }
        out
    }

    /// Status line: Running, Exited · code 0, Stopped, Timed out, ...
    pub fn status_label(&self) -> String {
        let code = self
            .exit_code
            .map(|c| format!(" · code {c}"))
            .unwrap_or_default();
        if self.timed_out {
            "Timed out".into()
        } else {
            match self.status.as_str() {
                "running" => "Running".into(),
                "killed" => "Stopped".into(),
                "exited" => format!("Exited{code}"),
                "failed" => format!("Failed{code}"),
                other => other.to_string(),
            }
        }
    }
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
        request: console_proto::PermissionRequest,
    },
    AskQuestion {
        request: console_proto::AskQuestionRequest,
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
        request: console_proto::BrowserActionRequest,
    },
    TurnEnd {
        #[serde(rename = "turnId")]
        turn_id: String,
    },
    SubagentStart {
        #[serde(flatten)]
        event: console_proto::SubagentStartEvent,
    },
    SubagentActivity {
        #[serde(flatten)]
        event: console_proto::SubagentActivityEvent,
    },
    SubagentEnd {
        #[serde(flatten)]
        event: console_proto::SubagentEndEvent,
    },
    BackgroundWorkStarted {
        work: BackgroundWork,
    },
    BackgroundWorkProgress {
        work: BackgroundWork,
    },
    BackgroundWorkCompleted {
        work: BackgroundWork,
    },
    BackgroundWorkFailed {
        work: BackgroundWork,
    },
    BackgroundWorkCancelled {
        work: BackgroundWork,
    },
    /// An idle session resumed on its own because background work finished.
    /// Clients may surface `message`; the wake turn itself is never rendered.
    BackgroundWorkContinue {
        message: String,
        #[serde(default)]
        work: Vec<BackgroundWork>,
    },
    SessionEnd,
    SessionTitleUpdated {
        title: String,
    },
    Error {
        error: console_proto::ErrorPayload,
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

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/event.proto): the error frame carries one string.
/// The old hand shape's data field was never populated server-side.
pub use console_proto::ErrorPayload;
