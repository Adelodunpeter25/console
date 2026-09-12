//! Run tab panel: project script list with expandable output rows.
//!
//! Pure `RenderOnce` component following the settings-page pattern — all
//! state lives in the desktop app, this module only renders it. The pure
//! helpers (output cap, shortcut display, status labels) live here so they
//! can be unit-tested from `tests/`.

use std::rc::Rc;

use console_core::ScriptRunStatus;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

/// Retained output cap per run, in bytes. The server keeps full logs; the
/// desktop renders a tail so a chatty dev server can't grow memory.
pub const MAX_RUN_OUTPUT_BYTES: usize = 200_000;

/// Drop the head of an overgrown buffer, keeping valid UTF-8 boundaries.
pub fn truncate_run_output_head(output: &mut String) {
    if output.len() > MAX_RUN_OUTPUT_BYTES {
        let mut cut = output.len() - MAX_RUN_OUTPUT_BYTES;
        while !output.is_char_boundary(cut) {
            cut += 1;
        }
        output.drain(..cut);
    }
}

/// Append live stream text, keeping the tail.
pub fn push_run_output(output: &mut String, text: &str) {
    output.push_str(text);
    truncate_run_output_head(output);
}

/// Cap retained output, keeping the tail.
pub fn cap_run_output(output: String) -> String {
    let mut output = output;
    truncate_run_output_head(&mut output);
    output
}

/// Format a `console.toml` shortcut (`cmd-r`, `cmd-shift-b`) for display
/// (`⌘R`, `⇧⌘B`). Display only — no binding happens in this slice.
pub fn display_script_shortcut(shortcut: &str) -> String {
    let mut parts: Vec<&str> = shortcut.split('-').collect();
    let key = parts.pop().unwrap_or("").to_uppercase();
    let key = if key.trim().is_empty() {
        "Space".to_string()
    } else {
        key
    };
    let mut prefix = String::new();
    for modifier in ["ctrl", "alt", "opt", "shift", "cmd"] {
        if parts.iter().any(|part| part.eq_ignore_ascii_case(modifier)) {
            prefix.push_str(match modifier {
                "ctrl" => "⌃",
                "alt" | "opt" => "⌥",
                "shift" => "⇧",
                _ => "⌘",
            });
        }
    }
    format!("{prefix}{key}")
}

/// One-line status summary for a script row.
pub fn script_row_status_label(status: Option<ScriptRunStatus>, starting: bool) -> &'static str {
    if starting {
        return "Starting…";
    }
    match status {
        None => "Not run yet",
        Some(ScriptRunStatus::Running) => "Running…",
        Some(ScriptRunStatus::Succeeded) => "Succeeded",
        Some(ScriptRunStatus::Failed) => "Failed",
        Some(ScriptRunStatus::Stopped) => "Stopped",
    }
}

fn status_dot_color(status: Option<ScriptRunStatus>, starting: bool, theme: &Theme) -> gpui::Hsla {
    if starting {
        return theme.success;
    }
    match status {
        None => theme.text_ghost,
        Some(ScriptRunStatus::Running) => theme.success,
        Some(ScriptRunStatus::Succeeded) => theme.text_tertiary,
        Some(ScriptRunStatus::Failed) => theme.danger,
        Some(ScriptRunStatus::Stopped) => theme.warning,
    }
}

/// One script row's render model, mapped from app state by the caller.
#[derive(Clone)]
pub struct RunScriptRow {
    pub script_id: String,
    pub label: String,
    pub command: String,
    pub shortcut: Option<String>,
    pub status: Option<ScriptRunStatus>,
    pub exit_code: Option<i32>,
    pub starting: bool,
    pub output: String,
    pub expanded: bool,
    /// `Some(shortcut)` when another script in the same project claims the
    /// same keystroke — both bindings are blocked at the app layer and the
    /// conflict surfaces here as a warning.
    pub shortcut_conflict: Option<String>,
}

