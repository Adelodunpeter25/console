//! The floating row of status pills just above the composer: changed files,
//! active tasks, running subagents, and background shells. The row is an
//! overlay on the transcript's bottom edge, so chat scrolls behind it the way
//! it does on mobile instead of the pills claiming their own band. Each pill
//! is a compact left-aligned summary that opens the surface holding the full
//! list; multiple kinds sit side by side, never stacked.

use std::rc::Rc;

use console_core::TodoItem;
use gpui::{
    AnyElement, App, Div, ElementId, FontWeight, ParentElement, Stateful,
    StatefulInteractiveElement, Styled, Window, div, prelude::*, px,
};

use crate::chat::change_chips::ChangesSummary;
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, popover};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ClickHandler = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// The active task list plus the menu handle that anchors its popover to the
/// leading pill.
pub struct TasksPill {
    pub completed: usize,
    pub total: usize,
    pub menu: ContextMenuHandle,
    pub items: Rc<Vec<TodoItem>>,
}

/// The pills row, or `None` when the session has nothing to summarise.
///
/// `subagents` is `(count, any running)`; `shells` is the live count; `changes`
/// is the session's change totals plus the handler that opens the changes view.
#[allow(clippy::type_complexity)]
pub fn composer_pills(
    key: &str,
    tasks: Option<TasksPill>,
    subagents: Option<(usize, bool)>,
    shells: usize,
    on_subagents: Option<ClickHandler>,
    on_shells: Option<ClickHandler>,
    changes: Option<(ChangesSummary, ClickHandler)>,
    theme: Theme,
) -> Option<Div> {
    if tasks.is_none() && subagents.is_none() && shells == 0 && changes.is_none() {
        return None;
    }

    let mut row = div()
        .w_full()
        .max_w(px(768.0))
        .flex()
        .flex_row()
        .items_center()
        .gap(px(8.0));

    if let Some((summary, on_changes)) = changes {
        row = row.child(changes_pill(
            format!("{key}-changes"),
            summary,
            Some(on_changes),
            theme,
        ));
    }

    if let Some(tasks) = tasks {
        let trigger = pill(
            format!("{key}-tasks"),
            IconName::Checklist,
            format!("{}/{} tasks", tasks.completed, tasks.total),
            false,
            None,
            theme,
        );
        let items = tasks.items;
        row = row.child(popover(
            trigger,
            &tasks.menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, cx| tasks_popover(items.clone(), Theme::current(cx)),
        ));
    }
    if let Some((count, running)) = subagents {
        row = row.child(pill(
            format!("{key}-subagents"),
            IconName::Bot,
            format!(
                "{count} {}",
                if count == 1 { "subagent" } else { "subagents" }
            ),
            running,
            on_subagents,
            theme,
        ));
    }
    if shells > 0 {
        row = row.child(pill(
            format!("{key}-shells"),
            IconName::Terminal,
            format!("{shells} {}", if shells == 1 { "shell" } else { "shells" }),
            true,
            on_shells,
            theme,
        ));
    }

    Some(
        div()
            .absolute()
            .left_0()
            .right_0()
            .bottom(px(8.0))
            .w_full()
            .px(px(20.0))
            .flex()
            .justify_center()
            .child(row),
    )
}

