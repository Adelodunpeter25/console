//! Files tab: opening it and toggling its folders.

use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use std::rc::Rc;

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
        let tree = self.file_tree_nodes.clone();
        self.file_tree_data.entry(tab_id).or_insert(tree);
        self.persist_workspaces();

        self.refresh_file_tree(cx);
        cx.notify();
    }

    pub fn toggle_file_tree_folder(&mut self, tab_id: &str, path: String, cx: &mut Context<Self>) {
        let expanded = self.file_tree_expanded.entry(tab_id.to_string()).or_default();
        if !expanded.remove(&path) {
            expanded.insert(path);
        }
        cx.notify();
    }

    /// True while a Files tab is the active tab of any pane.
    pub(crate) fn file_tree_tab_active(&self) -> bool {
        self.workspace_root.leaves().iter().any(|leaf| {
            matches!(
                leaf.active_tab_id
                    .as_deref()
                    .and_then(|id| leaf.tabs.iter().find(|t| t.id() == id)),
                Some(WorkspaceTabConfig::FileTree { .. })
            )
        })
    }

    /// Refetch the tree and git status and reconcile the watch streams with
    /// what is on screen. Cheap to call: it does nothing unless a Files tab
    /// is active, and drops the streams (closing their SSE connections) then.
    pub fn refresh_file_tree(&mut self, cx: &mut Context<Self>) {
        if !self.file_tree_tab_active() {
            self.file_tree_fs_watch = None;
            self.file_tree_git_watch = None;
            return;
        }
        let (_, cwd) = self.active_pane_target();
        if cwd.is_none() {
            self.file_tree_fs_watch = None;
            self.file_tree_git_watch = None;
            self.file_tree_nodes = Rc::new(Vec::new());
            self.set_working_changes(Vec::new());
            cx.notify();
            return;
        }
        self.fetch_file_tree(cx);
        self.fetch_git_changes(cx);
        self.ensure_file_tree_fs_watcher(cx);
        self.ensure_file_tree_git_watcher(cx);
    }

    pub fn ensure_file_tree_fs_watcher(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_pane_target();
        let Some(cwd) = cwd else {
            return;
        };

        // One stream per target: reuse the live watcher unless the inspector
        // target changed. Dropping the previous task cancels its SSE loop, so
        // repeated refreshes never accumulate duplicate streams.
        if let Some((watched_cwd, _)) = &self.file_tree_fs_watch
            && watched_cwd == &cwd
        {
            return;
        }
        self.file_tree_fs_watch = None;

        let client = self.client.clone();
        let transport = self.transport;
        let watch_cwd = cwd.clone();
        let task = cx.spawn(async move |entity, cx| {
            use futures_util::StreamExt;
            if let Ok(mut stream) = transport.watch_fs(&client, &watch_cwd).await {
                while let Some(Ok(_event)) = stream.next().await {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                // Git working changes arrive on their own
                                // watch stream now; fs events only refresh the tree.
                                if this.file_tree_tab_active() {
                                    this.schedule_file_tree_fetch(cx);
                                }
                            });
                        }
                    });
                }
            }
        });
        self.file_tree_fs_watch = Some((cwd, task));
    }

    /// Live git working changes via GET /api/git/status/watch SSE.
    /// One-shot fetch_git_changes stays as the initial snapshot
    /// (and fallback for servers without the watch route); the stream
    /// pushes every recomputed summary after that.
    pub fn ensure_file_tree_git_watcher(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_pane_target();
        let Some(cwd) = cwd else {
            return;
        };

        // One stream per target: reuse the live watcher unless the inspector
        // target changed (dropping the previous task cancels its SSE loop).
        if let Some((watched_cwd, _)) = &self.file_tree_git_watch
            && watched_cwd == &cwd
        {
            return;
        }
        self.file_tree_git_watch = None;

        let client = self.client.clone();
        let transport = self.transport;
        let watch_cwd = cwd.clone();
        let task = cx.spawn(async move |entity, cx| {
            use futures_util::StreamExt;
            let mut stream = match transport.watch_git_status(&client, &watch_cwd).await {
                Ok(stream) => stream,
                Err(err) => {
                    log::warn!("Failed to connect to git status watch: {}", err);
                    return;
                }
            };
            while let Some(Ok(status)) = stream.next().await {
                let watch_cwd = watch_cwd.clone();
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if this.file_tree_tab_active() {
                                this.set_working_changes(status.files);
                            }
                            // Keep branches in sync across all panes whose cwd matches this repository.
                            let matching_panes: Vec<String> = this
                                .workspace_pane_states
                                .iter()
                                .filter(|(_, state)| {
                                    state.branch_cwd.as_deref() == Some(watch_cwd.as_str())
                                        && state.branch_loaded
                                        && state
                                            .branches
                                            .iter()
                                            .find(|b| b.current)
                                            .map(|b| b.name.as_str())
                                            != Some(status.branch.as_str())
                                })
                                .map(|(pane_id, _)| pane_id.clone())
                                .collect();
                            for pane_id in matching_panes {
                                this.reload_branches_for_pane(pane_id, watch_cwd.clone(), cx);
                            }
                            cx.notify();
                        });
                    }
                });
            }
        });
        self.file_tree_git_watch = Some((cwd, task));
    }

    /// Trailing debounce for inspector tree refreshes: fs events arrive in
    /// bursts (builds, checkouts, git operations), and each one previously
    /// triggered a full depth-25 directory listing. Coalesce each burst into
    /// a single fetch ~300ms after the last event.
    pub fn schedule_file_tree_fetch(&mut self, cx: &mut Context<Self>) {
        if self.fs_tree_fetch_pending || !self.file_tree_tab_active() {
            return;
        }
        self.fs_tree_fetch_pending = true;
        cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(300))
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.fs_tree_fetch_pending = false;
                        if this.file_tree_tab_active() {
                            this.fetch_file_tree(cx);
                        }
                    });
                }
            });
        })
        .detach();
    }

    pub fn fetch_file_tree(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_pane_target();
        let Some(cwd) = cwd else {
            self.file_tree_nodes = Rc::new(Vec::new());
            cx.notify();
            return;
        };

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            // Keep the reqwest future on GPUI's context so it can use the
            // Tokio runtime entered by `main`. GPUI's background executor is
            // not a Tokio runtime, and polling reqwest there makes its
            // connector call `tokio::spawn` without a runtime handle.
            let result = match client.fs.get_entries(&cwd, Some(25), Some(false)).await {
                Ok(entries) => tokio::task::spawn_blocking(move || {
                    console_ui::build_tree_from_entries(&entries)
                })
                .await
                .map_err(anyhow::Error::from),
                Err(error) => Err(error),
            };
            match result {
                Ok(tree) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let tree = Rc::new(tree);
                                for data in this.file_tree_data.values_mut() {
                                    *data = tree.clone();
                                }
                                this.file_tree_nodes = tree;
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

    /// The one place working changes are written: the Files tree's badges are
    /// derived here, once per update, instead of on every repaint.
    fn set_working_changes(&mut self, files: Vec<console_core::types::GitFileEntry>) {
        self.git_badges = Rc::new(console_ui::aggregate_git_badges(&files));
        self.working_changes = Rc::new(files);
    }

    pub fn fetch_git_changes(&mut self, cx: &mut Context<Self>) {
        let (_, cwd) = self.active_pane_target();
        let Some(cwd) = cwd else {
            self.set_working_changes(Vec::new());
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
                                this.set_working_changes(status.files);
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
}
