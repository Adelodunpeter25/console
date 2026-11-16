//! Files tab: opening it and toggling its folders.

use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the Files tab in the active pane.
    pub fn open_file_tree_tab(&mut self, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());

        let tab = WorkspaceTabConfig::FileTree {
            title: "Files".to_string(),
            project_id: self.pane_workspace_key(&pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        let tab_id = tab.id();

        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        let tree = self.inspector_tree.clone();
        self.file_tree_data.entry(tab_id).or_insert(tree);
        self.persist_workspaces();

        self.refresh_inspector(cx);
        cx.notify();
    }

    pub fn toggle_file_tree_folder(&mut self, tab_id: &str, path: String, cx: &mut Context<Self>) {
        let expanded = self.file_tree_expanded.entry(tab_id.to_string()).or_default();
        if !expanded.remove(&path) {
            expanded.insert(path);
        }
        cx.notify();
    }
}
