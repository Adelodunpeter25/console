//! "View all" session changes review tab: opening, fetching, and toggling
//! per-file reviewed state.

use console_core::WorkspaceTabConfig;
use console_core::types::ChangesScope;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the review tab for the given session
    /// and scope in the right-hand split beside the chat.
    pub fn open_changes_review_tab(
        &mut self,
        session_id: String,
        scope: ChangesScope,
        cx: &mut Context<Self>,
    ) {
        let pane_id = self.target_subtab_pane_id();

        let title = format!("Changes: {}", scope.label());
        let tab = WorkspaceTabConfig::ChangesReview {
            session_id: session_id.clone(),
            scope,
            title,
            project_id: self.pane_workspace_key(&pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        let tab_id = tab.id();

        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        self.persist_workspaces();

        self.changes_review_data
            .entry(tab_id.clone())
            .or_insert_with(|| std::rc::Rc::new(Vec::new()));
        self.fetch_changes_review_data(&tab_id, session_id, cx);
        cx.notify();
    }

    /// Fetch the full (all-turns) change list for a review tab and seed its
    /// initial collapsed set (reviewed files start collapsed).
    pub(crate) fn fetch_changes_review_data(
        &mut self,
        tab_id: &str,
        session_id: String,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let tab_id = tab_id.to_string();
        cx.spawn(async move |entity, cx| {
            match client.sessions.get_changes(&session_id, None).await {
                Ok(changes) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let collapsed: std::collections::HashSet<String> = changes
                                    .iter()
                                    .filter(|c| c.reviewed)
                                    .map(|c| c.path.clone())
                                    .collect();
                                this.changes_review_collapsed
                                    .insert(tab_id.clone(), collapsed);
                                this.changes_review_data
                                    .insert(tab_id.clone(), std::rc::Rc::new(changes));
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch changes review data: {}", err);
                }
            }
        })
        .detach();
    }

    /// Toggle a file's local expand/collapse state within a review tab.
    pub fn toggle_changes_review_collapsed(
        &mut self,
        tab_id: &str,
        path: &str,
        cx: &mut Context<Self>,
    ) {
        let set = self
            .changes_review_collapsed
            .entry(tab_id.to_string())
            .or_default();
        if !set.remove(path) {
            set.insert(path.to_string());
        }
        cx.notify();
    }
}
