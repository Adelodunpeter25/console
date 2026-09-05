use std::cell::RefCell;
use std::rc::Rc;

use console_ui::InspectorTab;
use gpui::{AppContext, Context};

use super::{
    ConsoleDesktopApp, RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT, RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT,
    RIGHT_SIDEBAR_MAX_WIDTH, RIGHT_SIDEBAR_MIN_WIDTH,
};

impl ConsoleDesktopApp {
    pub fn toggle_right_sidebar(&mut self, cx: &mut Context<Self>) {
        self.right_sidebar_visible = !self.right_sidebar_visible;
        self.persist_layout();
        self.maybe_refresh_inspector(cx);
        cx.notify();
    }

    #[allow(dead_code)]
    pub fn set_right_sidebar_visible(&mut self, visible: bool, cx: &mut Context<Self>) {
        if self.right_sidebar_visible == visible {
            return;
        }
        self.right_sidebar_visible = visible;
        self.persist_layout();
        self.maybe_refresh_inspector(cx);
        cx.notify();
    }

    pub fn begin_right_sidebar_resize(&mut self, start_x: f32) {
        self.right_sidebar_resize_start = Some((start_x, self.right_sidebar_width));
    }

    pub fn resize_right_sidebar(&mut self, current_x: f32) -> bool {
        let Some((start_x, start_width)) = self.right_sidebar_resize_start else {
            return false;
        };
        // Right sidebar expands when dragged left (decreasing x)
        let width = (start_width + (start_x - current_x))
            .clamp(RIGHT_SIDEBAR_MIN_WIDTH, RIGHT_SIDEBAR_MAX_WIDTH);
        if (self.right_sidebar_width - width).abs() < 0.5 {
            return false;
        }
        self.right_sidebar_width = width;
        true
    }

    pub fn finish_right_sidebar_resize(&mut self) -> bool {
        if self.right_sidebar_resize_start.take().is_some() {
            self.persist_layout();
            true
        } else {
            false
        }
    }

    pub fn begin_right_sidebar_bottom_resize(&mut self, start_y: f32) {
        self.right_sidebar_bottom_resize_start = Some((start_y, self.right_sidebar_bottom_height));
    }

    pub fn resize_right_sidebar_bottom(&mut self, current_y: f32) -> bool {
        let Some((start_y, start_height)) = self.right_sidebar_bottom_resize_start else {
            return false;
        };
        // Bottom split expands when dragged up (decreasing y)
        let height = (start_height + (start_y - current_y))
            .clamp(RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT, RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT);
        if (self.right_sidebar_bottom_height - height).abs() < 0.5 {
            return false;
        }
        self.right_sidebar_bottom_height = height;
        true
    }

    pub fn finish_right_sidebar_bottom_resize(&mut self) -> bool {
        if self.right_sidebar_bottom_resize_start.take().is_some() {
            self.persist_layout();
            true
        } else {
            false
        }
    }

    pub fn select_right_sidebar_terminal_tab(
        &mut self,
        index: usize,
        cx: &mut Context<Self>,
    ) {
        if index < self.right_sidebar_terminals.len() {
            self.right_sidebar_active_terminal_idx = index;
            cx.notify();
        }
    }

    pub fn add_right_sidebar_terminal(
        &mut self,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) {
        let (_, cwd) = self.active_inspector_target();
        let Some(cwd) = cwd else {
            return;
        };

        // Max 3 terminal tabs
        if self.right_sidebar_terminals.len() >= 3 {
            return;
        }

        let id = self.right_sidebar_next_terminal_id;
        self.right_sidebar_next_terminal_id += 1;

        let client = self.client.clone();
        let terminal = cx.new(|cx| {
            let params = console_core::types::terminal::TerminalSpawnParams {
                cwd,
                ..Default::default()
            };
            console_ui::terminal::TerminalView::new(params, client, window, cx)
        });

        self.right_sidebar_terminals.push((id, terminal));
        self.right_sidebar_active_terminal_idx = self.right_sidebar_terminals.len().saturating_sub(1);
        cx.notify();
    }

    pub fn close_right_sidebar_terminal(
        &mut self,
        index: usize,
        cx: &mut Context<Self>,
    ) {
        if index < self.right_sidebar_terminals.len() {
            self.right_sidebar_terminals.remove(index);
            if self.right_sidebar_active_terminal_idx >= self.right_sidebar_terminals.len() {
                self.right_sidebar_active_terminal_idx =
                    self.right_sidebar_terminals.len().saturating_sub(1);
            }
            cx.notify();
        }
    }

