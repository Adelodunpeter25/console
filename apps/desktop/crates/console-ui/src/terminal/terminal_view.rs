use console_core::types::terminal::{TerminalSize, TerminalSpawnParams, TerminalStatus};
use console_core::ConsoleClient;
use std::sync::Arc;
use gpui::{
    App, Context, FocusHandle, Focusable, IntoElement, KeyDownEvent, ParentElement, Render, Styled,
    Window, div, prelude::*, px, rgb, SharedString,
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
                    "[" => Some("\x1b".into()),
                    "\\" => Some("\x1c".into()),
                    "]" => Some("\x1d".into()),
                    "^" => Some("\x1e".into()),
                    "_" => Some("\x1f".into()),
                    _ => None,
                };
            }
        }

        if mods.alt && key.len() == 1 {
            return Some(format!("\x1b{key}"));
        }

        match key {
            "enter" => Some("\r".into()),
            "space" => Some(" ".into()),
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
            _ if key.len() == 1 => Some(key.to_string()),
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

        let handle_for_key = self.handle.clone();
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
                    .p(px(8.0))
                    .when_some(snapshot, |el, snap| {
                        el.child(render_snapshot(&snap, ttheme))
                    })
                    .when(self.snapshot.is_none(), |el| {
                        el.child(
                            div()
                                .size_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .text_color(theme.text_ghost)
                                .text_size(px(12.0))
                                .child(SharedString::from("No output yet")),
                        )
                    }),
            )
    }
}

fn render_snapshot(
    snapshot: &console_core::types::terminal::TerminalGridSnapshot,
    theme: TerminalTheme,
) -> gpui::AnyElement {
    let to_rgb_u32 = |c: Option<console_core::types::terminal::TerminalColor>| {
        c.map(|c| c.r as u32 * 256 * 256 + c.g as u32 * 256 + c.b as u32)
    };
    let cursor = snapshot.cursor;

    div()
        .flex()
        .flex_col()
        .font_family("GeistMono")
        .text_size(px(13.0))
        .line_height(px(17.0))
        .children(snapshot.rows.iter().enumerate().map(|(row_idx, row)| {
            // Group consecutive same-styled cells into one text run: an
            // 80x24 grid renders ~dozens of elements instead of ~2k divs,
            // which is what made typed input echo visibly lag behind.
            let mut runs: Vec<(Option<u32>, Option<u32>, bool, String)> = Vec::new();
            for (col_idx, cell) in row.iter().enumerate() {
                let fg = to_rgb_u32(cell.fg);
                let bg = to_rgb_u32(cell.bg);
                let is_cursor =
                    row_idx as u16 == cursor.row && col_idx as u16 == cursor.col && cursor.visible;
                match runs.last_mut() {
                    Some((run_fg, run_bg, run_cursor, text))
                        if *run_fg == fg && *run_bg == bg && *run_cursor == is_cursor =>
                    {
                        text.push(cell.c);
                    }
                    _ => runs.push((fg, bg, is_cursor, cell.c.to_string())),
                }
            }

            // Runs flow at the font's natural advance, so the cursor block
            // always sits exactly where the glyphs end — no cell-grid math.
            div()
                .h(px(16.0))
                .flex()
                .flex_row()
                .children(runs.into_iter().map(|(fg, bg, is_cursor, text)| {
                    let (fg, bg) = if is_cursor {
                        (theme.cursor_text, theme.cursor)
                    } else {
                        (
                            fg.map(|v| rgb(v).into()).unwrap_or(theme.foreground),
                            bg.map(|v| rgb(v).into()).unwrap_or(theme.background),
                        )
                    };
                    div()
                        .h(px(16.0))
                        .flex()
                        .items_center()
                        .bg(bg)
                        .text_color(fg)
                        .child(SharedString::from(text))
                }))
        }))
        .into_any_element()
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
