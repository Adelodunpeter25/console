//! Computer tab: the server host's desktop, streamed into a webview.

use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::{AppContext, Context};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the Computer tab in the right-hand split.
    pub fn open_computer_tab(&mut self, cx: &mut Context<Self>) {
        let pane_id = self.target_subtab_pane_id();
        let tab = WorkspaceTabConfig::Computer {
            title: "Computer".to_string(),
            project_id: self.pane_workspace_key(&pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        self.persist_workspaces();
        self.sync_workspace_webviews(cx);
        cx.notify();
    }

    /// Show the native surface only while a Computer tab is the active tab of
    /// some pane and settings are closed.
    pub(crate) fn sync_computer_webview(&self, cx: &mut Context<Self>) {
        let visible = !self.settings_open
            && self.workspace_root.leaves().iter().any(|leaf| {
                matches!(
                    leaf.active_tab_id
                        .as_deref()
                        .and_then(|id| leaf.tabs.iter().find(|t| t.id() == id)),
                    Some(WorkspaceTabConfig::Computer { .. })
                )
            });
        if let Some(ref view) = self.computer_view {
            view.update(cx, |view, cx| {
                view.sync_native_state(visible, false, cx);
            });
        }
    }

    pub fn computer_view(
        &mut self,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) -> gpui::Entity<console_ui::ComputerViewer> {
        if let Some(ref view) = self.computer_view {
            return view.clone();
        }
        let client = self.client.clone();
        let view = cx.new(|cx| console_ui::ComputerViewer::new(client, window, cx));
        self.computer_view = Some(view.clone());
        view
    }
}
