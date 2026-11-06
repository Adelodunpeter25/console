//! Workspace layout persistence, through nsh-core's `Storage` (its
//! key-value store over the host's app-data directory).
//!
//! The GPUI app persisted workspaces through
//! `apps/desktop/src/persistence/workspace.rs`; the NSH app keeps the same
//! contract — one JSON document per workspace key, keyed the same way
//! (`project_id`, or `project_id::wt::worktree_path` for a worktree
//! checkout, built by `console_core::worktree_workspace_key`) — but the
//! document itself is owned by the TSX side (the pane-tree model in
//! `src/workspace/model.ts`, which serializes to the same camelCase shape
//! `LeafPaneNode`/`SplitPaneNode` parse). Only this side talks to Storage,
//! because Storage is Rust-only; the TSX store reads and writes whole
//! documents, never partial edits, so no locking is needed beyond nsh
//! core's own.

use nsh_core::storage::Storage;

pub struct WorkspacePersist;

impl WorkspacePersist {
    /// Saves one workspace's layout document (the pane-tree JSON). Overwrites
    /// any previous document for the key; the TSX store writes on every
    /// mutation, so the newest document wins and no merges happen.
    pub fn save(key: &str, document: &str) -> Result<(), String> {
        Storage::set_item(format!("workspace:{key}"), document)
            .map_err(|e| format!("saving workspace layout: {e}"))
    }

    /// Loads one workspace's layout document, or `None` when the workspace
    /// was never saved (a fresh project's first launch).
    pub fn load(key: &str) -> Option<String> {
        Storage::get_item(&format!("workspace:{key}"))
    }

    /// Deletes one workspace's layout document (the workspace was closed or
    /// its worktree removed; the desktop's `persistence` drops them the
    /// same way). Missing keys are fine.
    pub fn delete(key: &str) {
        Storage::remove_item(&format!("workspace:{key}"));
    }
}
