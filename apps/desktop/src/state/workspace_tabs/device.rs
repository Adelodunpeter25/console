//! Devices tab: opening it in the active pane.

use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the Devices tab in the active pane.
    pub fn open_device_tab(&mut self, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        let tab = WorkspaceTabConfig::Device {
            title: "Devices".to_string(),
            project_id: self.pane_workspace_key(&pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        self.persist_workspaces();
        self.sync_workspace_webviews(cx);
        cx.notify();
    }
}
