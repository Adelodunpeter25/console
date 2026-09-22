use console_core::ConsoleClient;
use console_core::types::terminal::{TerminalSize, TerminalSpawnParams, TerminalStatus};
use gpui::{
    App, Bounds, Context, ElementInputHandler, EntityInputHandler, ExternalPaths, FocusHandle,
    Focusable, IntoElement, KeyDownEvent, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent,
    ParentElement, Pixels, Render, ScrollWheelEvent, SharedString, Styled, UTF16Selection, Window,
    div, prelude::*, px,
};
use std::cell::RefCell;
use std::collections::HashMap;
use std::ops::Range;
use std::rc::Rc;
use std::sync::Arc;
use termy_core::{
    TerminalMouseButton, TerminalMouseEventKind, TerminalMouseModifiers, TerminalMousePosition,
    encode_mouse_report,
};

use super::actions::{TerminalShiftTab, TerminalTab};
use super::theme::TerminalTheme;
use crate::theme::Theme;

mod input;
mod paint;
mod selection;

use self::input::{dropped_paths_input, paste_input_for_clipboard};
use self::paint::{CachedRowPaint, render_canvas_grid};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct TerminalCellPos {
    pub col: u16,
    pub row: u16,
}

/// feeds server output → grid → snapshot, and forwards keyboard → `input`.
///
/// ```ignore
/// let params = TerminalSpawnParams { cwd: project.path.clone(), ..Default::default() };
/// let view = cx.new(|cx| TerminalView::new(params, client.clone(), window, cx));
/// div().child(view)
/// ```
pub struct TerminalView {
    focus: FocusHandle,
    handle: Option<Arc<console_core::services::terminal::TerminalHandle>>,
    /// Shared snapshot: cloned as `Arc` (refcount bump) per render instead of
    /// a deep grid clone, so idle frames stay O(1) before paint.
    snapshot: Option<Arc<console_core::types::terminal::TerminalGridSnapshot>>,
    /// Damage paired with `snapshot` (same backend lock). Lets the renderer
    /// skip clean rows before shaping/hashing.
    snapshot_damage: termy_core::TerminalDamageSnapshot,
    /// Mouse mode paired with `snapshot` — cached here so mousemove/scroll
    /// never touch the backend lock per event.
    cached_mouse_mode: termy_core::TerminalMouseMode,
    status: TerminalStatus,
    error: Option<String>,
    size: TerminalSize,
    selection_anchor: Option<TerminalCellPos>,
    selection_head: Option<TerminalCellPos>,
    selection_dragging: bool,
    /// Non-zero while the local terminal scrollback viewport is away from the
    /// live prompt. The backend snapshot keeps the cursor at its live-grid row;
    /// rendering it during scrollback makes the cursor appear to follow history.
    scrollback_offset: i32,
    /// Paint cache: row → pre-shaped runs. Shared with the paint closure so
    /// frames repaint cached rows without shaping. Cleared on theme change
    /// (resolved colors are baked into cached runs).
    paint_cache: Rc<RefCell<HashMap<u16, CachedRowPaint>>>,
    cache_theme: Option<(gpui::Hsla, gpui::Hsla)>,
    /// Measured cell metrics from the last canvas paint. Mouse→cell mapping
    /// must use these (not constants) or clicks land on the wrong cells.
    cell_metrics: Option<(Pixels, Pixels)>,
    /// Window-space origin of the painted grid (canvas bounds + padding) from
    /// the last paint. Mouse events are window-relative; without subtracting
    /// this, every click lands offset by wherever the pane sits on screen.
    grid_origin: Option<gpui::Point<Pixels>>,
    /// Last mouse-down for multi-click detection (double = word, triple =
    /// line). Count cycles 1 → 2 → 3 on same-cell clicks within 500ms.
    last_click: Option<(std::time::Instant, TerminalCellPos, u8)>,
    /// Button currently held while the PTY has mouse reporting enabled —
    /// drives drag/motion reports. `None` when reporting is off.
    mouse_down: Option<TerminalMouseButton>,
    /// Scrollback state paired with `snapshot`: (display_offset, history).
    /// Offset 0 is the live prompt; the scrollbar renders from this so it
    /// never touches the backend lock per frame.
    cached_scroll: (usize, usize),
    /// Alt-screen paired with `snapshot` — hides the scrollbar and local
    /// scrollback while a TUI owns the viewport.
    cached_alt_screen: bool,
    /// Window-space bounds of the scrollbar track from the last paint, for
    /// mapping scrollbar clicks/drags to scroll offsets.
    scrollbar_bounds: Option<gpui::Bounds<Pixels>>,
    /// A scrollbar thumb drag is in progress.
    scrollbar_dragging: bool,
}

