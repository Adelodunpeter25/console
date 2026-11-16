//! Shared resolution of "what is the active pane working on": the session and
//! checkout behind the focused tab, plus a few handlers that hang off it.

use std::rc::Rc;

use gpui::Context;

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Resolve the checkout a session is working in.
    ///
    /// The session's own `cwd` is authoritative: for a worktree session it is
    /// the worktree path, not the project root. Anything that needs a git
    /// status/diff target for a session must come through here rather than
    /// reading `project.path` directly, or worktree changes render empty.
    fn session_cwd(
        &self,
        session_id: &str,
        project_id: Option<&str>,
        pane_id: &str,
    ) -> Option<String> {
        self.sessions
            .iter()
            .find(|s| s.id == session_id)
            .and_then(|s| {
                if !s.cwd.is_empty() {
                    Some(s.cwd.clone())
                } else if let Some(pid) = &s.project_id {
                    self.projects
                        .iter()
                        .find(|p| &p.id == pid)
                        .map(|p| p.path.clone())
                } else {
                    None
                }
            })
            .or_else(|| project_id.and_then(|key| self.workspace_path_for_key(key)))
            .or_else(|| {
                self.selected_project_for_pane(pane_id)
                    .map(|p| p.path.clone())
            })
    }

    pub fn active_pane_target(&self) -> (Option<String>, Option<String>) {
        let pane_id = self.active_pane_id.as_deref().unwrap_or("pane-main");

        let leaf = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|l| l.id == pane_id);

        let active_tab = leaf.as_ref().and_then(|l| {
            l.active_tab_id
                .as_deref()
                .and_then(|tab_id| l.tabs.iter().find(|t| t.id() == tab_id))
        });

        match active_tab {
            Some(console_core::WorkspaceTabConfig::Chat {
                session_id,
                project_id,
                ..
            }) => {
                let cwd = self.session_cwd(session_id, project_id.as_deref(), pane_id);
                (Some(session_id.clone()), cwd)
            }
            // A changes-review tab is scoped to a session, so it resolves its
            // checkout from that session like a chat does. Folding it into the
            // project-path arm below dropped its session id and pointed the
            // git status/diff calls at the project root, so a worktree
            // session's changes rendered empty.
            Some(console_core::WorkspaceTabConfig::ChangesReview {
                session_id,
                project_id,
                ..
            }) => {
                let cwd = self.session_cwd(session_id, project_id.as_deref(), pane_id);
                (Some(session_id.clone()), cwd)
            }
            Some(console_core::WorkspaceTabConfig::Settings { .. }) => (None, None),
            Some(console_core::WorkspaceTabConfig::File { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Diff { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Terminal { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Browser { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::FileTree { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Device { project_id, .. }) => {
                // When focused on a tab in the workspace (Diff, Terminal, File), retain the pane's
                // active session and its cwd so the changes inspector doesn't clear out.
                let session_id = self.active_session_for_pane(pane_id);
                let cwd = session_id
                    .as_deref()
                    .and_then(|sid| self.session_cwd(sid, project_id.as_deref(), pane_id))
                    .or_else(|| {
                        project_id
                            .as_ref()
                            .and_then(|key| self.workspace_path_for_key(key))
                    })
                    .or_else(|| {
                        self.selected_project_for_pane(pane_id)
                            .map(|p| p.path.clone())
                    });
                (session_id, cwd)
            }
            None => (None, None),
        }
    }

        /// Open the active pane's subagents popover — the same card the
    /// subagents pill opens — from a transcript subagent-card click.
    pub fn open_subagents_popover(&self, window: &mut gpui::Window, cx: &mut gpui::Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        console_ui::toggle_popover(
            &self.pane_subagents_menu(&pane_id),
            console_ui::MenuAlign::AboveLeft,
            window,
            cx,
        );
    }

    /// Fetch a session's recorded file changes (without diff text; chips load
    /// a diff on demand) and hand them to the pane's transcript so each turn
    /// can show its changed-file chips.
    pub fn refresh_turn_changes_for_pane(
        &mut self,
        pane_id: &str,
        session_id: &str,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let pane_id = pane_id.to_string();
        let session_id = session_id.to_string();
        cx.spawn(async move |entity, cx| {
            match client.sessions.get_changes_without_diff(&session_id).await {
                Ok(changes) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if this.active_session_for_pane(&pane_id).as_deref()
                                    != Some(session_id.as_str())
                                {
                                    return;
                                }
                                let changes = Rc::new(changes);
                                this.transcript_for_pane(&pane_id)
                                    .update(cx, |t, cx| t.set_session_changes(changes, cx));
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch turn changes: {}", err);
                }
            }
        })
        .detach();
    }
}