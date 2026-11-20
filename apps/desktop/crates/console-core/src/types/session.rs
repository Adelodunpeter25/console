/// Optional pagination parameters for [`crate::services::SessionService::get`].
///
/// The server returns at most `limit` messages per response. With `before`
/// set to a rowid from a previous response's `next_cursor`, only messages
/// strictly older than that row are returned.
#[derive(Clone, Copy, Debug, Default)]
pub struct SessionPageOptions {
    pub limit: Option<u32>,
    pub before: Option<i64>,
}

use super::agent::{AgentMessage, ImageAttachment, QueuedPrompt, TodoItem, WakeRecord};
use super::events::BackgroundWork;
use super::model::ThinkingLevel;
use serde::{Deserialize, Serialize};

// Canonical wire type from the shared protobuf schema
// (proto/console/v1/session.proto). Timestamps encode as protojson strings;
// status and thinking level stay plain strings, matched literally.
pub use console_proto::{SessionHeader, SessionWorktree};

// Run statuses were a snake_case enum; the wire is strings now. UI-domain
// construction/comparison moved to plain strings with the server vocabulary
// (idle/working/done/needs_attention); unknown values fall through to the
// default branches at each match site.
pub trait SessionHeaderExt {
    fn display_title(&self) -> &str;

    fn sort_timestamp(&self) -> i64;

    fn is_pinned(&self) -> bool;
}

impl SessionHeaderExt for SessionHeader {
    fn display_title(&self) -> &str {
        // The server guarantees a non-empty stored title (Create/UpdateTitle
        // default it), so there is no client-side fallback to keep in sync.
        let trimmed = self.title.trim();
        if let Some(first_line) = trimmed.lines().find(|l| !l.trim().is_empty()) {
            first_line.trim()
        } else {
            trimmed
        }
    }

    fn sort_timestamp(&self) -> i64 {
        self.updated_at.max(self.created_at)
    }

    fn is_pinned(&self) -> bool {
        self.pinned_at.is_some()
    }
}

/// One user prompt from `GET /api/sessions/:id/prompts`. `id` matches the
/// message id on loaded transcript messages.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionPrompt {
    pub id: String,
    pub created_at: i64,
    pub preview: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDetailResponse {
    #[serde(alias = "session")]
    pub header: SessionHeader,
    /// Decoded from the canonical wire messages; unparseable rows are
    /// skipped, matching the old untagged leniency for forward growth.
    #[serde(deserialize_with = "deserialize_messages")]
    pub messages: Vec<AgentMessage>,
    /// Whether older messages exist before the oldest row in `messages`.
    #[serde(default)]
    pub has_more: bool,
    /// Rowid cursor for the next older batch; null when at the start of history.
    #[serde(default)]
    pub next_cursor: Option<i64>,
    /// Persisted wake-turn markers extracted from synthetic user turns.
    #[serde(default)]
    pub wake_records: Vec<WakeRecord>,
}

fn deserialize_messages<'de, D>(deserializer: D) -> Result<Vec<AgentMessage>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    // Rows are the canonical oneof shape; unparseable rows are skipped,
    // matching the old untagged leniency for forward growth.
    let raw: Vec<serde_json::Value> = serde::Deserialize::deserialize(deserializer)?;
    Ok(raw
        .into_iter()
        .filter_map(|value| {
            serde_json::from_value::<console_proto::AgentMessage>(value)
                .ok()
                .and_then(AgentMessage::from_proto)
        })
        .collect())
}

/// Everything a pane needs to paint a freshly opened chat in one round-trip.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionBootstrapResponse {
    #[serde(flatten)]
    pub detail: SessionDetailResponse,
    pub changes: Vec<SessionFileChange>,
    pub todos: Vec<TodoItem>,
    pub suggestions: Vec<String>,
    pub queued_prompts: Vec<QueuedPrompt>,
    pub background_work: Vec<BackgroundWork>,
    pub draft: Option<console_proto::ChatDraft>,
}

impl SessionDetailResponse {
    /// Convert a binary history page. The JSON path uses the same field
    /// names, so callers see one type either way.
    pub fn from_proto_page(p: console_proto::SessionMessagesPage) -> Self {
        Self {
            header: p.header.unwrap_or_default(),
            messages: p
                .messages
                .into_iter()
                .filter_map(AgentMessage::from_proto)
                .collect(),
            has_more: p.has_more,
            next_cursor: p.next_cursor,
            wake_records: p
                .wake_records
                .into_iter()
                .map(WakeRecord::from_proto)
                .collect(),
        }
    }
}

impl SessionBootstrapResponse {
    /// Convert the binary envelope. Messages are the only field needing a
    /// hand-written decoder; the rest are generated proto types already
    /// used by the JSON path.
    pub fn from_proto(p: console_proto::SessionBootstrapResponse) -> Self {
        Self {
            detail: SessionDetailResponse {
                header: p.header.unwrap_or_default(),
                messages: p
                    .messages
                    .into_iter()
                    .filter_map(AgentMessage::from_proto)
                    .collect(),
                has_more: p.has_more,
                next_cursor: p.next_cursor,
                wake_records: p
                    .wake_records
                    .into_iter()
                    .map(WakeRecord::from_proto)
                    .collect(),
            },
            changes: p.changes,
            todos: p.todos,
            suggestions: p.suggestions,
            queued_prompts: p.queued_prompts,
            background_work: p
                .background_work
                .into_iter()
                .map(BackgroundWork::from_proto)
                .collect(),
            draft: p.draft,
        }
    }
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
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
    /// Opt into a fresh worktree + branch backing the new session. An empty
    /// spec (all fields None) lets the server auto-derive the branch name.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub worktree: Option<CreateWorktreeSpec>,
}

