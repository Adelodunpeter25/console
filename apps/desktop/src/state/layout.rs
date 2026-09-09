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
/// Minimum spacing between workspace.json writes. Bursts (rapid tab
/// switching) coalesce: the first write goes out immediately, the rest set a
/// dirty flag flushed from the render loop (see `flush_workspaces_if_dirty`).
const WORKSPACES_SAVE_DEBOUNCE: Duration = Duration::from_millis(500);
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

fn clean_root_for_workspace(
    root: console_core::WorkspaceNode,
    project_id: &Option<String>,
) -> console_core::WorkspaceNode {
    let mut clean = strip_diff_tabs(root);
    // A workspace must never persist tabs pointing at another folder.
    console_ui::workspace::ops::retain_project_tabs(&mut clean, project_id);
    clean
}

impl ConsoleDesktopApp {
    pub(crate) fn persist_layout(&self) {
        if !self.is_main_window {
            // Secondary "New Window" windows never persist layout; only the
            // main window's state survives restarts.
            return;
        }
        persistence::layout::save(persistence::PersistedLayoutState {
            sidebar_visible: self.sidebar_visible,
            sidebar_width: self.sidebar_width,
            right_sidebar_visible: self.right_sidebar_visible,
            right_sidebar_width: self.right_sidebar_width,
            right_sidebar_bottom_height: self.right_sidebar_bottom_height,
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
        // Build the document from memory: every field comes from live state,
        // so no disk re-read is needed to preserve anything.
        persistence::save_workspace_state(&persistence::WorkspaceStateDocument {
            version: 1,
            active_workspace_id: Some(cur_wid),
            sidebar_visible: self.sidebar_visible,
            sidebar_width: self.sidebar_width,
            right_sidebar_visible: self.right_sidebar_visible,
            right_sidebar_width: self.right_sidebar_width,
            right_sidebar_bottom_height: self.right_sidebar_bottom_height,
            right_sidebar_bottom_collapsed: self.right_sidebar_bottom_collapsed,
        });
    }

    pub fn persist_workspaces(&self) {
        // Only the main window's workspaces survive restarts (same rule as
        // `persist_layout`); secondaries skipping the write also avoids them
        // clobbering the main window's saved trees with their transient ones.
        if !self.is_main_window {
            return;
        }
        let Some(bytes) = self.serialized_workspaces() else {
            return;
        };
        // Skip redundant writes: tab actions often persist without changes.
        if self
            .persisted_workspaces_bytes
            .borrow()
            .as_deref()
            .is_some_and(|last| last == bytes.as_slice())
        {
            return;
        }
        // Coalesce bursts (rapid tab switching): at most one write per
        // interval, with a trailing flush from the render loop.
        let now = std::time::Instant::now();
        let too_soon = self.last_workspaces_persist.get().is_some_and(|last| {
            now.duration_since(last) < WORKSPACES_SAVE_DEBOUNCE
        });
        if too_soon {
            self.workspaces_dirty.set(true);
            return;
        }
        self.write_workspaces_bytes(bytes);
    }

    /// Render-loop trailing flush for throttled workspace saves. Called from
    /// `maybe_persist_window_state`, so a dirty tree is written at most one
    /// frame-batch after the burst ends.
    pub(crate) fn flush_workspaces_if_dirty(&self) {
        if !self.is_main_window || !self.workspaces_dirty.get() {
            return;
        }
        self.workspaces_dirty.set(false);
        let Some(bytes) = self.serialized_workspaces() else {
            return;
        };
        if self
            .persisted_workspaces_bytes
            .borrow()
            .as_deref()
            .is_some_and(|last| last == bytes.as_slice())
        {
            return;
        }
        self.write_workspaces_bytes(bytes);
    }

    /// Serialize the persistable workspace document (compact JSON bytes), or
    /// `None` when serialization fails (nothing is written then).
    fn serialized_workspaces(&self) -> Option<Vec<u8>> {
        let mut workspaces_map = std::collections::HashMap::new();

        for (proj_id_opt, root) in &self.project_workspace_roots {
            let wid = proj_id_opt
                .clone()
                .unwrap_or_else(|| "__default__".to_string());
            let clean_root = clean_root_for_workspace(root.clone(), proj_id_opt);
            // Remembered split focus when still present, else the first leaf.
            // The top-level active tab follows that pane (not always the first).
            let pane_id = self
                .project_active_panes
                .get(proj_id_opt)
                .cloned()
                .filter(|id| clean_root.leaves().iter().any(|leaf| &leaf.id == id))
                .or_else(|| clean_root.first_leaf().map(|leaf| leaf.id.clone()));
            let active_tab_id = pane_id.as_deref().and_then(|pid| {
                clean_root
                    .leaves()
                    .into_iter()
                    .find(|leaf| leaf.id == pid)
                    .and_then(|leaf| leaf.active_tab_id.clone())
            });
            let proj_info = proj_id_opt
                .as_ref()
                .and_then(|pid| self.projects.iter().find(|p| &p.id == pid));
            let cwd = proj_info.map(|p| p.path.clone());
            let (term_count, term_active_idx) = cwd
                .as_ref()
                .and_then(|c| self.right_sidebar_terminals_by_cwd.get(c))
                .map(|t| (Some(t.terminals.len()), Some(t.active_idx)))
                .unwrap_or((None, None));
            workspaces_map.insert(
                wid.clone(),
                persistence::PersistedWorkspace {
                    id: wid,
                    project_id: proj_id_opt.clone(),
                    cwd,
                    name: proj_info.map(|p| p.name.clone()),
                    root: clean_root,
                    active_tab_id,
                    active_pane_id: pane_id,
                    bottom_terminal_tab_count: term_count,
                    bottom_terminal_active_idx: term_active_idx,
                },
            );
        }

        let cur_wid = self
            .selected_project_id
            .clone()
            .unwrap_or_else(|| "__default__".to_string());
        let clean_cur_root =
            clean_root_for_workspace(self.workspace_root.clone(), &self.selected_project_id);
        let cur_pane_id = self
            .active_pane_id
            .clone()
            .filter(|pid| clean_cur_root.leaves().iter().any(|l| &l.id == pid))
            .or_else(|| {
                clean_cur_root
                    .first_leaf()
                    .map(|leaf| leaf.id.clone())
            });
        let cur_active_tab = cur_pane_id.as_deref().and_then(|pid| {
            clean_cur_root
                .leaves()
                .into_iter()
                .find(|l| l.id == pid)
                .and_then(|l| l.active_tab_id.clone())
        });
        let cur_proj = self
            .selected_project_id
            .as_ref()
            .and_then(|pid| self.projects.iter().find(|p| &p.id == pid));
        let cur_cwd = cur_proj.map(|p| p.path.clone()).or_else(|| {
            let (_, cwd) = self.active_inspector_target();
            cwd
        });
        let (cur_term_count, cur_term_active_idx) = cur_cwd
            .as_ref()
            .and_then(|c| self.right_sidebar_terminals_by_cwd.get(c))
            .map(|t| (Some(t.terminals.len()), Some(t.active_idx)))
            .unwrap_or((None, None));
        workspaces_map.insert(
            cur_wid.clone(),
            persistence::PersistedWorkspace {
                id: cur_wid.clone(),
                project_id: self.selected_project_id.clone(),
                cwd: cur_cwd,
                name: cur_proj.map(|p| p.name.clone()),
                root: clean_cur_root,
                active_tab_id: cur_active_tab,
                active_pane_id: cur_pane_id,
                bottom_terminal_tab_count: cur_term_count,
                bottom_terminal_active_idx: cur_term_active_idx,
            },
        );

        let doc = persistence::WorkspacesDocument {
            version: 1,
            active_workspace_id: Some(cur_wid),
            workspaces: workspaces_map.into_values().collect(),
        };
        serde_json::to_vec(&doc).ok()
    }

    /// Record + write already-serialized workspace bytes.
    fn write_workspaces_bytes(&self, bytes: Vec<u8>) {
        *self.persisted_workspaces_bytes.borrow_mut() = Some(bytes.clone());
        self.last_workspaces_persist
            .set(Some(std::time::Instant::now()));
        persistence::save_workspaces_bytes(&bytes);
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
        if !self.is_main_window {
            // Secondary windows track bounds for cascade offsets only; they
            // never overwrite the persisted main-window frame.
            return;
        }
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
        // Trailing flush for throttled workspace saves; runs at the render
        // loop's poll cadence so bursts settle without extra timers.
        self.flush_workspaces_if_dirty();
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
            // Same no-re-read rule as `persist_layout`: every field comes
            // from live state, including the bottom-split fields.
            persistence::save_workspace_state(&persistence::WorkspaceStateDocument {
                version: 1,
                active_workspace_id: Some(cur_wid),
                sidebar_visible: self.sidebar_visible,
                sidebar_width: self.sidebar_width,
                right_sidebar_visible: self.right_sidebar_visible,
                right_sidebar_width: self.right_sidebar_width,
                right_sidebar_bottom_height: self.right_sidebar_bottom_height,
                right_sidebar_bottom_collapsed: self.right_sidebar_bottom_collapsed,
            });
        }
    }
}
