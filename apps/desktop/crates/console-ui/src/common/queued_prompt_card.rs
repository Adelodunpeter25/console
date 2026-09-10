//! Queued next-turn prompt card shown above the composer while a run is
//! active. Compact horizontal pill with inline Edit / Delete / Steer actions.

use std::rc::Rc;

use console_core::QueuedPrompt;
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement, Styled, Window,
    div, prelude::FluentBuilder, px,
};

use crate::common::centered_stripe;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub fn queued_prompt_card(
    prompt: QueuedPrompt,
    on_edit: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    on_delete: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    on_steer: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    theme: Theme,
) -> impl IntoElement {
    let has_attachments = prompt
        .attachments
        .as_ref()
        .is_some_and(|attachments| !attachments.is_empty());
    let text = prompt.prompt.clone();
    let edit_id = format!("queued-edit-{}", prompt.id);
    let delete_id = format!("queued-delete-{}", prompt.id);
    let steer_id = format!("queued-steer-{}", prompt.id);

    let edit_handler = on_edit.clone();
    let delete_handler = on_delete.clone();
    let steer_handler = on_steer.clone();

    centered_stripe(
        div()
            .w_full()
            .max_w(px(728.0))
            .px(px(12.0))
            .py(px(8.0))
            .rounded(px(8.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.border_strong)
            .shadow_sm()
            .flex()
            .flex_row()
            .items_center()
            .justify_between()
            .gap(px(10.0))
            // Left: Prompt text + optional attachment indicator
            .child(
                div()
                    .flex_1()
                    .min_w(px(0.0))
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child(
                        div()
                            .flex_1()
                            .min_w(px(0.0))
                            .text_size(px(13.0))
                            .line_height(px(18.0))
                            .text_color(theme.text)
                            .truncate()
                            .child(text),
                    )
                    .when(has_attachments, |el| {
                        el.child(
                            div()
                                .flex_shrink_0()
                                .child(app_icon(IconName::Paperclip, 11.0, theme.text_tertiary)),
                        )
                    }),
            )
            // Right: Inline Action Buttons (Edit, Delete, Steer/Send)
            .child(
                div()
                    .flex_shrink_0()
                    .flex()
                    .items_center()
                    .gap(px(2.0))
                    // Edit (Pencil)
                    .child(
                        div()
                            .id(edit_id)
                            .size(px(24.0))
                            .rounded(px(5.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .when_some(edit_handler, |element, handler| {
                                element
                                    .cursor_pointer()
                                    .hover(|style| style.bg(theme.overlay))
                                    .active(|style| style.bg(theme.overlay_strong))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .child(app_icon(IconName::Pencil, 12.0, theme.text_secondary)),
                    )
                    // Delete (Trash)
                    .child(
                        div()
                            .id(delete_id)
                            .size(px(24.0))
                            .rounded(px(5.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .when_some(delete_handler, |element, handler| {
                                element
                                    .cursor_pointer()
                                    .hover(|style| style.bg(theme.danger_soft))
                                    .active(|style| style.bg(theme.danger_soft))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .child(app_icon(IconName::TrashBinMinimalistic, 12.0, theme.text_secondary)),
                    )
                    // Steer / Send Now (Arrow Up)
                    .child(
                        div()
                            .id(steer_id)
                            .size(px(24.0))
                            .rounded(px(5.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .when_some(steer_handler, |element, handler| {
                                element
                                    .cursor_pointer()
                                    .hover(|style| style.bg(theme.overlay))
                                    .active(|style| style.bg(theme.overlay_strong))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .child(app_icon(IconName::ArrowUp, 12.0, theme.text_secondary)),
                    ),
            ),
        6.0,
    )
}