impl TerminalView {
    pub fn new(
        params: TerminalSpawnParams,
        client: ConsoleClient,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let focus = cx.focus_handle();
        window.focus(&focus, cx);

        let size = TerminalSize::new(80, 24);
        let mut this = Self {
            focus,
            handle: None,
            snapshot: None,
            snapshot_damage: termy_core::TerminalDamageSnapshot::Full,
            cached_mouse_mode: termy_core::TerminalMouseMode::default(),
            status: TerminalStatus::Spawning,
            error: None,
            size,
            selection_anchor: None,
            selection_head: None,
            selection_dragging: false,
            scrollback_offset: 0,
            paint_cache: Rc::new(RefCell::new(HashMap::new())),
            cache_theme: None,
            cell_metrics: None,
            grid_origin: None,
            last_click: None,
            mouse_down: None,
            cached_scroll: (0, 0),
            cached_alt_screen: false,
            scrollbar_bounds: None,
            scrollbar_dragging: false,
        };

        this.spawn(params, client, cx);
        this
    }

    pub fn with_cwd(
        cwd: impl Into<String>,
        client: ConsoleClient,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let params = TerminalSpawnParams {
            cwd: cwd.into(),
            ..Default::default()
        };
        Self::new(params, client, window, cx)
    }

    fn spawn(
        &mut self,
        params: TerminalSpawnParams,
        client: ConsoleClient,
        cx: &mut Context<Self>,
    ) {
        let size = self.size;

        cx.spawn(async move |this, cx| {
            let service = client.terminal_service();
            let handle = match service.spawn(params, size).await {
                Ok(h) => Arc::new(h),
                Err(e) => {
                    let _ = this.update(cx, |view, cx| {
                        view.status = TerminalStatus::Error;
                        view.error = Some(format!("Failed to spawn terminal: {e}"));
                        cx.notify();
                    });
                    return;
                }
            };

            let handle_for_watch = handle.clone();
            // Termy's internal state is guarded by a *blocking* mutex that the
            // WS reader task holds while parsing output. Locking it on the main
            // thread would stall the UI behind the parser during output bursts
            // (e.g. `git push` progress), so run the whole lock-and-snapshot on
            // the background executor and only hop back to the main thread to
            // apply the result. Snapshot + damage + mouse mode come from one
            // lock via `snapshot_full` so dirty rows stay in sync with the grid
            // and mouse events never touch the backend lock.
            let (initial_snapshot, initial_damage, initial_mouse, initial_scroll, initial_alt, initial_status, initial_error) = cx
                .background_executor()
                .spawn({
                    let handle = handle.clone();
                    async move {
                        let full = handle.snapshot_full().await;
                        let status = handle.status().await;
                        let error = handle.error.read().await.clone();
                        (full.snapshot, full.damage, full.mouse_mode, full.scroll_state, full.is_alt_screen, status, error)
                    }
                })
                .await;

            let _ = this.update(cx, |view, cx| {
                view.handle = Some(handle);
                view.snapshot = Some(Arc::new(initial_snapshot));
                view.snapshot_damage = initial_damage;
                view.cached_mouse_mode = initial_mouse;
                view.cached_scroll = initial_scroll;
                view.cached_alt_screen = initial_alt;
                view.status = initial_status;
                view.error = initial_error;
                cx.notify();
            });

            let this_watch = this.clone();
            cx.spawn(async move |cx| {
                loop {
                    handle_for_watch.notify.notified().await;
                    // Coalesce output bursts (paste, `cat`, prompt redraws) into a
                    // single snapshot + re-render pass instead of one per frame.
                    cx.background_executor()
                        .timer(std::time::Duration::from_millis(8))
                        .await;
                    // Snapshot on the background executor (see the initial
                    // snapshot above): termy's blocking mutex must never be
                    // awaited on the main thread.
                    let handle_for_snapshot = handle_for_watch.clone();
                    let (snapshot, damage, mouse_mode, scroll_state, is_alt_screen, status, error) = cx
                        .background_executor()
                        .spawn(async move {
                            let full = handle_for_snapshot.snapshot_full().await;
                            let status = handle_for_snapshot.status().await;
                            let error = handle_for_snapshot.error.read().await.clone();
                            (full.snapshot, full.damage, full.mouse_mode, full.scroll_state, full.is_alt_screen, status, error)
                        })
                        .await;
                    let _ = this_watch.update(cx, |view, cx| {
                        view.snapshot = Some(Arc::new(snapshot));
                        view.snapshot_damage = damage;
                        view.cached_mouse_mode = mouse_mode;
                        view.cached_scroll = scroll_state;
                        view.cached_alt_screen = is_alt_screen;
                        view.status = status;
                        view.error = error;
                        cx.notify();
                    });
                }
            })
            .detach();
        })
        .detach();
    }

