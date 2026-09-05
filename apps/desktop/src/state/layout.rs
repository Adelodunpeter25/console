//! Sidebar and window layout: visibility, width resizing, collapsed date
//! groups, and window-bounds persistence.

use std::rc::Rc;
use std::time::Duration;

use console_ui::utils::SessionDateGroup;
use gpui::{Context, Window};

use super::{ConsoleDesktopApp, SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH};
use crate::persistence;

/// Minimum spacing between window-bounds writes. `maybe_persist_window_state`
/// runs every render but returns early unless the poll interval has elapsed;
/// the in-memory snapshot then short-circuits unchanged frames and this
/// debounce coalesces a continuous drag into one write per interval.
const WINDOW_SAVE_DEBOUNCE: Duration = Duration::from_millis(500);
/// Minimum spacing between render-loop window-bounds polls. The render path
/// has no OS move/resize callback in this gpui version, so each frame would
/// otherwise call `window.window_bounds()`; this keeps the poll to ~4Hz while
/// a drag still coalesces through [`WINDOW_SAVE_DEBOUNCE`].
const WINDOW_POLL_INTERVAL: Duration = Duration::from_millis(250);

fn strip_diff_tabs(mut root: console_core::WorkspaceNode) -> console_core::WorkspaceNode {
    for leaf in root.leaves_mut() {
        leaf.tabs
            .retain(|tab| !matches!(tab, console_core::WorkspaceTabConfig::Diff { .. }));
        if let Some(active) = &leaf.active_tab_id {
            if active.starts_with("diff:") {
                leaf.active_tab_id = leaf.tabs.last().map(|t| t.id());
            }
        }
    }
    root
}

impl ConsoleDesktopApp {
    pub(crate) fn persist_layout(&self) {
        persistence::layout::save(persistence::PersistedLayoutState {
            sidebar_visible: self.sidebar_visible,
            sidebar_width: self.sidebar_width,
            right_sidebar_visible: self.right_sidebar_visible,
            right_sidebar_width: self.right_sidebar_width,
            collapsed_groups: self
                .collapsed_groups
                .iter()
                .map(|group| group.index())
                .collect(),
        });
        let cur_wid = self
            .selected_project_id
            .clone()
            .unwrap_or_else(|| "__default__".to_string());
        let mut doc = persistence::load_workspace_state();
        doc.active_workspace_id = Some(cur_wid.clone());
        doc.sidebar_visible = self.sidebar_visible;
        doc.sidebar_width = self.sidebar_width;
        doc.right_sidebar_visible = self.right_sidebar_visible;
        doc.right_sidebar_width = self.right_sidebar_width;

        let bounds = self.saved_window_state.unwrap_or(persistence::window::PersistedWindowState {
            x: 100.0,
            y: 100.0,
            width: persistence::window::DEFAULT_WIDTH,
            height: persistence::window::DEFAULT_HEIGHT,
            maximized: false,
        });

        let descriptor = persistence::PersistedWindowDescriptor {
            id: self.window_id.clone(),
            bounds,
            active_workspace_id: Some(cur_wid),
            sidebar_visible: self.sidebar_visible,
            sidebar_width: self.sidebar_width,
            right_sidebar_visible: self.right_sidebar_visible,
            right_sidebar_width: self.right_sidebar_width,
        };

        if let Some(existing) = doc.windows.iter_mut().find(|w| w.id == self.window_id) {
            *existing = descriptor;
        } else {
            doc.windows.push(descriptor);
        }
        persistence::save_workspace_state(&doc);
    }

