//! The plain structs that cross into TSX; NSH derives their TypeScript
//! shapes into `.nsh/types/console-nsh/native.d.ts`.

use nsh_core::prelude::*;

/// What the sidebar shows for a project.
#[derive(Clone, JsObject)]
pub struct ProjectSummary {
    pub id: String,
    pub name: String,
    pub path: String,
}

/// What the sidebar shows for a chat.
#[derive(Clone, JsObject)]
pub struct SessionSummary {
    pub id: String,
    pub title: String,
    pub status: String,
    /// Unix millis.
    pub updated_at: i64,
    /// The project the chat belongs to, when the app knows it.
    pub project_id: Option<String>,
    pub pinned: bool,
    pub settled: bool,
}