    pub fn send_input(&self, data: String) {
        if let Some(h) = &self.handle {
            h.send_input(data);
        }
    }

    pub fn resize(&mut self, size: TerminalSize, _cx: &mut Context<Self>) {
        self.size = size;
        if let Some(h) = &self.handle {
            h.resize(size);
        }
    }

    pub fn kill(&self) {
        if let Some(h) = &self.handle {
            h.kill();
        }
    }

    pub fn status(&self) -> TerminalStatus {
        self.status
    }

    fn theme(&self, cx: &App) -> TerminalTheme {
        TerminalTheme::from_app_theme(&Theme::current(cx))
    }
    /// The URL link under a grid cell, if any. Only URL targets exist in
    /// snapshots (see `detect_url_links` in console-core).
    pub fn link_target_at(&self, pos: TerminalCellPos) -> Option<String> {
        let snapshot = self.snapshot.as_ref()?;
        snapshot
            .links
            .iter()
            .find(|l| {
                pos.row >= l.start_row
                    && pos.row <= l.end_row
                    && (pos.row > l.start_row || pos.col >= l.start_col)
                    && (pos.row < l.end_row || pos.col <= l.end_col)
            })
            .map(|l| l.target.clone())
    }


    /// Map a mouse position to a grid cell using the measured cell metrics
    /// from the last paint — never constants, which drift with the font and
    /// put clicks/selections on the wrong cells.
    fn cell_at_point(&self, x: Pixels, y: Pixels) -> TerminalCellPos {
        let (cell_w, cell_h) = self.cell_metrics.unwrap_or((px(7.2), px(16.0)));
        // Mouse events are window-relative; the grid is painted at the canvas
        // bounds origin + padding. Subtract it or every click lands offset by
        // wherever the pane sits in the window.
        let origin = self
            .grid_origin
            .unwrap_or_else(|| gpui::point(px(0.0), px(0.0)));
        let col = ((x - origin.x).max(px(0.0)) / cell_w).floor() as u16;
        let row = ((y - origin.y).max(px(0.0)) / cell_h).floor() as u16;
        TerminalCellPos {
            col: col.min(self.size.cols.saturating_sub(1)),
            row: row.min(self.size.rows.saturating_sub(1)),
        }
    }

    /// Mouse-reporting mode, cached per snapshot (see `snapshot_full`).
    /// TUIs like btop/opencode enable it via DECSET 1000/1002/1003/1006.
    /// Cached so mousemove/scroll never touch the backend lock per event.
    fn mouse_mode(&self) -> termy_core::TerminalMouseMode {
        self.cached_mouse_mode
    }

    /// Forward a mouse event to the PTY when the running TUI has enabled
    /// mouse reporting. Returns true when the event was consumed as a report;
    /// callers then skip local selection handling.
    fn forward_mouse_report(
        &self,
        kind: TerminalMouseEventKind,
        pos: TerminalCellPos,
        modifiers: gpui::Modifiers,
    ) -> bool {
        let Some(bytes) = encode_mouse_report(
            self.mouse_mode(),
            kind,
            TerminalMousePosition {
                col: pos.col as usize,
                row: pos.row as usize,
            },
            TerminalMouseModifiers {
                shift: modifiers.shift,
                alt: modifiers.alt,
                control: modifiers.control,
            },
        ) else {
            return false;
        };
        self.send_input(String::from_utf8_lossy(&bytes).into_owned());
        true
    }

    /// Jump back to the live prompt, locally and in the backend. Any fresh
    /// input (keys, paste, drops) calls this so the viewport follows output.
    pub fn scroll_to_bottom(&mut self) {
        self.scrollback_offset = 0;
        if let Some(h) = &self.handle {
            h.scroll_to_bottom();
        }
    }

