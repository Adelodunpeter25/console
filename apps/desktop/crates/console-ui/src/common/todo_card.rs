//! The agent's live task checklist card, shown above the composer while a run
//! has outstanding todos.

use console_core::TodoItem;
use gpui::{FontWeight, IntoElement, ParentElement, Styled, div, px};

use crate::common::centered_stripe;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub fn todo_card(items: Vec<TodoItem>, theme: Theme) -> impl IntoElement {
    centered_stripe(
        div()
            .w_full()
            .max_w(px(768.0))
            .p(px(10.0))
            .rounded(px(8.0))
            .bg(theme.inset)
            .border_1()
            .border_color(theme.border)
            .flex()
            .flex_col()
            .gap(px(5.0))
            .child(
                div()
                    .text_size(px(11.5))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.text_secondary)
                    .child("TASKS"),
            )
            .children(items.into_iter().map(|item| {
                let completed = matches!(
                    item.status.to_ascii_lowercase().as_str(),
                    "done" | "completed" | "complete"
                );
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .text_size(px(12.0))
                    .text_color(if completed {
                        theme.text_ghost
                    } else {
                        theme.text
                    })
                    .child(app_icon(
                        if completed {
                            IconName::Check
                        } else {
                            IconName::Circle
                        },
                        11.0,
                        if completed {
                            theme.success
                        } else {
                            theme.text_ghost
                        },
                    ))
                    .child(item.content)
            })),
        6.0,
    )
}
