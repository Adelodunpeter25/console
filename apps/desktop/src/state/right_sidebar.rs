use std::rc::Rc;

use console_ui::InspectorTab;
use gpui::Context;

use super::{ConsoleDesktopApp, RIGHT_SIDEBAR_MAX_WIDTH, RIGHT_SIDEBAR_MIN_WIDTH};

impl ConsoleDesktopApp {
    pub fn toggle_right_sidebar(&mut self, cx: &mut Context<Self>) {
        self.right_sidebar_visible = !self.right_sidebar_visible;
        self.persist_layout();
        if self.right_sidebar_visible {
            self.refresh_inspector(cx);
        }
        cx.notify();
    }

    #[allow(dead_code)]
    pub fn set_right_sidebar_visible(&mut self, visible: bool, cx: &mut Context<Self>) {
        if self.right_sidebar_visible == visible {
            return;
        }
        self.right_sidebar_visible = visible;
        self.persist_layout();
        if visible {
            self.refresh_inspector(cx);
        }
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
        let width = (start_width + (start_x - current_x)).clamp(
            RIGHT_SIDEBAR_MIN_WIDTH,
            RIGHT_SIDEBAR_MAX_WIDTH,
        );
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

    pub fn set_inspector_tab(&mut self, tab: InspectorTab, cx: &mut Context<Self>) {
        if self.inspector_active_tab == tab {
            return;
        }
        self.inspector_active_tab = tab;
        cx.notify();
    }

    #[allow(dead_code)]
    pub fn set_inspector_search_query(&mut self, query: String, cx: &mut Context<Self>) {
        self.inspector_search_query = query;
        cx.notify();
    }

    pub fn toggle_inspector_folder(&mut self, path: String, cx: &mut Context<Self>) {
        let expanded = Rc::make_mut(&mut self.inspector_expanded_folders);
        if !expanded.remove(&path) {
            expanded.insert(path);
        }
        cx.notify();
    }

    pub fn select_inspector_file(&mut self, path: String, cx: &mut Context<Self>) {
        self.inspector_selected_path = Some(path);
        cx.notify();
    }

    pub fn refresh_inspector(&mut self, cx: &mut Context<Self>) {
        self.fetch_inspector_fs_tree(cx);
        self.fetch_inspector_git_changes(cx);
        self.fetch_inspector_session_changes(cx);
        self.ensure_inspector_fs_watcher(cx);
    }

    pub fn ensure_inspector_fs_watcher(&mut self, cx: &mut Context<Self>) {
        let cwd = self
            .selected_session_id
            .as_deref()
            .and_then(|id| self.sessions.iter().find(|s| s.id == id))
            .map(|s| s.cwd.clone())
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default().to_string_lossy().to_string());

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
        let cwd = self
            .selected_session_id
            .as_deref()
            .and_then(|id| self.sessions.iter().find(|s| s.id == id))
            .map(|s| s.cwd.clone())
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default().to_string_lossy().to_string());

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            match client.fs.get_entries(&cwd, Some(4), Some(false)).await {
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
        let cwd = self
            .selected_session_id
            .as_deref()
            .and_then(|id| self.sessions.iter().find(|s| s.id == id))
            .map(|s| s.cwd.clone())
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default().to_string_lossy().to_string());

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            match client.git.get_status(Some(&cwd)).await {
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
            }
        })
        .detach();
    }

    pub fn fetch_inspector_session_changes(&mut self, cx: &mut Context<Self>) {
        let Some(session_id) = self.selected_session_id.clone() else {
            self.inspector_session_changes = Rc::new(Vec::new());
            return;
        };

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            match client.sessions.get_changes(&session_id).await {
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
            }
        })
        .detach();
    }
}