/// The changed-files pill: icon, "N files changed", and the colored `+N -N`
/// totals. Clicking opens the changes review for the session.
fn changes_pill(
    id: String,
    summary: ChangesSummary,
    on_click: Option<ClickHandler>,
    theme: Theme,
) -> Stateful<Div> {
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .flex()
        .items_center()
        .gap(px(6.0))
        .px(px(10.0))
        .py(px(5.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        .when_some(on_click, |element, handler| {
            element.on_click(move |_, window, cx| (handler)(window, cx))
        })
        .child(app_icon(IconName::FileDiff, 13.0, theme.text_tertiary))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(format!(
                    "{} file{} changed",
                    summary.files,
                    if summary.files == 1 { "" } else { "s" }
                )),
        )
        .when(summary.additions > 0, |element| {
            element.child(
                div()
                    .text_size(px(12.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.success)
                    .child(format!("+{}", summary.additions)),
            )
        })
        .when(summary.deletions > 0, |element| {
            element.child(
                div()
                    .text_size(px(12.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.danger)
                    .child(format!("-{}", summary.deletions)),
            )
        })
}

fn pill(
    id: String,
    icon: IconName,
    text: String,
    running: bool,
    on_click: Option<ClickHandler>,
    theme: Theme,
) -> Stateful<Div> {
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .flex()
        .items_center()
        .gap(px(6.0))
        .px(px(10.0))
        .py(px(5.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        .hover(|style| style.bg(theme.overlay_strong))
        .when_some(on_click, |element, handler| {
            element.on_click(move |_, window, cx| (handler)(window, cx))
        })
        .child(app_icon(
            icon,
            13.0,
            if running {
                theme.accent
            } else {
                theme.text_tertiary
            },
        ))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(text),
        )
}

/// The floating task list that opens above the tasks pill: a compact card
/// with one truncated title per row and a status marker, capped so a long
/// checklist scrolls instead of covering the chat.
fn tasks_popover(items: Rc<Vec<TodoItem>>, theme: Theme) -> AnyElement {
    let total = items.len();
    let completed = items
        .iter()
        .filter(|item| {
            matches!(
                item.status.to_ascii_lowercase().as_str(),
                "done" | "completed" | "complete"
            )
        })
        .count();

    div()
        .id("composer-tasks-popover")
        .w(px(300.0))
        .max_h(px(340.0))
        .rounded(px(12.0))
        .bg(theme.raised)
        .border_1()
        .border_color(theme.border_strong)
        .shadow_xl()
        .overflow_hidden()
        .flex()
        .flex_col()
        .child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .px(px(11.0))
                .py(px(8.0))
                .border_b_1()
                .border_color(theme.border)
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .child(app_icon(IconName::Checklist, 12.0, theme.text_secondary))
                        .child(
                            div()
                                .text_size(px(10.5))
                                .font_weight(FontWeight::BOLD)
                                .text_color(theme.text_secondary)
                                .child("TASKS"),
                        ),
                )
                .child(
                    div()
                        .text_size(px(10.5))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.text_tertiary)
                        .child(format!("{completed}/{total}")),
                ),
        )
        .child(
            div()
                .id("composer-tasks-popover-scroll")
                .max_h(px(300.0))
                .overflow_y_scroll()
                .on_scroll_wheel(|_, _, cx| cx.stop_propagation())
                .p(px(5.0))
                .flex()
                .flex_col()
                .gap(px(1.0))
                .children(items.iter().enumerate().map(|(index, item)| {
                    let status = item.status.to_ascii_lowercase();
                    let done = matches!(status.as_str(), "done" | "completed" | "complete");
                    let in_progress =
                        matches!(status.as_str(), "in_progress" | "started" | "running");

                    div()
                        .id(format!("composer-task-row-{index}"))
                        .w_full()
                        .min_w_0()
                        .flex()
                        .items_center()
                        .gap(px(8.0))
                        .rounded(px(6.0))
                        .px(px(6.0))
                        .py(px(6.0))
                        .hover(|style| style.bg(theme.overlay))
                        .child(
                            div()
                                .size(px(15.0))
                                .flex_none()
                                .rounded_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .border_1()
                                .when(done, |marker| {
                                    marker
                                        .bg(theme.success.opacity(0.16))
                                        .border_color(theme.success)
                                        .child(app_icon(IconName::Check, 9.0, theme.success))
                                })
                                .when(in_progress, |marker| {
                                    marker
                                        .bg(theme.accent.opacity(0.12))
                                        .border_color(theme.accent)
                                        .child(div().size(px(5.0)).rounded_full().bg(theme.accent))
                                })
                                .when(!done && !in_progress, |marker| {
                                    marker.border_color(theme.border_strong)
                                }),
                        )
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .truncate()
                                .text_size(px(12.5))
                                .text_color(if done {
                                    theme.text_tertiary
                                } else {
                                    theme.text
                                })
                                .child(item.content.clone()),
                        )
                })),
        )
        .into_any_element()
}