    pub fn active_inspector_target(&self) -> (Option<String>, Option<String>) {
        let pane_id = self
            .active_pane_id
            .as_deref()
            .unwrap_or("pane-main");

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
                let session = self.sessions.iter().find(|s| &s.id == session_id);
                let cwd = session
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
                    .or_else(|| {
                        project_id
                            .as_ref()
                            .and_then(|pid| {
                                self.projects
                                    .iter()
                                    .find(|p| &p.id == pid)
                                    .map(|p| p.path.clone())
                            })
                    })
                    .or_else(|| {
                        self.selected_project_for_pane(pane_id).map(|p| p.path.clone())
                    });
                (Some(session_id.clone()), cwd)
            }
            Some(console_core::WorkspaceTabConfig::File { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Diff { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Terminal { project_id, .. }) => {
                let cwd = project_id
                    .as_ref()
                    .and_then(|pid| {
                        self.projects
                            .iter()
                            .find(|p| &p.id == pid)
                            .map(|p| p.path.clone())
                    })
                    .or_else(|| self.selected_project_for_pane(pane_id).map(|p| p.path.clone()));
                (None, cwd)
            }
            None => (None, None),
        }
    }

    pub fn set_inspector_tab(&mut self, tab: InspectorTab, cx: &mut Context<Self>) {
        if self.inspector_active_tab == tab {
            return;
        }
        self.inspector_active_tab = tab;
        match tab {
            InspectorTab::AllFiles => self.fetch_inspector_fs_tree(cx),
            InspectorTab::Changes => {
                self.fetch_inspector_git_changes(cx);
                self.fetch_inspector_session_changes(cx);
            }
            InspectorTab::Subagents => self.fetch_inspector_subagents(cx),
        }
        cx.notify();
    }

    #[allow(dead_code)]
    pub fn set_inspector_search_query(&mut self, query: String, cx: &mut Context<Self>) {
        self.inspector_search_query = query;
        cx.notify();
    }

    pub fn ensure_right_sidebar_terminal(
        &mut self,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) {
        let (_, cwd) = self.active_inspector_target();
        // If cwd is None, clear terminals so empty state is displayed
        let Some(cwd) = cwd else {
            if !self.right_sidebar_terminals.is_empty() || self.right_sidebar_terminal_cwd.is_some() {
                self.right_sidebar_terminals.clear();
                self.right_sidebar_active_terminal_idx = 0;
                self.right_sidebar_terminal_cwd = None;
            }
            return;
        };

        // If cwd changed or no terminals exist, reset and spawn the initial Terminal 1
        if self.right_sidebar_terminals.is_empty() || self.right_sidebar_terminal_cwd.as_deref() != Some(&cwd) {
            self.right_sidebar_terminal_cwd = Some(cwd.clone());
            self.right_sidebar_terminals.clear();
            self.right_sidebar_next_terminal_id = 1;
            let id = self.right_sidebar_next_terminal_id;
            self.right_sidebar_next_terminal_id += 1;

            let client = self.client.clone();
            let terminal = cx.new(|cx| {
                let params = console_core::types::terminal::TerminalSpawnParams {
                    cwd,
                    ..Default::default()
                };
                console_ui::terminal::TerminalView::new(params, client, window, cx)
            });
            self.right_sidebar_terminals.push((id, terminal));
            self.right_sidebar_active_terminal_idx = 0;
        }
    }

    pub fn toggle_inspector_folder(&mut self, path: String, cx: &mut Context<Self>) {
        let expanded = Rc::make_mut(&mut self.inspector_expanded_folders);
        if !expanded.remove(&path) {
            expanded.insert(path);
        }
        cx.notify();
    }

    pub fn toggle_subagent_expanded(&mut self, subagent_id: String, cx: &mut Context<Self>) {
        if !self.expanded_subagents.remove(&subagent_id) {
            self.expanded_subagents.insert(subagent_id);
        }
        cx.notify();
    }

    pub fn view_subagent_in_panel(&mut self, _call_or_subagent_id: &str, cx: &mut Context<Self>) {
        self.right_sidebar_visible = true;
        self.inspector_active_tab = InspectorTab::Subagents;
        self.fetch_inspector_subagents(cx);
        self.persist_layout();
        cx.notify();
    }

    #[allow(dead_code)]
    pub fn session_subagents(
        &self,
        session_id: &str,
    ) -> Rc<Vec<console_core::types::SubagentInfo>> {
        self.session_subagents
            .get(session_id)
            .cloned()
            .unwrap_or_else(|| Rc::new(Vec::new()))
    }

