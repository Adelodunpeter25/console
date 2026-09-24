//! "View all" session changes review tab: opening, fetching, and toggling
//! per-file reviewed state (§6 of `docs/session-changes-turn-diff-plan.md`).

use console_core::types::ChangesScope;
use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open (or focus, if already open) the review tab for the given session
    /// and scope in the active pane.
    pub fn open_changes_review_tab(&mut self, session_id: String, scope: ChangesScope, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());

        let title = format!("Changes: {}", scope.label());
        let tab = WorkspaceTabConfig::ChangesReview {
            session_id: session_id.clone(),
            scope,
            title,
            project_id: self.pane_project_id(&pane_id),
            last_active_at_ms: None,
        };
        let tab_id = tab.id();

        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        self.persist_workspaces();

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
        cx.spawn(async move |entity, cx| match client.sessions.get_changes(&session_id, None).await {
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
        })
        .detach();
    }

    /// Toggle a file block's local expand/collapse state within a review tab.
    pub fn toggle_changes_review_collapsed(
        &mut self,
        tab_id: &str,
        path: &str,
        cx: &mut Context<Self>,
    ) {
        let set = self.changes_review_collapsed.entry(tab_id.to_string()).or_default();
        if !set.remove(path) {
            set.insert(path.to_string());
        }
        cx.notify();
    }

    /// Toggle the reviewed flag for one `(path, turn_index)` row: optimistic
    /// local update (also auto-collapses/expands the file block), then
    /// confirmed via `POST /changes/reviewed`.
    pub fn toggle_change_reviewed(
        &mut self,
        tab_id: &str,
        session_id: String,
        path: String,
        turn_index: u64,
        cx: &mut Context<Self>,
    ) {
        let new_reviewed = {
            let Some(data) = self.changes_review_data.get(tab_id) else {
                return;
            };
            let Some(entry) = data.iter().find(|c| c.path == path && c.turn_index == turn_index) else {
                return;
            };
            !entry.reviewed
        };

        if let Some(data) = self.changes_review_data.get_mut(tab_id) {
            let updated: Vec<_> = data
                .iter()
                .map(|c| {
                    let mut c = c.clone();
                    if c.path == path && c.turn_index == turn_index {
                        c.reviewed = new_reviewed;
                    }
                    c
                })
                .collect();
            *data = std::rc::Rc::new(updated);
        }
        let collapsed = self.changes_review_collapsed.entry(tab_id.to_string()).or_default();
        if new_reviewed {
            collapsed.insert(path.clone());
        } else {
            collapsed.remove(&path);
        }
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |_entity, _cx| {
            if let Err(err) = client
                .sessions
                .set_change_reviewed(&session_id, &path, turn_index, new_reviewed)
                .await
            {
                log::warn!("Failed to persist reviewed state: {}", err);
            }
        })
        .detach();
    }
}