    pub fn persist_workspaces(&self) {
        let mut workspaces_map = std::collections::HashMap::new();

        for (proj_id_opt, root) in &self.project_workspace_roots {
            let wid = proj_id_opt
                .clone()
                .unwrap_or_else(|| "__default__".to_string());
            let clean_root = strip_diff_tabs(root.clone());
            let active_tab_id = clean_root.leaves().first().and_then(|l| l.active_tab_id.clone());
            let proj_info = proj_id_opt
                .as_ref()
                .and_then(|pid| self.projects.iter().find(|p| &p.id == pid));
            workspaces_map.insert(
                wid.clone(),
                persistence::PersistedWorkspace {
                    id: wid,
                    project_id: proj_id_opt.clone(),
                    cwd: proj_info.map(|p| p.path.clone()),
                    name: proj_info.map(|p| p.name.clone()),
                    root: clean_root,
                    active_tab_id,
                },
            );
        }

        let cur_wid = self
            .selected_project_id
            .clone()
            .unwrap_or_else(|| "__default__".to_string());
        let clean_cur_root = strip_diff_tabs(self.workspace_root.clone());
        let cur_active_tab = self
            .active_pane_id
            .as_deref()
            .and_then(|pid| clean_cur_root.leaves().into_iter().find(|l| l.id == pid))
            .and_then(|l| l.active_tab_id.clone())
            .or_else(|| {
                clean_cur_root
                    .leaves()
                    .first()
                    .and_then(|l| l.active_tab_id.clone())
            });
        let cur_proj = self
            .selected_project_id
            .as_ref()
            .and_then(|pid| self.projects.iter().find(|p| &p.id == pid));
        workspaces_map.insert(
            cur_wid.clone(),
            persistence::PersistedWorkspace {
                id: cur_wid.clone(),
                project_id: self.selected_project_id.clone(),
                cwd: cur_proj.map(|p| p.path.clone()),
                name: cur_proj.map(|p| p.name.clone()),
                root: clean_cur_root,
                active_tab_id: cur_active_tab,
            },
        );

        let doc = persistence::WorkspacesDocument {
            version: 1,
            active_workspace_id: Some(cur_wid),
            workspaces: workspaces_map.into_values().collect(),
        };
        persistence::save_workspaces(&doc);
    }

    /// Collapse or expand a sidebar date group.
    pub fn toggle_sidebar_group(&mut self, group: SessionDateGroup, cx: &mut Context<Self>) {
        let collapsed = Rc::make_mut(&mut self.collapsed_groups);
        if !collapsed.remove(&group) {
            collapsed.insert(group);
        }
        self.persist_layout();
        cx.notify();
    }

    pub fn toggle_left_sidebar(&mut self, cx: &mut Context<Self>) {
        self.sidebar_visible = !self.sidebar_visible;
        self.persist_layout();
        cx.notify();
    }

    pub fn begin_sidebar_resize(&mut self, start_x: f32) {
        self.sidebar_resize_start = Some((start_x, self.sidebar_width));
    }

    pub fn resize_sidebar(&mut self, current_x: f32) -> bool {
        let Some((start_x, start_width)) = self.sidebar_resize_start else {
            return false;
        };
        let width = (start_width + current_x - start_x).clamp(SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH);
        if (self.sidebar_width - width).abs() < 0.5 {
            return false;
        }
        self.sidebar_width = width;
        true
    }

    pub fn finish_sidebar_resize(&mut self) -> bool {
        if self.sidebar_resize_start.take().is_some() {
            self.persist_layout();
            true
        } else {
            false
        }
    }

    pub fn begin_split_resize(
        &mut self,
        split_id: String,
        direction: console_core::SplitDirection,
        start_pos: gpui::Point<gpui::Pixels>,
        window: &Window,
    ) {
        let sizes = console_ui::workspace::find_split_sizes(&self.workspace_root, &split_id)
            .unwrap_or([50.0, 50.0]);
        let viewport_size = window.viewport_size();
        self.split_resize = Some((split_id, direction, start_pos, sizes, viewport_size));
    }

