use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    pub(crate) fn update_session_title_in_panes(&mut self, session_id: &str, title: &str, _cx: &mut Context<Self>) {
        console_ui::workspace::ops::rename_tabs(
            &mut self.workspace_root,
            |tab| matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: tab_id, .. } if tab_id == session_id),
            title.to_string(),
        );
        for root in self.project_workspace_roots.values_mut() {
            console_ui::workspace::ops::rename_tabs(
                root,
                |tab| matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: tab_id, .. } if tab_id == session_id),
                title.to_string(),
            );
        }
    }
}