    /// Scroll the backend so `frac` (0.0 = top of history, 1.0 = live
    /// prompt) sits at the viewport. Drives the scrollbar thumb/track.
    fn scroll_to_frac(&mut self, frac: f32) {
        let (offset, history) = self.cached_scroll;
        if history == 0 {
            return;
        }
        let target = ((1.0 - frac.clamp(0.0, 1.0)) * history as f32).round() as usize;
        if target == offset {
            return;
        }
        let delta = target as i32 - offset as i32;
        if let Some(h) = &self.handle {
            h.scroll(delta);
        }
        self.scrollback_offset = (self.scrollback_offset + delta).max(0);
    }
}

/// Open a URL in the user's default browser. Fire-and-forget: a failure to
/// spawn the opener is logged, never surfaced into the terminal UI.
fn open_url_in_browser(url: &str) {
    #[cfg(target_os = "macos")]
    let result = std::process::Command::new("open").arg(url).spawn();
    #[cfg(target_os = "linux")]
    let result = std::process::Command::new("xdg-open").arg(url).spawn();
    #[cfg(target_os = "windows")]
    let result = std::process::Command::new("cmd")
        .args(["/C", "start", "", url])
        .spawn();
    #[cfg(any(target_os = "macos", target_os = "linux", target_os = "windows"))]
    if let Err(err) = result {
        log::warn!("Failed to open URL '{url}' in browser: {err}");
    }
    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    let _ = url;
}

impl Focusable for TerminalView {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus.clone()
    }
}

impl EntityInputHandler for TerminalView {
    fn text_for_range(
        &mut self,
        _range_utf16: Range<usize>,
        _adjusted_range: &mut Option<Range<usize>>,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) -> Option<String> {
        None
    }

    fn selected_text_range(
        &mut self,
        _ignore_disabled_input: bool,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) -> Option<UTF16Selection> {
        Some(UTF16Selection {
            range: 0..0,
            reversed: false,
        })
    }

    fn marked_text_range(
        &self,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) -> Option<Range<usize>> {
        None
    }

    fn unmark_text(&mut self, _window: &mut Window, _cx: &mut Context<Self>) {}

    fn replace_text_in_range(
        &mut self,
        _range: Option<Range<usize>>,
        text: &str,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if !text.is_empty() {
            self.send_input(text.to_string());
            self.scroll_to_bottom();
            let _ = self.clear_selection();
            cx.notify();
        }
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        _range: Option<Range<usize>>,
        _new_text: &str,
        _new_selected_range: Option<Range<usize>>,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) {
    }

    fn bounds_for_range(
        &mut self,
        _range_utf16: Range<usize>,
        element_bounds: Bounds<Pixels>,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) -> Option<Bounds<Pixels>> {
        Some(element_bounds)
    }

    fn character_index_for_point(
        &mut self,
        _point: gpui::Point<Pixels>,
        _window: &mut Window,
        _cx: &mut Context<Self>,
    ) -> Option<usize> {
        None
    }
}