/// Client-facing worktree request for `POST /api/sessions`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateWorktreeSpec {
    /// Names the branch to create. Omitted means the server derives one.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub branch: Option<String>,
    /// An existing branch to cut the new branch from. Distinct from `branch`
    /// because an existing branch's name can never be reused as the new
    /// branch's name — git rejects `worktree add -b <existing>`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub base_branch: Option<String>,
}

/// Request body for `POST /api/sessions/with-worktree` — the instant launch:
/// the server creates the session immediately and provisions the worktree +
/// runs `prompt` in the background. The response header carries
/// launchState ("provisioning" / "failed") until the prompt is running.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LaunchWorktreeDto {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    pub project_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
    pub worktree: CreateWorktreeSpec,
    pub prompt: LaunchPromptDto,
}

/// The stored first prompt of a worktree launch; the server runs it once the
/// worktree exists.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LaunchPromptDto {
    pub prompt: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub context_files: Vec<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub attachments: Vec<ImageAttachment>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSessionDto {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    // Double option: None omits the key (server infers from cwd),
    // Some(None) sends explicit null (server takes the scratchpad path),
    // Some(Some(id)) links the project. `No project` must send null —
    // omitting the key would let the server re-infer the old project.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub project_id: Option<Option<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub approval_mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pinned: Option<bool>,
}

/// Request body for `POST /api/sessions/:id/fork` — the message to fork
/// through (inclusive). Hand-built like the other session POST bodies.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ForkSessionDto {
    pub message_id: String,
}

/// Canonical wire type from the shared protobuf schema
/// (proto/console/v1/session.proto). Counts narrow to u32 and stay JSON
/// numbers; updated_at encodes as a protojson string.
pub use console_proto::{SessionFileChange, SessionFileChangeDiff};

/// Turn-scope selector for the Changes tab and review tab. Purely a
/// client-side filter over an already-fetched (unscoped) change list —
/// avoids a network round-trip per scope switch.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum ChangesScope {
    ThisTurn,
    #[default]
    AllTurns,
    PreviousTurn,
}

impl ChangesScope {
    pub const ALL: [Self; 3] = [Self::ThisTurn, Self::AllTurns, Self::PreviousTurn];

    pub fn label(self) -> &'static str {
        match self {
            Self::ThisTurn => "This Turn",
            Self::AllTurns => "All Turns",
            Self::PreviousTurn => "Previous Turn",
        }
    }
}

/// Count items in a change list for the given scope without cloning or
/// allocating `Vec<SessionFileChange>` or diff text.
pub fn count_changes_for_scope(changes: &[SessionFileChange], scope: ChangesScope) -> usize {
    if changes.is_empty() {
        return 0;
    }
    let latest_turn = changes.iter().map(|c| c.turn_index).max().unwrap_or(0);
    match scope {
        ChangesScope::AllTurns => {
            let mut paths = std::collections::HashSet::new();
            for change in changes {
                paths.insert(change.path.as_str());
            }
            paths.len()
        }
        ChangesScope::ThisTurn => changes
            .iter()
            .filter(|c| c.turn_index == latest_turn)
            .count(),
        ChangesScope::PreviousTurn => {
            let previous_turn = changes
                .iter()
                .map(|c| c.turn_index)
                .filter(|&t| t < latest_turn)
                .max();
            match previous_turn {
                Some(previous) => changes.iter().filter(|c| c.turn_index == previous).count(),
                None => 0,
            }
        }
    }
}

/// Filter a full (all-turns) change list down to the given scope.
///
/// `AllTurns` aggregates per-turn rows to one entry per unique path,
/// summing churn (additions and deletions) across all turns that touched
/// the file, while keeping the latest turn's metadata.
pub fn filter_changes_for_scope(
    changes: &[SessionFileChange],
    scope: ChangesScope,
) -> Vec<SessionFileChange> {
    if changes.is_empty() {
        return Vec::new();
    }
    let latest_turn = changes.iter().map(|c| c.turn_index).max().unwrap_or(0);
    // Turns that edited nothing have no rows, so "previous" is the most
    // recent turn below the latest that has changes, not `latest - 1`.
    let previous_turn = changes
        .iter()
        .map(|c| c.turn_index)
        .filter(|&t| t < latest_turn)
        .max();
    match scope {
        ChangesScope::AllTurns => {
            let mut out: Vec<SessionFileChange> = Vec::new();
            let mut index_by_path: std::collections::HashMap<String, usize> =
                std::collections::HashMap::new();
            for change in changes {
                if let Some(&idx) = index_by_path.get(&change.path) {
                    out[idx].additions += change.additions;
                    out[idx].deletions += change.deletions;
                    if change.status == "added" && out[idx].status != "deleted" {
                        out[idx].status = "added".into();
                    }
                    if out[idx].diff_text.is_none() && change.diff_text.is_some() {
                        out[idx].diff_text = change.diff_text.clone();
                    }
                } else {
                    index_by_path.insert(change.path.clone(), out.len());
                    out.push(change.clone());
                }
            }
            out
        }
        ChangesScope::ThisTurn => changes
            .iter()
            .filter(|c| c.turn_index == latest_turn)
            .cloned()
            .collect(),
        ChangesScope::PreviousTurn => match previous_turn {
            Some(previous) => changes
                .iter()
                .filter(|c| c.turn_index == previous)
                .cloned()
                .collect(),
            None => Vec::new(),
        },
    }
}
