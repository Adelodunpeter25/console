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
}

impl SessionHeaderExt for SessionHeader {
    fn display_title(&self) -> &str {
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

