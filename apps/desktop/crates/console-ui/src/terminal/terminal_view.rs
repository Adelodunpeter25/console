use console_core::ConsoleClient;
use console_core::types::terminal::{TerminalSize, TerminalSpawnParams, TerminalStatus};
use gpui::{
    App, Bounds, Context, ElementInputHandler, EntityInputHandler, FocusHandle, Focusable,
    IntoElement, KeyDownEvent, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent,
    ParentElement, Pixels, Render, ScrollWheelEvent, SharedString, Styled, UTF16Selection, Window,
    div, prelude::*, px,
};
use termy_core::{
    TerminalKeyEventKind, TerminalKeyboardMode, TermyKeystroke, TermyModifiers, keystroke_to_input,
    TerminalMouseButton, TerminalMouseEventKind, TerminalMouseModifiers, TerminalMousePosition,
    encode_mouse_report,
};
use std::cell::RefCell;
use std::collections::HashMap;
use std::ops::Range;
use std::rc::Rc;
use std::sync::Arc;

use super::actions::{TerminalShiftTab, TerminalTab};
use super::theme::TerminalTheme;
use crate::theme::Theme;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct TerminalCellPos {
    pub col: u16,
    pub row: u16,
}

/// Drop-in terminal pane. Owns its `AlacrittyBackend` + WS `TerminalHandle`,
/// feeds server output → grid → snapshot, and forwards keyboard → `input`.
///
/// One terminal row, pre-shaped. Repainting it costs a few quads plus cached
/// glyph paint instead of a full text-shaping pass — this is what keeps
/// constantly-redrawing TUIs (btop, editors) cheap: unchanged rows never
/// re-shape, no matter how often the frame repaints.
struct CachedRowPaint {
    hash: u64,
    bg_runs: Vec<(usize, usize, gpui::Hsla)>,
    text_runs: Vec<CachedTextRun>,
}

struct CachedTextRun {
    start_col: usize,
    shaped: gpui::ShapedLine,
}

fn hash_cell_runs(runs: &[CellRun]) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    for run in runs {
        run.text.hash(&mut h);
        run.count.hash(&mut h);
        for f in [
            run.fg.h, run.fg.s, run.fg.l, run.fg.a, run.bg.h, run.bg.s, run.bg.l,
            run.bg.a,
        ] {
            f.to_bits().hash(&mut h);
        }
        run.bold.hash(&mut h);
        run.italic.hash(&mut h);
        run.underline.hash(&mut h);
    }
    h.finish()
}

struct CellRun {
    start_col: usize,
    count: usize,
    fg: gpui::Hsla,
    bg: gpui::Hsla,
    bold: bool,
    italic: bool,
    underline: bool,
    text: String,
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
    snapshot: Option<console_core::types::terminal::TerminalGridSnapshot>,
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
    /// Last mouse-down for double-click-to-open detection.
    last_click: Option<(std::time::Instant, TerminalCellPos)>,
    /// Button currently held while the PTY has mouse reporting enabled —
    /// drives drag/motion reports. `None` when reporting is off.
    mouse_down: Option<TerminalMouseButton>,
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
            // apply the result.
            let (initial_snapshot, initial_status, initial_error) = cx
                .background_executor()
                .spawn({
                    let handle = handle.clone();
                    async move {
                        let snapshot = handle.snapshot().await;
                        let status = handle.status().await;
                        let error = handle.error.read().await.clone();
                        (snapshot, status, error)
                    }
                })
                .await;