impl Render for TerminalView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let ttheme = self.theme(cx);

        let status_banner = match self.status {
            TerminalStatus::Spawning => Some(("Spawning shell…", theme.text_ghost)),
            TerminalStatus::Error => self.error.as_deref().map(|e| (e as &str, theme.warning)),
            TerminalStatus::Exited => {
                Some(("Shell exited — close or respawn", theme.text_tertiary))
            }
            TerminalStatus::Running => None,
        };

        let snapshot = self.snapshot.clone();
        let view_handle = cx.entity().clone();
        let view_for_key = view_handle.clone();
        let handle_for_key = self.handle.clone();
        let focus_for_key = self.focus.clone();
        let keyboard_mode = snapshot
            .as_ref()
            .map(|s| s.keyboard_mode)
            .unwrap_or_default();
        let bracketed_paste = snapshot
            .as_ref()
            .map(|s| s.bracketed_paste)
            .unwrap_or(false);
        let selection_range = self.selection_range();
        let render_cursor = self.scrollback_offset == 0;
        // Theme change invalidates every cached row (resolved colors are
        // baked into cached runs).
        if self.cache_theme != Some((ttheme.background, ttheme.foreground)) {
            self.paint_cache.borrow_mut().clear();
            self.cache_theme = Some((ttheme.background, ttheme.foreground));
        }
        let paint_cache = self.paint_cache.clone();
        let view_for_drop = view_handle.clone();
        let focus_for_drop = self.focus.clone();

        // Scrollbar state paired with the snapshot (no backend lock per
        // frame). Hidden on the alt-screen where TUIs own the viewport.
        let (scroll_offset, scroll_history) = self.cached_scroll;
        let show_scrollbar = !self.cached_alt_screen && scroll_history > 0;
        let scrollbar_viewport_rows = self.size.rows.max(1) as f32;
        let scrollbar_track = theme.border.opacity(0.5);
        let scrollbar_thumb = theme.text_ghost.opacity(0.7);
        let scrollbar = {
            let view_for_bar = view_handle.clone();
            div()
                .w(px(10.0))
                .h_full()
                .py(px(4.0))
                .on_mouse_down(
                    MouseButton::Left,
                    cx.listener(
                        move |this, event: &MouseDownEvent, _window, cx| {
                            // Click-to-jump: the grab point becomes the thumb
                            // position, then motion drags it.
                            if let Some(bounds) = this.scrollbar_bounds {
                                let height = f32::from(bounds.size.height);
                                if height > 0.0 {
                                    let frac = (f32::from(event.position.y)
                                        - f32::from(bounds.origin.y))
                                        / height;
                                    this.scroll_to_frac(frac);
                                    this.scrollbar_dragging = true;
                                    cx.notify();
                                }
                            }
                            cx.stop_propagation();
                        },
                    ),
                )
                .on_mouse_move(cx.listener(
                    move |this, event: &MouseMoveEvent, _window, cx| {
                        if this.scrollbar_dragging {
                            if let Some(bounds) = this.scrollbar_bounds {
                                let height = f32::from(bounds.size.height);
                                if height > 0.0 {
                                    let frac = (f32::from(event.position.y)
                                        - f32::from(bounds.origin.y))
                                        / height;
                                    this.scroll_to_frac(frac);
                                    cx.notify();
                                }
                            }
                        }
                    },
                ))
                .on_mouse_up(
                    MouseButton::Left,
                    cx.listener(move |this, _event: &MouseUpEvent, _window, cx| {
                        if this.scrollbar_dragging {
                            this.scrollbar_dragging = false;
                            cx.notify();
                        }
                    }),
                )
                .child(
                    gpui::canvas(
                        move |_bounds, _window, _cx| (),
                        move |bounds, _, window, cx| {
                            view_for_bar.update(cx, |view, _| {
                                view.scrollbar_bounds = Some(bounds);
                            });
                            let total = scroll_history as f32 + scrollbar_viewport_rows;
                            let height = f32::from(bounds.size.height);
                            if height <= 0.0 || total <= 0.0 {
                                return;
                            }
                            window.paint_quad(gpui::fill(bounds, scrollbar_track));
                            // `min` before `max`: `clamp` would panic on a
                            // track shorter than the minimum thumb.
                            let thumb_h = (height * scrollbar_viewport_rows / total)
                                .min(height)
                                .max(8.0);
                            let max_top = (height - thumb_h).max(0.0);
                            let frac = if scroll_history == 0 {
                                0.0
                            } else {
                                (scroll_offset as f32 / scroll_history as f32).clamp(0.0, 1.0)
                            };
                            let thumb_top = max_top * (1.0 - frac);
                            let width = f32::from(bounds.size.width);
                            let thumb = gpui::Bounds {
                                origin: gpui::point(
                                    bounds.origin.x + px(3.0),
                                    bounds.origin.y + px(thumb_top),
                                ),
                                size: gpui::size(px((width - 6.0).max(2.0)), px(thumb_h)),
                            };
                            window.paint_quad(gpui::fill(thumb, scrollbar_thumb));
                        },
                    )
                    .size_full(),
                )
        };

        div()
            .id("terminal-view")
            .key_context("Terminal")
            .track_focus(&focus_for_key)
            .on_action(cx.listener(|this, _: &TerminalTab, window, cx| {
                this.send_tab(false, window, cx);
            }))
            .on_action(cx.listener(|this, _: &TerminalShiftTab, window, cx| {
                this.send_tab(true, window, cx);
            }))
            .size_full()
            .flex()
            .flex_col()
            .bg(ttheme.background)
            .text_color(ttheme.foreground)
            .overflow_hidden()
            .on_key_down(
                move |event: &KeyDownEvent, window: &mut Window, cx: &mut App| {
                    if !focus_for_key.is_focused(window) {
                        window.focus(&focus_for_key, cx);
                    }

                    // Copy shortcut: Cmd+C (macOS) / Ctrl+Shift+C
                    let is_copy = (event.keystroke.modifiers.platform
                        && event.keystroke.key == "c")
                        || (event.keystroke.modifiers.control
                            && event.keystroke.modifiers.shift
                            && event.keystroke.key == "c");
                    if is_copy {
                        let text = view_for_key.read(cx).selected_text();
                        if let Some(text) = text {
                            cx.write_to_clipboard(gpui::ClipboardItem::new_string(text));
                            cx.stop_propagation();
                            return;
                        }
                    }

                    let is_paste = (event.keystroke.modifiers.platform
                        && event.keystroke.key == "v")
                        || (event.keystroke.modifiers.control
                            && event.keystroke.modifiers.shift
                            && event.keystroke.key == "v");
                    if is_paste {
                        if let Some(clipboard) = cx.read_from_clipboard() {
                            if let Some(h) = &handle_for_key {
                                if let Some(input) =
                                    paste_input_for_clipboard(&clipboard, bracketed_paste)
                                {
                                    h.send_input(input);
                                }
                            }
                        }
                        view_for_key.update(cx, |view, cx| {
                            view.scroll_to_bottom();
                            cx.notify();
                        });
                        cx.stop_propagation();
                        return;
                    }

                    // Select-all: Cmd+A (macOS) / Ctrl+Shift+A. Ctrl+A alone
                    // still goes to the shell (line start in readline).
                    let is_select_all = (event.keystroke.modifiers.platform
                        && event.keystroke.key == "a")
                        || (event.keystroke.modifiers.control
                            && event.keystroke.modifiers.shift
                            && event.keystroke.key == "a");
                    if is_select_all {
                        view_for_key.update(cx, |view, cx| {
                            view.select_all();
                            cx.notify();
                        });
                        cx.stop_propagation();
                        return;
                    }

                    // Printable characters without Ctrl/Alt/Platform are delegated to IME / InputHandler
                    let key = event.keystroke.key.as_str();
                    let is_plain_printable = event.keystroke.key_char.is_some()
                        && !event.keystroke.modifiers.control
                        && !event.keystroke.modifiers.alt
                        && !event.keystroke.modifiers.platform
                        && !event.keystroke.modifiers.function
                        && !matches!(
                            key,
                            "enter" | "tab" | "space" | "backspace" | "escape" | "delete"
                        );
                    if is_plain_printable {
                        // Let the event propagate to EntityInputHandler::replace_text_in_range
                        return;
                    }

                    if let Some(bytes) = TerminalView::key_to_bytes(event, keyboard_mode) {
                        if let Some(h) = &handle_for_key {
                            h.send_input(bytes);
                        }
                        view_for_key.update(cx, |view, cx| {
                            view.scroll_to_bottom();
                            if view.clear_selection() {
                                cx.notify();
                            }
                        });
                        cx.stop_propagation();
                    }
                },
            )
            .on_scroll_wheel(
                cx.listener(move |this, event: &ScrollWheelEvent, _window, cx| {
                    let delta = match event.delta {
                        gpui::ScrollDelta::Lines(lines) => lines.y.round() as i32,
                        gpui::ScrollDelta::Pixels(pixels) => {
                            (f32::from(pixels.y) / 16.0).round() as i32
                        }
                    };
                    if delta == 0 {
                        return;
                    }
                    // TUI mouse reporting: forward wheel as WheelUp/WheelDown
                    // reports (one per notch) instead of scrolling local
                    // scrollback. Shift bypasses so scrollback still works.
                    let mode = this.mouse_mode();
                    if mode.enabled && !event.modifiers.shift {
                        let pos = this.cell_at_point(event.position.x, event.position.y);
                        let kind = if delta > 0 {
                            TerminalMouseEventKind::WheelUp
                        } else {
                            TerminalMouseEventKind::WheelDown
                        };
                        for _ in 0..delta.unsigned_abs().min(10) {
                            this.forward_mouse_report(kind, pos, event.modifiers);
                        }
                        cx.stop_propagation();
                        return;
                    }
                    if let Some(h) = &this.handle {
                        h.scroll(delta);
                        this.scrollback_offset = (this.scrollback_offset + delta).max(0);
                        cx.stop_propagation();
                    }
                }),
            )
            .when_some(status_banner, |el, (msg, color)| {
                el.child(
                    div()
                        .px(px(12.0))
                        .py(px(6.0))
                        .text_size(px(11.0))
                        .text_color(color)
                        .child(SharedString::from(msg.to_string())),
                )
            })
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .w_full()
                    .overflow_hidden()
                    .flex()
                    .flex_row()
                    .on_drop(move |paths: &ExternalPaths, window, cx| {
                        // Finder drops become quoted paths typed at the
                        // prompt, mirroring termy's file-drop behavior.
                        if let Some(input) = dropped_paths_input(paths.paths()) {
                            view_for_drop.update(cx, |view, cx| {
                                view.send_input(input);
                                view.scroll_to_bottom();
                                cx.notify();
                            });
                            window.focus(&focus_for_drop, cx);
                        }
                    })
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .min_h_0()
                            .overflow_hidden()
                            .on_mouse_down(
                                MouseButton::Left,
                        cx.listener(move |this, event: &MouseDownEvent, _window, cx| {
                            let pos = this.cell_at_point(event.position.x, event.position.y);

                            // TUI mouse reporting (btop, opencode, ...): when
                            // the shell enabled mouse mode, forward the click
                            // as an escape report instead of doing local
                            // selection. Shift bypasses reporting so text can
                            // still be selected and copied.
                            if !event.modifiers.shift
                                && this.forward_mouse_report(
                                    TerminalMouseEventKind::Press(TerminalMouseButton::Left),
                                    pos,
                                    event.modifiers,
                                )
                            {
                                this.mouse_down = Some(TerminalMouseButton::Left);
                                cx.notify();
                                return;
                            }

                            // Multi-click selection: double-click selects the
                            // word token, triple-click the full row. Count
                            // cycles 1 → 2 → 3 on same-cell clicks within
                            // 500ms. Links still open via Cmd+Click below.
                            let now = std::time::Instant::now();
                            let count = match this.last_click {
                                Some((t, p, c))
                                    if p == pos
                                        && now.duration_since(t).as_millis() < 500 =>
                                {
                                    (c % 3) + 1
                                }
                                _ => 1,
                            };
                            this.last_click = Some((now, pos, count));
                            if count == 2 {
                                if this.select_token_at_cell(pos) {
                                    cx.notify();
                                    return;
                                }
                                // Nothing word-like under the cursor — fall
                                // through to a fresh single-click selection.
                            } else if count == 3 {
                                if this.select_line_at_row(pos.row) {
                                    cx.notify();
                                    return;
                                }
                            }

                            // Cmd+Click (Ctrl+Click elsewhere) on a URL opens
                            // it in the default browser instead of selecting.
                            if event.modifiers.platform || event.modifiers.control {
                                if let Some(url) = this.link_target_at(pos) {
                                    this.clear_selection();
                                    cx.notify();
                                    open_url_in_browser(&url);
                                    return;
                                }
                            }

                            this.selection_anchor = Some(pos);
                            this.selection_head = Some(pos);
                            this.selection_dragging = true;
                            cx.notify();
                        }),
                    )
                    .on_mouse_move(
                        cx.listener(move |this, event: &MouseMoveEvent, _window, cx| {
                            // TUI mouse reporting: drag (1002) or full motion
                            // (1003) reports instead of local selection drags.
                            if this.mouse_mode().enabled {
                                let pos = this.cell_at_point(event.position.x, event.position.y);
                                match this.mouse_down {
                                    Some(button) => {
                                        if !this.forward_mouse_report(
                                            TerminalMouseEventKind::Drag(button),
                                            pos,
                                            event.modifiers,
                                        ) && this.mouse_mode().report_motion
                                        {
                                            this.forward_mouse_report(
                                                TerminalMouseEventKind::Move,
                                                pos,
                                                event.modifiers,
                                            );
                                        }
                                    }
                                    None => {
                                        if this.mouse_mode().report_motion {
                                            this.forward_mouse_report(
                                                TerminalMouseEventKind::Move,
                                                pos,
                                                event.modifiers,
                                            );
                                        }
                                    }
                                }
                                return;
                            }
                            if this.selection_dragging {
                                let pos = this.cell_at_point(event.position.x, event.position.y);
                                if this.selection_head != Some(pos) {
                                    this.selection_head = Some(pos);
                                    cx.notify();
                                }
                            }
                        }),
                    )
                    .on_mouse_up(
                        MouseButton::Left,
                        cx.listener(move |this, event: &MouseUpEvent, _window, cx| {
                            // Release report for an in-progress TUI mouse drag/click.
                            if let Some(button) = this.mouse_down.take() {
                                let pos = this.cell_at_point(event.position.x, event.position.y);
                                this.forward_mouse_report(
                                    TerminalMouseEventKind::Release(button),
                                    pos,
                                    event.modifiers,
                                );
                                cx.notify();
                                return;
                            }
                            if this.selection_dragging {
                                this.selection_dragging = false;
                                if this.selection_anchor == this.selection_head {
                                    this.clear_selection();
                                }
                                cx.notify();
                            }
                        }),
                    )
                    .child(
                        gpui::canvas(
                            {
                                // Measure once: the font is fixed (12px mono), so reuse
                                // the last measured cell metrics instead of shaping
                                // "0123456789" on every layout pass.
                                let cached_metrics = self.cell_metrics;
                                move |bounds, window, _cx| {
                                    let pad_x = px(8.0);
                                    let pad_y = px(8.0);
                                    let avail_w = (bounds.size.width - pad_x * 2.0).max(px(0.0));
                                    let avail_h = (bounds.size.height - pad_y * 2.0).max(px(0.0));

                                    let (cell_w, cell_h) = if let Some((w, h)) = cached_metrics {
                                        (w, h)
                                    } else {
                                        let run = gpui::TextRun {
                                            len: 10,
                                            font: gpui::font(crate::markdown::render::MONO_FAMILY),
                                            color: gpui::white(),
                                            ..Default::default()
                                        };
                                        let sample = window.text_system().shape_line(
                                            SharedString::from("0123456789"),
                                            px(12.0),
                                            &[run],
                                            None,
                                        );
                                        let w = (sample.width / 10.0).max(px(1.0));
                                        // Derive row height from the shaped line
                                        // metrics so mouse mapping never drifts
                                        // from the painted grid.
                                        let h = (sample.ascent + sample.descent).max(px(1.0));
                                        (w, h)
                                    };

                                    let cols = ((avail_w / cell_w).floor() as u16).max(20);
                                    let rows = ((avail_h / cell_h).floor() as u16).max(5);

                                    (cols, rows, cell_w, cell_h)
                                }
                            },
                            {
                                let snapshot = snapshot.clone();
                                let damage = self.snapshot_damage.clone();
                                let paint_cache = paint_cache.clone();
                                let view_for_canvas = view_handle.clone();
                                let focus_for_canvas = self.focus.clone();
                                move |bounds, (cols, rows, cell_w, cell_h), window, cx| {
                                    window.handle_input(
                                        &focus_for_canvas,
                                        ElementInputHandler::new(bounds, view_for_canvas.clone()),
                                        cx,
                                    );

                                    view_for_canvas.update(cx, |view, _| {
                                        view.cell_metrics = Some((cell_w, cell_h));
                                        // Window-space grid origin, used by
                                        // cell_at_point to map mouse events.
                                        view.grid_origin =
                                            Some(bounds.origin + gpui::point(px(8.0), px(8.0)));
                                        if view.size.cols != cols || view.size.rows != rows {
                                            view.size = TerminalSize { cols, rows };
                                            if let Some(h) = &view.handle {
                                                h.resize(view.size);
                                            }
                                        }
                                    });

                                    render_canvas_grid(
                                        bounds,
                                        cols,
                                        rows,
                                        cell_w,
                                        cell_h,
                                        snapshot.as_deref(),
                                        &damage,
                                        selection_range,
                                        render_cursor,
                                        &paint_cache,
                                        ttheme,
                                        window,
                                        cx,
                                    );
                                }
                            },
                        )
                        .size_full(),
                    ),
                )
                .when(show_scrollbar, |el| el.child(scrollbar)),
            )
    }
}
pub fn estimate_size(width: f32, height: f32, font_size: f32) -> TerminalSize {
    let cols = (width / (font_size * 0.62)).floor() as u16;
    let rows = (height / (font_size * 1.35)).floor() as u16;
    TerminalSize {
        cols: cols.max(20),
        rows: rows.max(5),
    }
}

pub fn advance_backend<B: console_core::types::terminal::TerminalBackend>(
    backend: &mut B,
    data: &str,
) -> console_core::types::terminal::TerminalGridSnapshot {
    backend.advance(data);
    backend.snapshot()
}
