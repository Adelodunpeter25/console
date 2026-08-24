//! Sidebar and window layout: visibility, width resizing, collapsed date
//! groups, and window-bounds persistence.

use std::rc::Rc;
use std::time::Duration;

use console_ui::utils::SessionDateGroup;
use gpui::{Context, Window};

use super::{ConsoleDesktopApp, SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH};
use crate::persistence;

/// Minimum spacing between window-bounds writes. `persist_window_state` runs
/// every render; the in-memory snapshot short-circuits unchanged frames and
/// this debounce coalesces a continuous drag into one write per interval.
const WINDOW_SAVE_DEBOUNCE: Duration = Duration::from_millis(500);

impl ConsoleDesktopApp {
    fn persist_layout(&self) {
        persistence::layout::save(persistence::PersistedLayoutState {
            sidebar_visible: self.sidebar_visible,
            sidebar_width: self.sidebar_width,
            collapsed_groups: self
                .collapsed_groups
                .iter()
                .map(|group| group.index())
                .collect(),
        });
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

    pub fn set_sidebar_visible(&mut self, visible: bool) {
        if self.sidebar_visible == visible {
            return;
        }
        self.sidebar_visible = visible;
        self.persist_layout();
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

    /// Track the window frame for persistence. Called every render: an
    /// unchanged frame costs one `Copy` struct comparison and no I/O; a
    /// changed frame is written at most once per [`WINDOW_SAVE_DEBOUNCE`],
    /// with the trailing timer always flushing the newest captured bounds.
    pub fn persist_window_state(&mut self, window: &Window, cx: &mut Context<Self>) {
        let Some(state) = persistence::window::capture(window) else {
            return;
        };
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
            cx.background_executor()
                .timer(WINDOW_SAVE_DEBOUNCE)
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _| this.flush_pending_window_state());
                }
            });
        })
        .detach();
    }

    fn flush_pending_window_state(&mut self) {
        if let Some(state) = self.pending_window_state.take() {
            persistence::window::save(state);
            self.saved_window_state = Some(state);
        }
    }
}