    #[allow(dead_code)]
    pub fn select_inspector_file(&mut self, path: String, cx: &mut Context<Self>) {
        self.inspector_selected_path = Some(path);
        cx.notify();
    }

    /// Refresh the inspector (fs tree, git changes, session changes,
    /// subagents) when the right sidebar is visible. A single call site for
    /// the `right_sidebar_visible` guard that was previously copy-pasted
    /// across every tab-select/close path in `view/mod.rs`.
    pub fn maybe_refresh_inspector(&mut self, cx: &mut Context<Self>) {
        if self.right_sidebar_visible {
            self.refresh_inspector(cx);
        }
    }

    pub fn refresh_inspector(&mut self, cx: &mut Context<Self>) {
        let (session_id, cwd) = self.active_inspector_target();

        if cwd.is_none() && session_id.is_none() {
            self.inspector_tree = Rc::new(Vec::new());
            self.inspector_working_changes = Rc::new(Vec::new());
            self.inspector_session_changes = Rc::new(Vec::new());
            self.inspector_selected_path = None;
            cx.notify();
            return;
        }

        self.fetch_inspector_fs_tree(cx);
        self.fetch_inspector_git_changes(cx);
        self.fetch_inspector_session_changes(cx);
        self.fetch_inspector_subagents(cx);
        self.ensure_inspector_fs_watcher(cx);
    }

    pub fn ensure_inspector_fs_watcher(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_inspector_target();
        let Some(cwd) = cwd else {
            return;
        };

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            use futures_util::StreamExt;
            if let Ok(mut stream) = client.fs.watch_events(&cwd).await {
                while let Some(Ok(_event)) = stream.next().await {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if this.right_sidebar_visible {
                                    this.fetch_inspector_fs_tree(cx);
                                    this.fetch_inspector_git_changes(cx);
                                }
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }

    pub fn fetch_inspector_fs_tree(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_inspector_target();
        let Some(cwd) = cwd else {
            self.inspector_tree = Rc::new(Vec::new());
            cx.notify();
            return;
        };

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            match client.fs.get_entries(&cwd, Some(25), Some(false)).await {
                Ok(entries) => {
                    let tree = console_ui::build_tree_from_entries(&entries);
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.inspector_tree = Rc::new(tree);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch inspector fs tree: {}", err);
                }
            }
        })
        .detach();
    }

    pub fn fetch_inspector_git_changes(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_inspector_target();
        let Some(cwd) = cwd else {
            self.inspector_working_changes = Rc::new(Vec::new());
            cx.notify();
            return;
        };

        let client = self.client.clone();
        cx.spawn(
            async move |entity, cx| match client.git.get_status(Some(&cwd)).await {
                Ok(status) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.inspector_working_changes = Rc::new(status.files);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch inspector git changes: {}", err);
                }
            },
        )
        .detach();
    }

    pub fn fetch_inspector_session_changes(&mut self, cx: &mut Context<Self>) {
        let (session_id, _) = self.active_inspector_target();
        let Some(session_id) = session_id else {
            self.inspector_session_changes = Rc::new(Vec::new());
            cx.notify();
            return;
        };

        let client = self.client.clone();
        cx.spawn(
            async move |entity, cx| match client.sessions.get_changes(&session_id).await {
                Ok(changes) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.inspector_session_changes = Rc::new(changes);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch inspector session changes: {}", err);
                }
            },
        )
        .detach();
    }

    pub fn fetch_inspector_subagents(&mut self, cx: &mut Context<Self>) {
        let (session_id, _) = self.active_inspector_target();
        let Some(session_id) = session_id else {
            return;
        };

        let client = self.client.clone();
        cx.spawn(
            async move |entity, cx| match client.sessions.get_subagents(&session_id).await {
                Ok(subagents) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                for sub in &subagents {
                                    if let Some(ref sum) = sub.summary {
                                        let view = this
                                            .subagent_markdown_views
                                            .borrow_mut()
                                            .entry(sub.subagent_id.clone())
                                            .or_insert_with(|| {
                                                Rc::new(RefCell::new(
                                                    console_ui::markdown::render::MarkdownView::new(),
                                                ))
                                            })
                                            .clone();
                                        view.borrow_mut().set_text(sum, false);
                                    }
                                }
                                this.session_subagents
                                    .insert(session_id, Rc::new(subagents));
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to fetch inspector session subagents: {}", err);
                }
            },
        )
        .detach();
    }
}
