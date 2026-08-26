use console_core::types::terminal::{TerminalSize, TerminalSpawnParams, TerminalStatus};
use console_core::ConsoleClient;
use std::sync::Arc;
use gpui::{
    App, Context, FocusHandle, Focusable, IntoElement, KeyDownEvent, ParentElement, Render, Styled,
    Window, div, prelude::*, px, SharedString,
};

use crate::theme::Theme;
use super::theme::TerminalTheme;

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
        };

        this.spawn(params, client, cx);
        this
    }

    pub fn with_cwd(cwd: impl Into<String>, client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let params = TerminalSpawnParams {
            cwd: cwd.into(),
            ..Default::default()
        };
        Self::new(params, client, window, cx)
    }

    fn spawn(&mut self, params: TerminalSpawnParams, client: ConsoleClient, cx: &mut Context<Self>) {
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
            let initial_snapshot = handle.snapshot().await;
            let initial_status = handle.status().await;
            let initial_error = handle.error.read().await.clone();

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
                    let snapshot = handle_for_watch.snapshot().await;
                    let status = handle_for_watch.status().await;
                    let error = handle_for_watch.error.read().await.clone();
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

    fn key_to_bytes(event: &KeyDownEvent) -> Option<String> {
        let key = event.keystroke.key.as_str();
        let mods = event.keystroke.modifiers;

        if mods.control {
            if let Some(ch) = key.chars().next() {
                let lower = ch.to_ascii_lowercase();
                if ('a'..='z').contains(&lower) {
                    let code = (lower as u8 - b'a' + 1) as char;
                    return Some(code.to_string());
                }
                return match key {
                    "space" => Some("\x00".into()),
                    "[" => Some("\x1b".into()),
                    "\\" => Some("\x1c".into()),
                    "]" => Some("\x1d".into()),
                    "^" => Some("\x1e".into()),
                    "_" => Some("\x1f".into()),
                    "backspace" => Some("\x17".into()),
                    "delete" => Some("\x1b[3;5~".into()),
                    "up" => Some("\x1b[1;5A".into()),
                    "down" => Some("\x1b[1;5B".into()),
                    "right" => Some("\x1b[1;5C".into()),
                    "left" => Some("\x1b[1;5D".into()),
                    _ => None,
                };
            }
        }

        if mods.alt {
            match key {
                "backspace" => return Some("\x1b\x7f".into()),
                "up" => return Some("\x1b[1;3A".into()),
                "down" => return Some("\x1b[1;3B".into()),
                "right" => return Some("\x1b[1;3C".into()),
                "left" => return Some("\x1b[1;3D".into()),
                _ if key.len() == 1 => return Some(format!("\x1b{key}")),
                _ => {}
            }
        }

        if mods.shift {
            match key {
                "up" => return Some("\x1b[1;2A".into()),
                "down" => return Some("\x1b[1;2B".into()),
                "right" => return Some("\x1b[1;2C".into()),
                "left" => return Some("\x1b[1;2D".into()),
                "tab" => return Some("\x1b[Z".into()),
                _ => {}
            }
        }

        match key {
            "enter" => Some("\r".into()),
            "space" => Some(" ".into()),
            "backspace" => Some("\x7f".into()),
            "delete" => Some("\x1b[3~".into()),
            "tab" => Some("\t".into()),
            "escape" => Some("\x1b".into()),
            "up" => Some("\x1b[A".into()),
            "down" => Some("\x1b[B".into()),
            "right" => Some("\x1b[C".into()),
            "left" => Some("\x1b[D".into()),
            "home" => Some("\x1b[H".into()),
            "end" => Some("\x1b[F".into()),
            "pageup" => Some("\x1b[5~".into()),
            "pagedown" => Some("\x1b[6~".into()),
            "f1" => Some("\x1bOP".into()),
            "f2" => Some("\x1bOQ".into()),
            "f3" => Some("\x1bOR".into()),
            "f4" => Some("\x1bOS".into()),
            "f5" => Some("\x1b[15~".into()),
            "f6" => Some("\x1b[17~".into()),
            "f7" => Some("\x1b[18~".into()),
            "f8" => Some("\x1b[19~".into()),
            "f9" => Some("\x1b[20~".into()),
            "f10" => Some("\x1b[21~".into()),
            "f11" => Some("\x1b[23~".into()),
            "f12" => Some("\x1b[24~".into()),
            _ if key.chars().count() == 1 => Some(key.to_string()),
            _ => None,
        }
    }
}

impl Focusable for TerminalView {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus.clone()
    }
}

impl Render for TerminalView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let ttheme = self.theme(cx);

        let status_banner = match self.status {
            TerminalStatus::Spawning => Some(("Spawning shell…", theme.text_ghost)),
            TerminalStatus::Error => self.error.as_deref().map(|e| (e as &str, theme.warning)),
            TerminalStatus::Exited => Some(("Shell exited — close or respawn", theme.text_tertiary)),
            TerminalStatus::Running => None,
        };

        let snapshot = self.snapshot.clone();
        let view_handle = cx.entity().clone();
        let handle_for_key = self.handle.clone();
        let handle_for_scroll = self.handle.clone();
        let focus_for_key = self.focus.clone();

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
            .on_key_down(move |event: &KeyDownEvent, window: &mut Window, cx: &mut App| {
                if !focus_for_key.is_focused(window) {
                    window.focus(&focus_for_key, cx);
                }
                if let Some(bytes) = TerminalView::key_to_bytes(event) {
                    if let Some(h) = &handle_for_key {
                        h.send_input(bytes);
                    }
                    cx.stop_propagation();
                }
            })
            .on_scroll_wheel(move |event, _window, cx| {
                if let Some(h) = &handle_for_scroll {
                    let delta = match event.delta {
                        gpui::ScrollDelta::Lines(lines) => lines.y.round() as i32,
                        gpui::ScrollDelta::Pixels(pixels) => (f32::from(pixels.y) / 16.0).round() as i32,
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
                                move |bounds, (cols, rows, cell_w, cell_h), window, cx| {
                                    view_handle.update(cx, |view, _| {
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

            let can_merge = if let Some(last) = runs.last() {
                last.fg == fg
                    && last.bg == bg
                    && last.bold == cell.flags.bold
                    && last.italic == cell.flags.italic
                    && last.underline == cell.flags.underline
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
                    underline: cell.flags.underline,
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

pub fn advance_backend<B: console_core::types::terminal::TerminalBackend>(backend: &mut B, data: &str) -> console_core::types::terminal::TerminalGridSnapshot {
    backend.advance(data);
    backend.snapshot()
}
