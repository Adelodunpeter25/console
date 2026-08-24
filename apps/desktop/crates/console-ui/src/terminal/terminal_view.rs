use console_core::types::terminal::{TerminalBackend, TerminalGridSnapshot, TerminalSize, TerminalStatus};
use gpui::{
    App, Context, FocusHandle, Focusable, IntoElement, ParentElement, Render, Styled, Window, div,
    prelude::*, px, rgb, SharedString,
};

use crate::theme::Theme;
use super::theme::TerminalTheme;

/// Isolated terminal surface — renders a `TerminalGridSnapshot` as monospaced text.
/// **Not wired into `ConsoleDesktopApp` yet**; the parent is expected to own a
/// `Box<dyn TerminalBackend>` (e.g. `AlacrittyBackend`) and call `advance` on
/// server output, then `snapshot` → `set_snapshot`.
pub struct TerminalView {
    snapshot: Option<TerminalGridSnapshot>,
    focus: FocusHandle,
    status: TerminalStatus,
    error: Option<String>,
    on_input: Option<std::rc::Rc<dyn Fn(String, &mut Window, &mut App)>>,
}

impl TerminalView {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let focus = cx.focus_handle();
        // Do not auto-focus on creation — parent will focus when the tab is activated.
        let _ = window;
        Self {
            snapshot: None,
            focus,
            status: TerminalStatus::Spawning,
            error: None,
            on_input: None,
        }
    }

    pub fn set_snapshot(&mut self, snapshot: TerminalGridSnapshot, cx: &mut Context<Self>) {
        self.snapshot = Some(snapshot);
        cx.notify();
    }

    pub fn set_status(&mut self, status: TerminalStatus, cx: &mut Context<Self>) {
        self.status = status;
        cx.notify();
    }

    pub fn set_error(&mut self, error: Option<String>, cx: &mut Context<Self>) {
        self.error = error;
        cx.notify();
    }

    pub fn on_input<F>(&mut self, f: F)
    where
        F: Fn(String, &mut Window, &mut App) + 'static,
    {
        self.on_input = Some(std::rc::Rc::new(f));
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
        let ttheme = TerminalTheme::from_app_theme(&theme);

        let status_banner = match self.status {
            TerminalStatus::Spawning => Some(("Spawning shell…", theme.text_ghost)),
            TerminalStatus::Error => self.error.as_deref().map(|e| (e as &str, theme.warning)),
            TerminalStatus::Exited => Some(("Shell exited — press Trash to respawn", theme.text_tertiary)),
            TerminalStatus::Running => None,
        };

        let snapshot = self.snapshot.clone();

        div()
            .size_full()
            .flex()
            .flex_col()
            .bg(ttheme.background)
            .text_color(ttheme.foreground)
            .overflow_hidden()
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

fn render_snapshot(snapshot: &TerminalGridSnapshot, theme: TerminalTheme) -> gpui::AnyElement {
    let rows = snapshot.rows.clone();
    let cursor = snapshot.cursor;
    div()
        .flex()
        .flex_col()
        .font_family("GeistMono")
        .text_size(px(12.0))
        .line_height(px(16.0))
        .children(rows.into_iter().enumerate().map(|(row_idx, row)| {
            let is_cursor_row = row_idx as u16 == cursor.row;
            div()
                .flex()
                .flex_row()
                .children(row.into_iter().enumerate().map(move |(col_idx, cell)| {
                    let is_cursor = is_cursor_row && col_idx as u16 == cursor.col && cursor.visible;
                    let fg = cell
                        .fg
                        .map(|c| rgb(c.r as u32 * 256 * 256 + c.g as u32 * 256 + c.b as u32).into())
                        .unwrap_or(theme.foreground);
                    let bg = cell
                        .bg
                        .map(|c| rgb(c.r as u32 * 256 * 256 + c.g as u32 * 256 + c.b as u32).into())
                        .unwrap_or(theme.background);
                    let ch = if cell.c == ' ' { ' ' } else { cell.c };
                    div()
                        .min_w(px(7.2))
                        .h(px(16.0))
                        .flex()
                        .items_center()
                        .justify_center()
                        .bg(if is_cursor { theme.cursor } else { bg })
                        .text_color(if is_cursor { theme.cursor_text } else { fg })
                        .child(SharedString::from(ch.to_string()))
                }))
        }))
        .into_any_element()
}

/// Convenience helper — estimate a `TerminalSize` from pixel bounds and a font size.
pub fn estimate_size(width: f32, height: f32, font_size: f32) -> TerminalSize {
    let cols = (width / (font_size * 0.62)).floor() as u16;
    let rows = (height / (font_size * 1.35)).floor() as u16;
    TerminalSize {
        cols: cols.max(20),
        rows: rows.max(5),
    }
}

/// Snapshot-driven helper — feed `data` into a backend and return the updated snapshot.
pub fn advance_backend<B: TerminalBackend>(backend: &mut B, data: &str) -> TerminalGridSnapshot {
    backend.advance(data);
    backend.snapshot()
}
