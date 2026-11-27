//! Devices tab: opening it in the active pane.

use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::{AppContext, Context};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the Devices tab in the right-hand split.
    pub fn open_device_tab(&mut self, cx: &mut Context<Self>) {
        let pane_id = self.target_subtab_pane_id();
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

    /// Show the native device surface only while a Devices tab is the active
    /// tab of some pane and settings are closed.
    pub(crate) fn sync_device_webview(&self, cx: &mut Context<Self>) {
        let visible = !self.settings_open
            && self.workspace_root.leaves().iter().any(|leaf| {
                matches!(
                    leaf.active_tab_id
                        .as_deref()
                        .and_then(|id| leaf.tabs.iter().find(|t| t.id() == id)),
                    Some(WorkspaceTabConfig::Device { .. })
                )
            });
        if let Some(ref device) = self.device_view {
            device.update(cx, |view, cx| {
                view.sync_native_state(visible, cx);
            });
        }
    }

    pub fn device_view(
        &mut self,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) -> gpui::Entity<console_ui::DeviceViewer> {
        if let Some(ref view) = self.device_view {
            view.clone()
        } else {
            let client = self.client.clone();
            let entity = cx.entity().downgrade();
            let view = cx.new(|cx| {
                console_ui::DeviceViewer::new(client, window, cx).on_screenshot(
                    move |bytes, name, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.stage_device_screenshot(bytes, name, cx);
                            });
                        }
                    },
                )
            });
            self.device_view = Some(view.clone());
            view
        }
    }
}
