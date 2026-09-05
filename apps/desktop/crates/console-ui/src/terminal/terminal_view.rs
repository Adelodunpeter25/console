use console_core::ConsoleClient;
use console_core::types::terminal::{TerminalSize, TerminalSpawnParams, TerminalStatus};
use gpui::{
    App, Bounds, Context, ElementInputHandler, EntityInputHandler, FocusHandle, Focusable,
    IntoElement, KeyDownEvent, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent,
    ParentElement, Pixels, Render, SharedString, Styled, UTF16Selection, Window, div,
    prelude::*, px,
};
use termy_core::{
    TerminalKeyEventKind, TerminalKeyboardMode, TermyKeystroke, TermyModifiers, keystroke_to_input,
};
use std::ops::Range;
use std::sync::Arc;

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
/// **Usage (not yet wired into `ConsoleDesktopApp`):**
/// ```rust
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

    pub fn clear_selection(&mut self) -> bool {
        let had = self.selection_anchor.is_some() || self.selection_head.is_some();
        self.selection_anchor = None;
        self.selection_head = None;
        self.selection_dragging = false;
        had
    }

    fn key_to_bytes(event: &KeyDownEvent, mode: TerminalKeyboardMode) -> Option<String> {
        let mods = TermyModifiers {
            control: event.keystroke.modifiers.control,
            alt: event.keystroke.modifiers.alt,
            shift: event.keystroke.modifiers.shift,
            platform: event.keystroke.modifiers.platform,
            function: event.keystroke.modifiers.function,
        };
        let ks = TermyKeystroke {
            key: event.keystroke.key.clone(),
            key_char: event.keystroke.key_char.clone(),
            modifiers: mods,
        };
        let bytes = keystroke_to_input(&ks, TerminalKeyEventKind::Press, mode, true)?;
        Some(String::from_utf8_lossy(&bytes).into_owned())
    }
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
        let handle_for_scroll = self.handle.clone();
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

        div()
            .id("terminal-view")
            .key_context("Terminal")
            .track_focus(&focus_for_key)
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
                            if view.clear_selection() {
                                cx.notify();
                            }
                        });
                        cx.stop_propagation();
                    }
                },
            )
            .on_scroll_wheel(move |event, _window, cx| {
                if let Some(h) = &handle_for_scroll {
                    let delta = match event.delta {
                        gpui::ScrollDelta::Lines(lines) => lines.y.round() as i32,
                        gpui::ScrollDelta::Pixels(pixels) => {
                            (f32::from(pixels.y) / 16.0).round() as i32
                        }
                    };
                    if delta != 0 {
                        h.scroll(delta);
                        cx.stop_propagation();
                    }
                }
            })
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
                            let cell_w = px(7.2);
                            let cell_h = px(16.0);
                            let col = ((event.position.x - px(8.0)).max(px(0.0)) / cell_w).floor() as u16;
                            let row = ((event.position.y - px(8.0)).max(px(0.0)) / cell_h).floor() as u16;
                            let pos = TerminalCellPos {
                                col: col.min(this.size.cols.saturating_sub(1)),
                                row: row.min(this.size.rows.saturating_sub(1)),
                            };
                            this.selection_anchor = Some(pos);
                            this.selection_head = Some(pos);
                            this.selection_dragging = true;
                            cx.notify();
                        }),
                    )
                    .on_mouse_move(cx.listener(move |this, event: &MouseMoveEvent, _window, cx| {
                        if this.selection_dragging {
                            let cell_w = px(7.2);
                            let cell_h = px(16.0);
                            let col = ((event.position.x - px(8.0)).max(px(0.0)) / cell_w).floor() as u16;
                            let row = ((event.position.y - px(8.0)).max(px(0.0)) / cell_h).floor() as u16;
                            let pos = TerminalCellPos {
                                col: col.min(this.size.cols.saturating_sub(1)),
                                row: row.min(this.size.rows.saturating_sub(1)),
                            };
                            if this.selection_head != Some(pos) {
                                this.selection_head = Some(pos);
                                cx.notify();
                            }
                        }
                    }))
                    .on_mouse_up(
                        MouseButton::Left,
                        cx.listener(move |this, _event: &MouseUpEvent, _window, cx| {
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
                                let view_for_canvas = view_handle.clone();
                                let focus_for_canvas = self.focus.clone();
                                move |bounds, (cols, rows, cell_w, cell_h), window, cx| {
                                    window.handle_input(
                                        &focus_for_canvas,
                                        ElementInputHandler::new(bounds, view_for_canvas.clone()),
                                        cx,
                                    );

                                    view_for_canvas.update(cx, |view, _| {
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

    let cursor = snap.cursor;

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

        for run in runs {
            let run_x = origin.x + cell_w * run.start_col as f32;
            let run_w = cell_w * run.count as f32;

            if run.bg != theme.background {
                let bg_quad = gpui::Bounds {
                    origin: gpui::point(run_x, y),
                    size: gpui::size(run_w, cell_h),
                };
                window.paint_quad(gpui::fill(bg_quad, run.bg));
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
