use gpui::{App, Bounds, Window, WindowBounds, point, px, size};
use serde::{Deserialize, Serialize};

use super::store;

pub const DEFAULT_WIDTH: f32 = 1240.0;
pub const DEFAULT_HEIGHT: f32 = 820.0;
const MIN_WIDTH: f32 = 720.0;
const MIN_HEIGHT: f32 = 480.0;
const MAX_DIMENSION: f32 = 10_000.0;

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq)]
pub struct PersistedWindowState {
    pub x: f32,
    pub y: f32,
    pub width: f32,
    pub height: f32,
    #[serde(default)]
    pub maximized: bool,
}

impl PersistedWindowState {
    fn is_valid(self) -> bool {
        self.width.is_finite()
            && self.height.is_finite()
            && self.x.is_finite()
            && self.y.is_finite()
            && self.width >= MIN_WIDTH
            && self.height >= MIN_HEIGHT
            && self.width <= MAX_DIMENSION
            && self.height <= MAX_DIMENSION
    }

    fn bounds(self) -> Bounds<gpui::Pixels> {
        Bounds::new(
            point(px(self.x), px(self.y)),
            size(px(self.width), px(self.height)),
        )
    }

    fn from_bounds(bounds: Bounds<gpui::Pixels>, maximized: bool) -> Self {
        Self {
            x: f32::from(bounds.origin.x),
            y: f32::from(bounds.origin.y),
            width: f32::from(bounds.size.width),
            height: f32::from(bounds.size.height),
            maximized,
        }
    }
}

/// Restore the last floating frame before the native window is created.
///
/// Invalid or missing state intentionally falls back to the same centered
/// frame used by a fresh installation. The saved coordinates are retained so
/// windows on a secondary display can be restored there as well.
pub fn load_window_bounds(cx: &App) -> WindowBounds {
    let Some(state) = store::load_window().filter(|state| state.is_valid()) else {
        return WindowBounds::Windowed(Bounds::centered(
            None,
            size(px(DEFAULT_WIDTH), px(DEFAULT_HEIGHT)),
            cx,
        ));
    };

    if state.maximized {
        WindowBounds::Maximized(state.bounds())
    } else {
        WindowBounds::Windowed(state.bounds())
    }
}

/// Capture the current floating frame. Fullscreen does not overwrite the
/// user's normal window frame, so closing while fullscreen restores cleanly.
pub fn capture(window: &Window) -> Option<PersistedWindowState> {
    match window.window_bounds() {
        WindowBounds::Fullscreen(_) => None,
        WindowBounds::Maximized(bounds) => Some(PersistedWindowState::from_bounds(bounds, true)),
        WindowBounds::Windowed(bounds) => Some(PersistedWindowState::from_bounds(
            bounds,
            window.is_maximized(),
        )),
    }
}

pub fn save(state: PersistedWindowState) {
    if state.is_valid() {
        store::save_window(state);
    }
}
