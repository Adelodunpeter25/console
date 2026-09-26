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

use super::agent::AgentMessage;
use super::model::ThinkingLevel;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum SessionStatus {
    Idle,
    Working,
    Done,
    NeedsAttention,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionHeader {
    pub id: String,
    pub title: String,
    pub cwd: String,
    pub project_id: Option<String>,
    pub model_id: String,
    pub provider: String,
    pub created_at: i64,
    pub updated_at: i64,
    pub message_count: Option<usize>,
    pub status: Option<SessionStatus>,
    pub approval_mode: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub thinking_level: Option<ThinkingLevel>,
    pub deleted_at: Option<i64>,
}

impl SessionHeader {
    pub fn display_title(&self) -> &str {
        let trimmed = self.title.trim();
        if trimmed.is_empty() {
            "New Chat"
        } else {
            trimmed
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDetailResponse {
    #[serde(alias = "session")]
    pub header: SessionHeader,
    pub messages: Vec<AgentMessage>,
    /// Whether older messages exist before the oldest row in `messages`.
    #[serde(default)]
    pub has_more: bool,
    /// Rowid cursor for the next older batch; null when at the start of history.
    #[serde(default)]
    pub next_cursor: Option<i64>,
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
    #[serde(skip_serializing_if = "Option::is_none")]
    pub branch: Option<String>,
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
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionFileChange {
    pub path: String,
    pub status: String,
    pub additions: u64,
    pub deletions: u64,
    pub turn_index: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub diff_text: Option<String>,
    #[serde(default)]
    pub reviewed: bool,
    pub updated_at: i64,
}

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

/// Filter a full (all-turns) change list down to the given scope.
///
/// `AllTurns` also dedupes to one row per path (keeping the first —
/// callers pass changes ordered most-recently-updated-first, matching the
/// server's `ORDER BY updated_at DESC`), since the backend returns raw
/// per-turn rows rather than a pre-aggregated-by-file view.
pub fn filter_changes_for_scope(
    changes: &[SessionFileChange],
    scope: ChangesScope,
) -> Vec<SessionFileChange> {
    if changes.is_empty() {
        return Vec::new();
    }
    let latest_turn = changes.iter().map(|c| c.turn_index).max().unwrap_or(0);
    match scope {
        ChangesScope::AllTurns => {
            let mut seen = std::collections::HashSet::new();
            changes
                .iter()
                .filter(|c| seen.insert(c.path.clone()))
                .cloned()
                .collect()
        }
        ChangesScope::ThisTurn => changes
            .iter()
            .filter(|c| c.turn_index == latest_turn)
            .cloned()
            .collect(),
        ChangesScope::PreviousTurn => {
            if latest_turn == 0 {
                Vec::new()
            } else {
                changes
                    .iter()
                    .filter(|c| c.turn_index == latest_turn - 1)
                    .cloned()
                    .collect()
            }
        }
    }
}