    pub fn resize_split_drag(&mut self, current_pos: gpui::Point<gpui::Pixels>) -> bool {
        let Some((split_id, direction, start_pos, start_sizes, viewport_size)) =
            self.split_resize.clone()
        else {
            return false;
        };

        let delta_percent = match direction {
            console_core::SplitDirection::Horizontal => {
                let avail_width = (f32::from(viewport_size.width) - self.sidebar_width).max(100.0);
                let delta_px = f32::from(current_pos.x - start_pos.x);
                (delta_px / avail_width) * 100.0
            }
            console_core::SplitDirection::Vertical => {
                let avail_height = f32::from(viewport_size.height).max(100.0);
                let delta_px = f32::from(current_pos.y - start_pos.y);
                (delta_px / avail_height) * 100.0
            }
        };

        let new_size_0 = (start_sizes[0] + delta_percent).clamp(10.0, 90.0);
        console_ui::workspace::resize_split(&mut self.workspace_root, &split_id, new_size_0)
    }

    pub fn finish_split_resize(&mut self) -> bool {
        self.split_resize.take().is_some()
    }

    /// Track the window frame for persistence. Called every render via
    /// [`Self::maybe_persist_window_state`]: an unchanged frame costs one
    /// `Copy` struct comparison and no I/O; a changed frame is written at most
    /// once per [`WINDOW_SAVE_DEBOUNCE`], with the trailing timer always
    /// flushing the newest captured bounds.
    pub fn persist_window_state(&mut self, window: &Window, cx: &mut Context<Self>) {
        let Some(state) = persistence::window::capture(window) else {
            return;
        };
        crate::window::update_active_window_bounds(state.bounds());
        if self.saved_window_state == Some(state) {
            return;
        }
        if self.pending_window_state == Some(state) {
            return;
        }
        let first_pending = self.pending_window_state.is_none();
        self.pending_window_state = Some(state);
        if !first_pending {
            // A flush is already scheduled; it will pick up this newer frame.
            return;
        }
        cx.spawn(async move |entity, cx| {
            cx.background_executor().timer(WINDOW_SAVE_DEBOUNCE).await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _| this.flush_pending_window_state());
                }
            });
        })
        .detach();
    }

    /// Render-loop entry point for window persistence. Throttles the
    /// `window.window_bounds()` poll to [`WINDOW_POLL_INTERVAL`] so idle
    /// frames skip the OS call entirely; bounds checks themselves stay in
    /// [`Self::persist_window_state`].
    pub fn maybe_persist_window_state(&mut self, window: &Window, cx: &mut Context<Self>) {
        let now = std::time::Instant::now();
        let too_soon = self
            .last_window_poll
            .is_some_and(|last| now.duration_since(last) < WINDOW_POLL_INTERVAL);
        if too_soon {
            return;
        }
        self.last_window_poll = Some(now);
        self.persist_window_state(window, cx);
    }

    fn flush_pending_window_state(&mut self) {
        if let Some(state) = self.pending_window_state.take() {
            persistence::window::save(state);
            self.saved_window_state = Some(state);
            let cur_wid = self
                .selected_project_id
                .clone()
                .unwrap_or_else(|| "__default__".to_string());
            let mut doc = persistence::load_workspace_state();
            doc.active_workspace_id = Some(cur_wid.clone());
            doc.sidebar_visible = self.sidebar_visible;
            doc.sidebar_width = self.sidebar_width;
            doc.right_sidebar_visible = self.right_sidebar_visible;
            doc.right_sidebar_width = self.right_sidebar_width;

            let descriptor = persistence::PersistedWindowDescriptor {
                id: self.window_id.clone(),
                bounds: state,
                active_workspace_id: Some(cur_wid),
                sidebar_visible: self.sidebar_visible,
                sidebar_width: self.sidebar_width,
                right_sidebar_visible: self.right_sidebar_visible,
                right_sidebar_width: self.right_sidebar_width,
            };

            if let Some(existing) = doc.windows.iter_mut().find(|w| w.id == self.window_id) {
                *existing = descriptor;
            } else {
                doc.windows.push(descriptor);
            }
            persistence::save_workspace_state(&doc);
        }
    }
}