#[derive(IntoElement)]
pub struct RunPanel {
    pub has_project: bool,
    pub loading: bool,
    pub error: Option<String>,
    pub source_missing: bool,
    pub rows: Vec<RunScriptRow>,
    pub on_run: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_toggle_expand: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

fn empty_state(icon: IconName, title: &str, hint: &str, theme: &Theme) -> impl IntoElement {
    div()
        .size_full()
        .flex()
        .flex_col()
        .items_center()
        .justify_center()
        .gap(px(8.0))
        .p(px(16.0))
        .child(app_icon(icon, 28.0, theme.text_ghost))
        .child(
            div()
                .text_size(px(13.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(title.to_string()),
        )
        .child(
            div()
                .text_size(px(11.5))
                .text_color(theme.text_tertiary)
                .child(hint.to_string()),
        )
}

impl RenderOnce for RunPanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let mut content = div().size_full().flex().flex_col().min_h_0();

        if let Some(error) = self.error {
            content = content.child(
                div()
                    .flex_none()
                    .m(px(8.0))
                    .mb(px(0.0))
                    .px(px(10.0))
                    .py(px(8.0))
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(theme.danger)
                    .bg(theme.danger_soft)
                    .text_size(px(11.5))
                    .text_color(theme.danger)
                    .child(error),
            );
        }

        if !self.has_project {
            return content
                .child(empty_state(
                    IconName::Play,
                    "No project selected",
                    "Select a project to see its run scripts.",
                    &theme,
                ))
                .into_any_element();
        }
        if self.loading && self.rows.is_empty() {
            return content
                .child(empty_state(
                    IconName::Play,
                    "Loading scripts…",
                    "Reading console.toml from the project root.",
                    &theme,
                ))
                .into_any_element();
        }
        if self.rows.is_empty() {
            let (title, hint) = if self.source_missing {
                (
                    "No console.toml",
                    "Define [scripts] in console.toml at the project root to run them here.",
                )
            } else {
                (
                    "No scripts defined",
                    "Add a [scripts.*] table with a label and command.",
                )
            };
            return content
                .child(empty_state(IconName::Play, title, hint, &theme))
                .into_any_element();
        }

        let rows = self.rows.into_iter().map(|row| {
            let script_id = row.script_id.clone();
            let expand_id = row.script_id.clone();
            let toggle_id = row.script_id.clone();
            let is_running = row.status == Some(ScriptRunStatus::Running);
            let dot = status_dot_color(row.status, row.starting, &theme);
            let mut status_text = script_row_status_label(row.status, row.starting).to_string();
            if row.status.is_some_and(|status| status.is_terminal())
                && let Some(code) = row.exit_code
            {
                status_text = format!("{status_text} ({code})");
            }
            let on_run = self.on_run.clone();
            let on_stop = self.on_stop.clone();
            let on_toggle = self.on_toggle_expand.clone();

            let mut card = div()
                .id(ElementId::from(format!("run-row-{script_id}")))
                .flex_none()
                .flex()
                .flex_col()
                .rounded(px(6.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.raised)
                .child(
                    div()
                        .h(px(34.0))
                        .flex_none()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .pl(px(4.0))
                        .pr(px(8.0))
                        .child(
                            div()
                                .id(ElementId::from(format!("run-expand-{expand_id}")))
                                .size(px(22.0))
                                .flex_none()
                                .rounded(px(4.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay))
                                .on_click(move |_, window, cx| {
                                    cx.stop_propagation();
                                    (on_toggle)(expand_id.clone(), window, cx);
                                })
                                .child(app_icon(
                                    if row.expanded {
                                        IconName::ChevronDown
                                    } else {
                                        IconName::ChevronRight
                                    },
                                    12.0,
                                    theme.text_tertiary,
                                )),
                        )
                        .child(div().size(px(7.0)).flex_none().rounded_full().bg(dot))
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .text_size(px(12.5))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.text)
                                .truncate()
                                .child(row.label.clone()),
                        )
                        .child(
                            div()
                                .flex_none()
                                .text_size(px(10.5))
                                .text_color(theme.text_tertiary)
                                .child(status_text),
                        )
                        .when_some(row.shortcut.clone(), |el, shortcut| {
                            el.child(
                                div()
                                    .flex_none()
                                    .px(px(5.0))
                                    .py(px(1.0))
                                    .rounded(px(4.0))
                                    .border_1()
                                    .border_color(theme.border)
                                    .bg(theme.surface)
                                    .text_size(px(10.5))
                                    .font_family(MONO_FAMILY)
                                    .text_color(theme.text_secondary)
                                    .child(display_script_shortcut(&shortcut)),
                            )
                        })
                        .when_some(row.shortcut_conflict.clone(), |el, _shortcut| {
                            el.child(
                                div()
                                    .flex_none()
                                    .px(px(5.0))
                                    .py(px(1.0))
                                    .rounded(px(4.0))
                                    .border_1()
                                    .border_color(theme.danger)
                                    .text_size(px(10.0))
                                    .text_color(theme.danger)
                                    .child("⚠ shortcut conflict"),
                            )
                        })
                        .child(
                            div()
                                .id(ElementId::from(format!("run-toggle-{toggle_id}")))
                                .size(px(24.0))
                                .flex_none()
                                .rounded(px(5.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay))
                                .on_click(move |_, window, cx| {
                                    cx.stop_propagation();
                                    if is_running || row.starting {
                                        (on_stop)(toggle_id.clone(), window, cx);
                                    } else {
                                        (on_run)(toggle_id.clone(), window, cx);
                                    }
                                })
                                .child(app_icon(
                                    if is_running || row.starting {
                                        IconName::StopFilled
                                    } else {
                                        IconName::Play
                                    },
                                    12.0,
                                    if is_running || row.starting {
                                        theme.danger
                                    } else {
                                        theme.text
                                    },
                                )),
                        ),
                );

            if row.expanded {
                let body = if row.output.is_empty() {
                    if row.status.is_none() && !row.starting {
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(4.0))
                            .child(
                                div()
                                    .text_size(px(11.0))
                                    .font_family(MONO_FAMILY)
                                    .text_color(theme.text_tertiary)
                                    .child(row.command.clone()),
                            )
                            .child(
                                div()
                                    .text_size(px(11.0))
                                    .text_color(theme.text_ghost)
                                    .child("Not run yet — press play."),
                            )
                            .into_any_element()
                    } else {
                        div()
                            .text_size(px(11.0))
                            .text_color(theme.text_ghost)
                            .child("(no output yet)")
                            .into_any_element()
                    }
                } else {
                    div()
                        .text_size(px(11.0))
                        .font_family(MONO_FAMILY)
                        .text_color(theme.text_secondary)
                        .child(row.output.clone())
                        .into_any_element()
                };
                card = card.child(
                    div()
                        .id(ElementId::from(format!("run-output-{script_id}")))
                        .mx(px(8.0))
                        .mb(px(8.0))
                        .p(px(8.0))
                        .rounded(px(6.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.inset)
                        .max_h(px(220.0))
                        .overflow_y_scroll()
                        // Own the wheel while hovering output: without this
                        // the event bubbles and the script list scrolls too.
                        .on_scroll_wheel(|_, _, cx| cx.stop_propagation())
                        .child(body),
                );
            }
            card
        });

        content
            .child(
                div()
                    .id("run-scripts-scroll")
                    .flex_1()
                    .min_h_0()
                    .w_full()
                    .overflow_y_scroll()
                    .p(px(8.0))
                    .flex()
                    .flex_col()
                    .gap(px(6.0))
                    .children(rows)
                    // Trailing spacer: guarantees max scroll reveals the
                    // last card fully with breathing room to spare.
                    .child(div().h(px(16.0)).flex_none()),
            )
            .into_any_element()
    }
}
