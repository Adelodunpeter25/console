//! Shells tab: detached background shells for the active session. The list
//! shows each running shell with a stop button; opening one shows its status
//! and live output (polled by the app) in a selectable log view.

use std::rc::Rc;

use gpui::{
    AnyElement, App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use crate::viewer::LogsViewer;

/// One shell row in the list.
#[derive(Clone)]
pub struct ShellRow {
    pub id: String,
    pub command: String,
}

/// The opened shell: its command, a status line, and the output view.
pub struct ShellDetail {
    pub id: String,
    pub command: String,
    pub status: String,
    pub running: bool,
    pub truncated: bool,
    pub unavailable: bool,
    /// None until the first poll answers.
    pub output: Option<LogsViewer>,
}

#[derive(IntoElement)]
pub struct ShellListView {
    shells: Vec<ShellRow>,
    detail: Option<ShellDetail>,
    on_open: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_back: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl ShellListView {
    pub fn new(
        shells: Vec<ShellRow>,
        detail: Option<ShellDetail>,
        on_open: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_back: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            shells,
            detail,
            on_open,
            on_back,
            on_stop,
        }
    }
}

fn stop_button(
    id: String,
    on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    theme: Theme,
) -> impl IntoElement {
    div()
        .id(format!("stop-shell-{id}"))
        .size(px(24.0))
        .flex_none()
        .rounded_full()
        .flex()
        .items_center()
        .justify_center()
        .cursor_pointer()
        .bg(theme.danger.opacity(0.16))
        .hover(|s| s.bg(theme.danger.opacity(0.28)))
        .on_click(move |_, window, cx| {
            cx.stop_propagation();
            on_stop(id.clone(), window, cx);
        })
        .child(app_icon(IconName::Stop, 11.0, theme.danger))
}

impl RenderOnce for ShellListView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let Self {
            shells,
            detail,
            on_open,
            on_back,
            on_stop,
        } = self;

        if let Some(detail) = detail {
            return shell_detail(detail, on_back, on_stop, theme);
        }

        if shells.is_empty() {
            return div()
                .id("shell-list-empty")
                .size_full()
                .flex()
                .flex_col()
                .items_center()
                .justify_center()
                .py(px(48.0))
                .px(px(16.0))
                .gap(px(10.0))
                .child(app_icon(IconName::Terminal, 28.0, theme.text_ghost))
                .child(
                    div()
                        .text_size(px(13.0))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.text_secondary)
                        .child("No Background Shells"),
                )
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_tertiary)
                        .text_align(gpui::TextAlign::Center)
                        .child("Long-running commands the assistant starts in the background show up here"),
                )
                .into_any_element();
        }

        div()
            .id("shell-list-container")
            .size_full()
            .min_h_0()
            .overflow_y_scroll()
            .p(px(8.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(8.0))
                    .children(shells.into_iter().map(|shell| {
                        let open_id = shell.id.clone();
                        let on_open = on_open.clone();
                        div()
                            .id(format!("shell-row-{}", shell.id))
                            .w_full()
                            .flex()
                            .items_center()
                            .gap(px(8.0))
                            .p(px(8.0))
                            .rounded(px(6.0))
                            .bg(theme.surface)
                            .border_1()
                            .border_color(theme.accent.opacity(0.3))
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .on_click(move |_, window, cx| on_open(open_id.clone(), window, cx))
                            .child(app_icon(IconName::Terminal, 14.0, theme.accent))
                            .child(
                                div()
                                    .flex_1()
                                    .min_w_0()
                                    .flex()
                                    .flex_col()
                                    .gap(px(2.0))
                                    .child(
                                        div()
                                            .truncate()
                                            .text_size(px(12.0))
                                            .font_weight(FontWeight::SEMIBOLD)
                                            .text_color(theme.text)
                                            .child(shell.command.clone()),
                                    )
                                    .child(
                                        div()
                                            .text_size(px(11.0))
                                            .text_color(theme.text_tertiary)
                                            .child("running · click to view output"),
                                    ),
                            )
                            .child(stop_button(shell.id, on_stop.clone(), theme))
                    })),
            )
            .into_any_element()
    }
}

fn shell_detail(
    detail: ShellDetail,
    on_back: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    theme: Theme,
) -> AnyElement {
    let ShellDetail {
        id,
        command,
        status,
        running,
        truncated,
        unavailable,
        output,
    } = detail;

    let header = div()
        .flex_none()
        .flex()
        .items_center()
        .gap(px(6.0))
        .px(px(8.0))
        .py(px(6.0))
        .border_b_1()
        .border_color(theme.sidebar_border)
        .child(
            div()
                .id("shell-back")
                .flex()
                .items_center()
                .gap(px(4.0))
                .px(px(4.0))
                .py(px(3.0))
                .rounded(px(4.0))
                .cursor_pointer()
                .hover(|s| s.bg(theme.overlay))
                .on_click(move |_, window, cx| on_back(window, cx))
                .child(app_icon(IconName::ArrowLeft, 12.0, theme.text_secondary))
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_secondary)
                        .child("Shells"),
                ),
        )
        .child(div().flex_1())
        .when(running, |el| el.child(stop_button(id, on_stop, theme)));

    let info = div()
        .flex_none()
        .flex()
        .flex_col()
        .gap(px(4.0))
        .px(px(10.0))
        .py(px(8.0))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text)
                .child(command),
        )
        .child(
            div()
                .flex()
                .items_center()
                .gap(px(6.0))
                .child(div().size(px(7.0)).rounded_full().bg(if running {
                    theme.accent
                } else {
                    theme.text_ghost
                }))
                .child(
                    div()
                        .text_size(px(11.0))
                        .text_color(theme.text_tertiary)
                        .child(status),
                )
                .when(truncated, |el| {
                    el.child(
                        div()
                            .text_size(px(11.0))
                            .text_color(theme.danger)
                            .child("· output truncated"),
                    )
                }),
        );

    let body = match output {
        Some(view) => div().flex_1().min_h_0().child(view).into_any_element(),
        None => div()
            .flex_1()
            .px(px(10.0))
            .text_size(px(11.5))
            .text_color(if unavailable {
                theme.danger
            } else {
                theme.text_tertiary
            })
            .child(if unavailable {
                "Output unavailable."
            } else {
                "Waiting for output…"
            })
            .into_any_element(),
    };

    div()
        .id("shell-detail")
        .size_full()
        .min_h_0()
        .flex()
        .flex_col()
        .child(header)
        .child(info)
        .child(body)
        .into_any_element()
}