            let _ = this.update(cx, |view, cx| {
                view.handle = Some(handle);
                view.snapshot = Some(initial_snapshot);
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
                    let (snapshot, status, error) = cx
                        .background_executor()
                        .spawn(async move {
                            let snapshot = handle_for_snapshot.snapshot().await;
                            let status = handle_for_snapshot.status().await;
                            let error = handle_for_snapshot.error.read().await.clone();
                            (snapshot, status, error)
                        })
                        .await;
                    let _ = this_watch.update(cx, |view, cx| {
                        view.snapshot = Some(snapshot);
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
    pub fn selected_text(&self) -> Option<String> {
        let (start, end) = self.selection_range()?;
        let snapshot = self.snapshot.as_ref()?;
        let mut lines = Vec::new();

        for row_idx in start.row..=end.row {
            let Some(row) = snapshot.rows.get(row_idx as usize) else {
                continue;
            };
            let start_c = if row_idx == start.row { start.col } else { 0 };
            let end_c = if row_idx == end.row {
                end.col
            } else {
                (row.len() as u16).saturating_sub(1)
            };

            let mut line_str = String::new();
            for col_idx in start_c..=end_c {
                if let Some(cell) = row.get(col_idx as usize) {
                    if !cell.flags.wide_char_spacer {
                        line_str.push(cell.c);
                    }
                }
            }
            lines.push(line_str.trim_end().to_string());
        }

        if lines.is_empty() {
            None
        } else {
            Some(lines.join("\n"))
        }
    }

    pub fn selection_range(&self) -> Option<(TerminalCellPos, TerminalCellPos)> {
        let anchor = self.selection_anchor?;
        let head = self.selection_head?;
        if (head.row, head.col) < (anchor.row, anchor.col) {
            Some((head, anchor))
        } else {
            Some((anchor, head))
        }
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

    pub fn clear_selection(&mut self) -> bool {
        let had = self.selection_anchor.is_some() || self.selection_head.is_some();
        self.selection_anchor = None;
        self.selection_head = None;
        self.selection_dragging = false;
        had
    }

    fn key_to_bytes(event: &KeyDownEvent, mode: TerminalKeyboardMode) -> Option<String> {
        Self::keystroke_to_bytes(
            event.keystroke.key.as_str(),
            event.keystroke.key_char.clone(),
            &event.keystroke.modifiers,
            mode,
        )
    }

    /// Mapping core shared by the `on_key_down` path and the Tab actions
    /// (which never see the original key event).
    fn keystroke_to_bytes(
        key: &str,
        key_char: Option<String>,
        modifiers: &gpui::Modifiers,
        mode: TerminalKeyboardMode,
    ) -> Option<String> {
        let mods = TermyModifiers {
            control: modifiers.control,
            alt: modifiers.alt,
            shift: modifiers.shift,
            platform: modifiers.platform,
            function: modifiers.function,
        };
        let ks = TermyKeystroke {
            key: key.to_owned(),
            key_char,
            modifiers: mods,
        };
        let bytes = keystroke_to_input(&ks, TerminalKeyEventKind::Press, mode, true)?;
        Some(String::from_utf8_lossy(&bytes).into_owned())
    }

    /// Send Tab / Shift-Tab from the keymap actions. The binding identity
    /// carries the key; live modifiers are sampled synchronously during
    /// dispatch, so this reconstructs exactly what `on_key_down` would see.
    /// Stops propagation so the global focus-traversal binding never fires
    /// while the terminal is focused.
    fn send_tab(&mut self, shift: bool, window: &mut Window, cx: &mut Context<Self>) {
        let mut modifiers = window.modifiers();
        modifiers.shift = shift;
        let mode = self
            .snapshot
            .as_ref()
            .map(|s| s.keyboard_mode)
            .unwrap_or_default();
        match Self::keystroke_to_bytes("tab", Some("\t".to_owned()), &modifiers, mode) {
            Some(bytes) => {
                log::debug!("terminal tab action -> {} bytes to pty", bytes.len());
                self.send_input(bytes);
                if self.clear_selection() {
                    cx.notify();
                }
                cx.stop_propagation();
            }
            None => {
                log::debug!("terminal tab action swallowed: no bytes produced");
            }
        }
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

    /// Mouse-reporting mode of the live backend (default = disabled). TUIs
    /// like btop/opencode enable it via DECSET 1000/1002/1003/1006.
    fn mouse_mode(&self) -> termy_core::TerminalMouseMode {
        self.handle
            .as_ref()
            .and_then(|h| h.backend.try_lock().ok().map(|b| b.mouse_mode()))
            .unwrap_or_default()
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
                            if let Some(text) = clipboard.text() {
                                if let Some(h) = &handle_for_key {
                                    if bracketed_paste {
                                        h.send_input(format!("\x1b[200~{}\x1b[201~", text));
                                    } else {
                                        h.send_input(text);
                                    }
                                }
                            }
                        }
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
                            view.scrollback_offset = 0;
                            if view.clear_selection() {
                                cx.notify();
                            }
                        });
                        cx.stop_propagation();
                    }
                },
            )
            .on_scroll_wheel(cx.listener(
                move |this, event: &ScrollWheelEvent, _window, cx| {
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
                },
            ))
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

                            // Double-click opens whatever termy resolved at
                            // the cell (OSC 8 hyperlink, file path, URL) —
                            // richer than the URL-regex list used for
                            // single Cmd+Click highlighting.
                            let now = std::time::Instant::now();
                            let is_double = matches!(this.last_click, Some((t, p))
                                if p == pos && now.duration_since(t).as_millis() < 500);
                            this.last_click = Some((now, pos));
                            if is_double {
                                this.clear_selection();
                                if let Some(h) = this.handle.clone() {
                                    if let Ok(b) = h.backend.try_lock() {
                                        if let Some(link) =
                                            b.link_at(pos.row as usize, pos.col as usize)
                                        {
                                            let target = link.target.clone();
                                            drop(b);
                                            cx.notify();
                                            open_url_in_browser(&target);
                                            return;
                                        }
                                    }
                                }
                                cx.notify();
                                return;
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
                    .on_mouse_move(cx.listener(move |this, event: &MouseMoveEvent, _window, cx| {
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
                    }))
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
                            move |bounds, window, _cx| {
                                let pad_x = px(8.0);
                                let pad_y = px(8.0);
                                let avail_w = (bounds.size.width - pad_x * 2.0).max(px(0.0));
                                let avail_h = (bounds.size.height - pad_y * 2.0).max(px(0.0));

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
                                let cell_w = (sample.width / 10.0).max(px(1.0));
                                let cell_h = px(16.0);

                                let cols = ((avail_w / cell_w).floor() as u16).max(20);
                                let rows = ((avail_h / cell_h).floor() as u16).max(5);

                                (cols, rows, cell_w, cell_h)
                            },
                            {
                                let snapshot = snapshot.clone();
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
                                        snapshot.as_ref(),
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
    }
}

fn color_to_hsla(c: console_core::types::terminal::TerminalColor) -> gpui::Hsla {
    let r = c.r as f32 / 255.0;
    let g = c.g as f32 / 255.0;
    let b = c.b as f32 / 255.0;
    gpui::Rgba { r, g, b, a: 1.0 }.into()
}

fn render_canvas_grid(
    bounds: gpui::Bounds<gpui::Pixels>,
    cols: u16,
    rows: u16,
    cell_w: gpui::Pixels,
    cell_h: gpui::Pixels,
    snapshot: Option<&console_core::types::terminal::TerminalGridSnapshot>,
    selection_range: Option<(TerminalCellPos, TerminalCellPos)>,
    render_cursor: bool,
    paint_cache: &Rc<RefCell<HashMap<u16, CachedRowPaint>>>,
    theme: TerminalTheme,
    window: &mut Window,
    cx: &mut App,
) {
    let pad_x = px(8.0);
    let pad_y = px(8.0);
    let origin = bounds.origin + gpui::point(pad_x, pad_y);

    window.paint_quad(gpui::fill(bounds, theme.background));

    let Some(snap) = snapshot else {
        return;
    };

    let cursor = if render_cursor {
        snap.cursor
    } else {
        console_core::types::terminal::CursorPosition {
            col: 0,
            row: 0,
            visible: false,
        }
    };

    for (row_idx, row) in snap.rows.iter().enumerate() {
        if row_idx as u16 >= rows {
            break;
        }
        let y = origin.y + cell_h * row_idx as f32;

        // Collect the links overlapping this row once per row instead of
        // scanning the full link list for every cell (O(cells × links) →
        // O(cells × row_links)).
        let row_links: Vec<&console_core::types::terminal::TerminalLink> = snap
            .links
            .iter()
            .filter(|l| row_idx as u16 >= l.start_row && row_idx as u16 <= l.end_row)
            .collect();

        let mut runs: Vec<CellRun> = Vec::new();

        for (col_idx, cell) in row.iter().enumerate() {
            if col_idx as u16 >= cols {
                break;
            }

            let mut fg = cell.fg.map(color_to_hsla).unwrap_or(theme.foreground);
            let mut bg = cell.bg.map(color_to_hsla).unwrap_or(theme.background);

            let is_selected = if let Some((start, end)) = selection_range {
                let r = row_idx as u16;
                let c = col_idx as u16;
                if r > start.row && r < end.row {
                    true
                } else if r == start.row && r == end.row {
                    c >= start.col && c <= end.col
                } else if r == start.row {
                    c >= start.col
                } else if r == end.row {
                    c <= end.col
                } else {
                    false
                }
            } else {
                false
            };

            if is_selected {
                bg = theme.selection;
            }

            if cell.flags.inverse {
                std::mem::swap(&mut fg, &mut bg);
            }
            if cell.flags.dim {
                fg.a *= 0.6;
            }
            if cell.flags.hidden {
                fg = bg;
            }

            let is_cursor =
                cursor.visible && row_idx as u16 == cursor.row && col_idx as u16 == cursor.col;
            if is_cursor {
                bg = theme.cursor;
                fg = theme.cursor_text;
            }

            let c = if cell.flags.wide_char_spacer {
                ' '
            } else {
                cell.c
            };

            let mut is_link = false;
            for l in &row_links {
                let r = row_idx as u16;
                let c = col_idx as u16;
                let in_start = r > l.start_row || c >= l.start_col;
                let in_end = r < l.end_row || c <= l.end_col;
                if in_start && in_end {
                    is_link = true;
                    break;
                }
            }

            if is_link {
                fg = gpui::blue();
            }

            let cell_underline = cell.flags.underline || is_link;

            let can_merge = if let Some(last) = runs.last() {
                last.fg == fg
                    && last.bg == bg
                    && last.bold == cell.flags.bold
                    && last.italic == cell.flags.italic
                    && last.underline == cell_underline
            } else {
                false
            };

            if can_merge {
                let last = runs.last_mut().unwrap();
                last.count += 1;
                last.text.push(c);
            } else {
                runs.push(CellRun {
                    start_col: col_idx,
                    count: 1,
                    fg,
                    bg,
                    bold: cell.flags.bold,
                    italic: cell.flags.italic,
                    underline: cell_underline,
                    text: c.to_string(),
                });
            }
        }

        // Shape cache: unchanged rows repaint stored quads + shaped lines
        // without shaping. Cursor/selection rows bypass (that state isn't in
        // the hash).
        let row_u16 = row_idx as u16;
        let cursor_here = cursor.visible && row_u16 == cursor.row;
        let selection_here = match selection_range {
            Some((start, end)) => row_u16 >= start.row && row_u16 <= end.row,
            None => false,
        };
        let cacheable = !cursor_here && !selection_here;
        let row_hash: Option<u64> = if cacheable { Some(hash_cell_runs(&runs)) } else { None };
        if let Some(hash) = row_hash {
            let hit = paint_cache
                .borrow()
                .get(&row_u16)
                .map(|entry| entry.hash == hash)
                .unwrap_or(false);
            if hit {
                if let Some(entry) = paint_cache.borrow().get(&row_u16) {
                    for (start_col, count, bg) in &entry.bg_runs {
                        if *bg != theme.background {
                            let bg_quad = gpui::Bounds {
                                origin: gpui::point(origin.x + cell_w * *start_col as f32, y),
                                size: gpui::size(cell_w * *count as f32, cell_h),
                            };
                            window.paint_quad(gpui::fill(bg_quad, *bg));
                        }
                    }
                    for text in &entry.text_runs {
                        let _ = text.shaped.paint(
                            gpui::point(origin.x + cell_w * text.start_col as f32, y),
                            cell_h,
                            gpui::TextAlign::Left,
                            None,
                            window,
                            cx,
                        );
                    }
                }
                continue;
            }
        }

        let mut cached_bg: Vec<(usize, usize, gpui::Hsla)> = Vec::new();
        let mut cached_text: Vec<CachedTextRun> = Vec::new();

        for run in runs {
            let run_x = origin.x + cell_w * run.start_col as f32;
            let run_w = cell_w * run.count as f32;

            if run.bg != theme.background {
                let bg_quad = gpui::Bounds {
                    origin: gpui::point(run_x, y),
                    size: gpui::size(run_w, cell_h),
                };
                window.paint_quad(gpui::fill(bg_quad, run.bg));
                if cacheable {
                    cached_bg.push((run.start_col, run.count, run.bg));
                }
            }

            if !run.text.trim().is_empty() || run.underline {
                let font_weight = if run.bold {
                    gpui::FontWeight::BOLD
                } else {
                    gpui::FontWeight::NORMAL
                };
                let font_style = if run.italic {
                    gpui::FontStyle::Italic
                } else {
                    gpui::FontStyle::Normal
                };

                let text_run = gpui::TextRun {
                    len: run.text.len(),
                    font: gpui::Font {
                        family: SharedString::from(crate::markdown::render::MONO_FAMILY),
                        weight: font_weight,
                        style: font_style,
                        features: Default::default(),
                        fallbacks: None,
                    },
                    color: run.fg,
                    background_color: None,
                    underline: if run.underline {
                        Some(gpui::UnderlineStyle {
                            color: Some(run.fg),
                            thickness: px(1.0),
                            wavy: false,
                        })
                    } else {
                        None
                    },
                    strikethrough: None,
                };

                let shaped = window.text_system().shape_line(
                    SharedString::from(run.text),
                    px(12.0),
                    &[text_run],
                    None,
                );
                if cacheable {
                    cached_text.push(CachedTextRun {
                        start_col: run.start_col,
                        shaped: shaped.clone(),
                    });
                }
                let _ = shaped.paint(
                    gpui::point(run_x, y),
                    cell_h,
                    gpui::TextAlign::Left,
                    None,
                    window,
                    cx,
                );
            }
        }

        if cacheable {
            paint_cache.borrow_mut().insert(
                row_u16,
                CachedRowPaint {
                    hash: row_hash.unwrap_or(0),
                    bg_runs: cached_bg,
                    text_runs: cached_text,
                },
            );
        }
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
