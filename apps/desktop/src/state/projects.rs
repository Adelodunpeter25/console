//! Project and branch selection: the footer pickers, the native folder
//! picker for adding a project, and Git branch checkout.

use std::rc::Rc;

use console_core::{ProjectInfo, UpdateSessionDto};
use gpui::Context;

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub(crate) fn selected_project_for_pane(&self, pane_id: &str) -> Option<&ProjectInfo> {
        self.pane_project_id(pane_id)
            .as_ref()
            .and_then(|id| self.projects.iter().find(|project| &project.id == id))
    }

    pub fn select_project_for_pane(
        &mut self,
        pane_id: String,
        project_id: String,
        cx: &mut Context<Self>,
    ) {
        // Lock the working directory once a chat has messages. Each run
        // reloads header.cwd for prompt-ref expansion, project context, and
        // all tool paths — changing it mid-chat mixes old context with a
        // new project.
        if self.session_has_messages(&pane_id) {
            return;
        }
        let old_workspace_id = self.selected_project_id.clone();
        let new_workspace_id = Some(project_id.clone());
        let session_id = self.active_session_for_pane(&pane_id);
        // Remember the user's pick until the server confirms it, so a fast
        // close + reopen can't resurrect the stale header project.
        if let Some(ref sid) = session_id {
            self.pending_project_override
                .insert(sid.clone(), Some(project_id.clone()));
        }
        let active_tab_id = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .and_then(|leaf| leaf.active_tab_id.clone());

        if old_workspace_id != new_workspace_id {
            // Move the pane's active tab into the new folder's workspace so a
            // workspace never holds tabs from another folder. Immediate switch
            // to the new workspace keeps the moved chat visible.
            let mut moved_tab = active_tab_id.and_then(|tid| {
                console_ui::workspace::ops::take_tab(&mut self.workspace_root, &tid)
            });
            if let Some(tab) = moved_tab.as_mut() {
                tab.set_project_id(new_workspace_id.clone());
            }
            // Remember this workspace's focused pane before leaving it.
            self.remember_active_pane();
            // Stash the remaining (old-folder) tabs under the old workspace.
            self.project_workspace_roots
                .insert(old_workspace_id, self.workspace_root.clone());
            // Load the target workspace, starting from a fresh leaf when unseen.
            let mut target_root = self
                .project_workspace_roots
                .get(&new_workspace_id)
                .cloned()
                .unwrap_or_else(|| console_core::WorkspaceNode::leaf(pane_id.clone()));
            // Drop any stale tabs from other folders (heals legacy mixed saves).
            console_ui::workspace::ops::retain_project_tabs(&mut target_root, &new_workspace_id);
            let target_pane_id = target_root
                .leaves()
                .iter()
                .find(|leaf| leaf.id == pane_id)
                .map(|leaf| leaf.id.clone())
                .or_else(|| target_root.first_leaf().map(|leaf| leaf.id.clone()))
                .unwrap_or_else(|| pane_id.clone());
            if let Some(tab) = moved_tab {
                console_ui::workspace::ops::open_tab(&mut target_root, &target_pane_id, tab);
            }
            // Remember the target root for future switches before swapping in.
            self.project_workspace_roots
                .insert(new_workspace_id.clone(), target_root.clone());
            self.workspace_root = target_root;
            self.active_pane_id = Some(target_pane_id.clone());
            // The moved tab is the visible focus of the new workspace.
            self.project_active_panes
                .insert(new_workspace_id.clone(), target_pane_id.clone());
            if let Some(state) = self.workspace_pane_states.get_mut(&pane_id) {
                state.selected_project_id = new_workspace_id.clone();
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = false;
                state.branch_is_git_repository = false;
            }
            if let Some(state) = self.workspace_pane_states.get_mut(&target_pane_id) {
                state.selected_project_id = new_workspace_id.clone();
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = false;
                state.branch_is_git_repository = false;
            }
            self.selected_project_id = new_workspace_id.clone();
            self.persist_layout();
            self.persist_workspaces();
        } else {
            if let Some(state) = self.workspace_pane_states.get_mut(&pane_id) {
                state.selected_project_id = Some(project_id.clone());
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = false;
                state.branch_is_git_repository = false;
            }
            self.selected_project_id = Some(project_id.clone());
            // Same-folder change: keep the tab's stored project in step.
            if let Some(tid) = active_tab_id {
                console_ui::workspace::ops::set_tab_project(
                    &mut self.workspace_root,
                    &tid,
                    new_workspace_id.clone(),
                );
            }
            self.persist_workspaces();
        }
        cx.notify();

        let effective_pane = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| pane_id.clone());
        let Some(project) = self.selected_project_for_pane(&effective_pane).cloned() else {
            return;
        };

        // Point the active session at this project so the sidebar reflects the
        // change immediately; persist the cwd change on the backend.
        // Use the pre-move session id: after the workspace switch the old
        // pane id may no longer exist in the active tree.
        if let Some(sid) = session_id.clone() {
            if let Some(session) = Rc::make_mut(&mut self.sessions)
                .iter_mut()
                .find(|s| s.id == sid)
            {
                session.project_id = Some(project.id.clone());
                session.cwd = project.path.clone();
            }
        }

        let path = project.path.clone();
        self.transcript_for_pane(&effective_pane)
            .update(cx, |transcript, _| {
                transcript.set_session_cwd(Some(path.clone()));
            });
        self.maybe_refresh_inspector(cx);

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let pane_id_for_result = effective_pane.clone();
        let project_id_for_dto = project.id.clone();
        cx.spawn(async move |_entity, cx| {
            if let Some(session_id) = session_id {
                match client
                    .sessions
                    .update(
                        &session_id,
                        UpdateSessionDto {
                            title: None,
                            cwd: Some(path.clone()),
                            project_id: Some(Some(project_id_for_dto)),
                            model_id: None,
                            provider: None,
                            approval_mode: None,
                        },
                    )
                    .await
                {
                    Ok(header) => {
                        let sid = session_id.clone();
                        cx.update(|cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, _| {
                                    this.confirm_project_override(&sid, &header)
                                });
                            }
                        });
                    }
                    Err(error) => {
                        let message = format!("Unable to update session workspace: {error}");
                        cx.update(|cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.pending_project_override.remove(&session_id);
                                    this.set_error(message, cx)
                                });
                            }
                        });
                    }
                }
            }

            match client.git.list_branches(Some(&path)).await {
                Ok(branches) => cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) =
                                this.workspace_pane_states.get_mut(&pane_id_for_result)
                            {
                                state.branches = Rc::new(branches.branches);
                                state.branch_loaded = true;
                                state.branch_is_git_repository = branches.is_git_repository;
                            }
                            cx.notify();
                        });
                    }
                }),
                Err(_) => cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) =
                                this.workspace_pane_states.get_mut(&pane_id_for_result)
                            {
                                Rc::make_mut(&mut state.branches).clear();
                                state.branch_loaded = true;
                                state.branch_is_git_repository = false;
                            }
                            cx.notify();
                        });
                    }
                }),
            }
        })
        .detach();
    }

    /// Drop the project selection: new sessions use the sandboxed scratch
    /// directory (`~/.console/scratch/<sessionId>`) with no project scope.
    pub fn clear_project_for_pane(&mut self, pane_id: String, cx: &mut Context<Self>) {
        // Same lock as select_project_for_pane — clearing the project
        // changes cwd, which is unsafe once a chat has messages.
        if self.session_has_messages(&pane_id) {
            return;
        }
        let old_workspace_id = self.selected_project_id.clone();
        let new_workspace_id: Option<String> = None;
        let pre_move_session = self.active_session_for_pane(&pane_id);
        let active_tab_id = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .and_then(|leaf| leaf.active_tab_id.clone());

        if old_workspace_id != new_workspace_id {
            let mut moved_tab = active_tab_id.and_then(|tid| {
                console_ui::workspace::ops::take_tab(&mut self.workspace_root, &tid)
            });
            if let Some(tab) = moved_tab.as_mut() {
                tab.set_project_id(None);
            }
            // Remember this workspace's focused pane before leaving it.
            self.remember_active_pane();
            self.project_workspace_roots
                .insert(old_workspace_id, self.workspace_root.clone());
            let mut target_root = self
                .project_workspace_roots
                .get(&new_workspace_id)
                .cloned()
                .unwrap_or_else(|| console_core::WorkspaceNode::leaf(pane_id.clone()));
            console_ui::workspace::ops::retain_project_tabs(&mut target_root, &new_workspace_id);
            let target_pane_id = target_root
                .leaves()
                .iter()
                .find(|leaf| leaf.id == pane_id)
                .map(|leaf| leaf.id.clone())
                .or_else(|| target_root.first_leaf().map(|leaf| leaf.id.clone()))
                .unwrap_or_else(|| pane_id.clone());
            if let Some(tab) = moved_tab {
                console_ui::workspace::ops::open_tab(&mut target_root, &target_pane_id, tab);
            }
            self.project_workspace_roots
                .insert(new_workspace_id.clone(), target_root.clone());
            self.workspace_root = target_root;
            self.active_pane_id = Some(target_pane_id.clone());
            // The moved tab is the visible focus of the new workspace.
            self.project_active_panes
                .insert(new_workspace_id.clone(), target_pane_id.clone());
            if let Some(state) = self.workspace_pane_states.get_mut(&pane_id) {
                state.selected_project_id = None;
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = true;
                state.branch_is_git_repository = false;
            }
            if let Some(state) = self.workspace_pane_states.get_mut(&target_pane_id) {
                state.selected_project_id = None;
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = true;
                state.branch_is_git_repository = false;
            }
            self.selected_project_id = None;
            self.persist_layout();
            self.persist_workspaces();
        } else {
            if let Some(state) = self.workspace_pane_states.get_mut(&pane_id) {
                state.selected_project_id = None;
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = true;
                state.branch_is_git_repository = false;
            }
            if let Some(tid) = active_tab_id {
                console_ui::workspace::ops::set_tab_project(&mut self.workspace_root, &tid, None);
            }
            self.persist_workspaces();
        }
        cx.notify();

        let effective_pane = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| pane_id.clone());
        let Some(session_id) = pre_move_session else {
            return;
        };
        // Remember the clear until the server confirms, like select above.
        self.pending_project_override
            .insert(session_id.clone(), None);
        let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
        let is_dev = std::env::var("CONSOLE_ENV")
            .map(|v| v == "dev")
            .unwrap_or(false);
        let folder = if is_dev { ".console-dev" } else { ".console" };
        let fallback_cwd = format!("{}/{}/scratch/{}", home, folder, session_id);

        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|s| s.id == session_id)
        {
            session.project_id = None;
            session.cwd = fallback_cwd.clone();
        }

        self.transcript_for_pane(&effective_pane)
            .update(cx, |transcript, _| {
                transcript.set_session_cwd(Some(fallback_cwd.clone()));
            });
        self.maybe_refresh_inspector(cx);

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            match client
                .sessions
                .update(
                    &session_id,
                    UpdateSessionDto {
                        title: None,
                        cwd: Some(fallback_cwd),
                        // Explicit null: omitting the key would let the server
                        // re-infer a project from the cwd.
                        project_id: Some(None),
                        model_id: None,
                        provider: None,
                        approval_mode: None,
                    },
                )
                .await
            {
                Ok(header) => {
                    let sid = session_id.clone();
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, _| this.confirm_project_override(&sid, &header));
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to update session workspace: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.pending_project_override.remove(&session_id);
                                this.set_error(message, cx)
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }

    /// Register `path` as a tracked project and select it for the active pane.
    /// Shared by the ⌘O project browser and the native folder picker.
    pub fn add_project_from_path(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| match client.projects.add(&path).await {
            Ok(project) => cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        if !this.projects.iter().any(|p| p.id == project.id) {
                            Rc::make_mut(&mut this.projects).push(project.clone());
                        }
                        this.select_project_for_pane(pane_id.clone(), project.id, cx);
                        crate::window::broadcast_settings_refresh(cx);
                    });
                }
            }),
            Err(error) => {
                let message = format!("Unable to add project: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.set_error(message, cx));
                    }
                });
            }
        })
        .detach();
    }

    /// Check out a branch in the selected pane's project and refresh its branch list.
    pub fn checkout_branch_for_pane(
        &mut self,
        pane_id: String,
        branch: String,
        cx: &mut Context<Self>,
    ) {
        let Some(path) = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
        else {
            return;
        };
        if let Some(state) = self.workspace_pane_states.get_mut(&pane_id) {
            state.branch_pending = true;
        }
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = match client.git.checkout_branch(Some(&path), &branch).await {
                Ok(()) => client
                    .git
                    .list_branches(Some(&path))
                    .await
                    .map_err(|error| error.to_string()),
                Err(error) => Err(error.to_string()),
            };
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        if let Some(state) = this.workspace_pane_states.get_mut(&pane_id) {
                            state.branch_pending = false;
                        }
                        match result {
                            Ok(branches) => {
                                if let Some(state) = this.workspace_pane_states.get_mut(&pane_id) {
                                    state.branches = Rc::new(branches.branches);
                                    state.branch_loaded = true;
                                    state.branch_is_git_repository = branches.is_git_repository;
                                }
                                this.maybe_refresh_inspector(cx);
                            }
                            Err(error) => {
                                this.set_error(format!("Unable to switch branch: {error}"), cx)
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn load_projects(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(projects) = client.projects.list().await {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.projects = Rc::new(projects);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn remove_project(&mut self, project_id: String, cx: &mut Context<Self>) {
        Rc::make_mut(&mut self.projects).retain(|p| p.id != project_id);
        if self.selected_project_id.as_deref() == Some(&project_id) {
            self.selected_project_id = None;
        }
        self.project_workspace_roots
            .remove(&Some(project_id.clone()));
        for state in self.workspace_pane_states.values_mut() {
            if state.selected_project_id.as_deref() == Some(&project_id) {
                state.selected_project_id = None;
                state.branches = Rc::new(Vec::new());
                state.branch_loaded = false;
            }
        }
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Err(error) = client.projects.remove(&project_id).await {
                let message = format!("Unable to remove project: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.set_error(message, cx));
                    }
                });
            } else {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |_, cx| crate::window::broadcast_settings_refresh(cx));
                    }
                });
            }
        })
        .detach();
    }
}
