//! The agent's live task checklist card, shown above the composer while a run
//! has outstanding todos.

use std::rc::Rc;

use console_core::TodoItem;
use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::common::centered_stripe;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub fn todo_card(
    items: Vec<TodoItem>,
    collapsed: bool,
    on_toggle: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    theme: Theme,
) -> impl IntoElement {
    let total_count = items.len();
    let completed_count = items
        .iter()
        .filter(|item| {
            matches!(
                item.status.to_ascii_lowercase().as_str(),
                "done" | "completed" | "complete"
            )
        })
        .count();

    // Find the next active/pending item (first in_progress or first pending after done)
    let next_item = items
        .iter()
        .find(|item| {
            matches!(
                item.status.to_ascii_lowercase().as_str(),
                "in_progress" | "started" | "running"
            )
        })
        .or_else(|| {
            items.iter().find(|item| {
                !matches!(
                    item.status.to_ascii_lowercase().as_str(),
                    "done" | "completed" | "complete"
                )
            })
        });

    let all_done = completed_count == total_count && total_count > 0;

    centered_stripe(
        div()
            .w_full()
            .max_w(px(768.0))
            .p(px(9.0))
            .rounded(px(8.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.border_strong)
            .shadow_sm()
            .flex()
            .flex_col()
            .gap(px(5.0))
            // Header row with checklist icon, TASKS title, counter badge, collapsed preview, and chevron toggle
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_between()
                    .gap(px(8.0))
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(
                                IconName::Checklist,
                                13.0,
                                theme.text_secondary,
                            ))
                            .child(
                                div()
                                    .text_size(px(11.0))
                                    .font_weight(FontWeight::BOLD)
                                    .text_color(theme.text_secondary)
                                    .child("TASKS"),
                            )
                            .child(
                                div()
                                    .px(px(5.0))
                                    .py(px(1.0))
                                    .rounded(px(4.0))
                                    .bg(theme.overlay)
                                    .text_size(px(10.5))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(if all_done {
                                        theme.success
                                    } else {
                                        theme.text_tertiary
                                    })
                                    .child(format!("{completed_count}/{total_count}")),
                            ),
                    )
                    // In collapsed mode, show the next active task right in the middle
                    .when(collapsed, |el| {
                        let next_text = if let Some(item) = next_item {
                            item.content.clone()
                        } else if all_done {
                            "All tasks completed".to_string()
                        } else {
                            "No tasks".to_string()
                        };
                        el.child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .px(px(4.0))
                                .truncate()
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child(next_text),
                        )
                    })
                    // Toggle chevron button
                    .child(
                        div()
                            .id("btn-toggle-todos")
                            .size(px(20.0))
                            .rounded(px(4.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_default()
                            .hover(|style| style.bg(theme.overlay))
                            .when_some(on_toggle, |element, handler| {
                                element.on_click(move |_, window, cx| {
                                    (handler)(window, cx);
                                })
                            })
                            .child(app_icon(
                                if collapsed {
                                    IconName::ChevronDown
                                } else {
                                    IconName::ChevronUp
                                },
                                12.0,
                                theme.text_tertiary,
                            )),
                    ),
            )
            // Expanded task checklist
            .when(!collapsed, |el| {
                el.child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(4.0))
                        .pt(px(2.0))
                        .children(items.into_iter().map(|item| {
                            let completed = matches!(
                                item.status.to_ascii_lowercase().as_str(),
                                "done" | "completed" | "complete"
                            );
                            let in_progress = matches!(
                                item.status.to_ascii_lowercase().as_str(),
                                "in_progress" | "started" | "running"
                            );

                            div()
                                .flex()
                                .items_center()
                                .gap(px(8.0))
                                .py(px(2.0))
                                .text_size(px(12.0))
                                .text_color(if completed {
                                    theme.text_tertiary
                                } else {
                                    theme.text
                                })
                                // Checkbox square
                                .child(
                                    div()
                                        .size(px(15.0))
                                        .rounded(px(3.5))
                                        .flex_none()
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .border_1()
                                        .when(completed, |box_el| {
                                            box_el
                                                .bg(theme.success.opacity(0.16))
                                                .border_color(theme.success)
                                                .child(app_icon(
                                                    IconName::Check,
                                                    9.5,
                                                    theme.success,
                                                ))
                                        })
                                        .when(in_progress, |box_el| {
                                            box_el
                                                .bg(theme.accent.opacity(0.12))
                                                .border_color(theme.accent)
                                                .child(
                                                    div()
                                                        .size(px(5.0))
                                                        .rounded_full()
                                                        .bg(theme.accent),
                                                )
                                        })
                                        .when(!completed && !in_progress, |box_el| {
                                            box_el
                                                .bg(theme.composer)
                                                .border_color(theme.border_strong)
                                        }),
                                )
                                .child(
                                    div()
                                        .flex_1()
                                        .min_w_0()
                                        .when(completed, |text_el| {
                                            text_el.line_through()
                                        })
                                        .child(item.content),
                                )
                        })),
                )
            }),
        6.0,
    )
}
