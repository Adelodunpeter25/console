//! Queued next-turn prompt card shown above the composer while a run is
//! active. Compact single-row strip with truncate, Edit / Delete / Steer.

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

    let edit_handler = on_edit.clone();
    let delete_handler = on_delete.clone();
    let steer_handler = on_steer.clone();

    centered_stripe(
        div()
            .w_full()
            .max_w(px(728.0))
            .h(px(32.0))
            .px(px(8.0))
            .rounded(px(8.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.border_strong)
            .shadow_sm()
            .flex()
            .items_center()
            .justify_between()
            .gap(px(6.0))
            // Left: icon + truncated prompt preview + optional attachment badge
            .child(
                div()
                    .flex_1()
                    .min_w(px(0.0))
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .child(app_icon(IconName::LoaderCircle, 11.0, theme.text_tertiary))
                    .child(
                        div()
                            .flex_1()
                            .min_w(px(0.0))
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .overflow_hidden()
                            .child(
                                div()
                                    .truncate()
                                    .child(text),
                            ),
                    )
                    .when(has_attachments, |el| {
                        el.child(
                            div()
                                .flex_shrink_0()
                                .child(app_icon(IconName::Paperclip, 11.0, theme.text_ghost)),
                        )
                    }),
            )
            // Right: three compact controls
            .child(
                div()
                    .flex_shrink_0()
                    .flex()
                    .items_center()
                    .gap(px(4.0))
                    // Edit
                    .child(
                        div()
                            .id("queued-edit")
                            .px(px(6.0))
                            .py(px(3.0))
                            .rounded(px(5.0))
                            .text_size(px(10.5))
                            .border_1()
                            .border_color(theme.border_strong)
                            .when_some(edit_handler, |element, handler| {
                                element
                                    .cursor_default()
                                    .hover(|style| style.bg(theme.overlay))
                                    .active(|style| style.bg(theme.overlay_strong))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .bg(theme.overlay)
                            .text_color(theme.text_secondary)
                            .child("Edit"),
                    )
                    // Delete
                    .child(
                        div()
                            .id("queued-delete")
                            .size(px(22.0))
                            .rounded(px(5.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .border_1()
                            .border_color(theme.border_strong)
                            .when_some(delete_handler, |element, handler| {
                                element
                                    .cursor_default()
                                    .hover(|style| style.bg(theme.danger_soft).border_color(theme.danger))
                                    .active(|style| style.bg(theme.danger_soft))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .child(app_icon(IconName::X, 10.0, theme.text_tertiary)),
                    )
                    // Steer / Send Now
                    .child(
                        div()
                            .id("queued-steer")
                            .px(px(7.0))
                            .py(px(3.0))
                            .rounded(px(5.0))
                            .flex()
                            .items_center()
                            .gap(px(4.0))
                            .text_size(px(10.5))
                            .bg(theme.accent)
                            .text_color(theme.canvas)
                            .when_some(steer_handler, |element, handler| {
                                element
                                    .cursor_default()
                                    .hover(|style| style.opacity(0.9))
                                    .active(|style| style.opacity(0.8))
                                    .on_click(move |_, window, cx| {
                                        (handler)(window, cx);
                                    })
                            })
                            .child(app_icon(IconName::Zap, 10.0, theme.canvas))
                            .child("Steer"),
                    ),
            ),
        6.0,
    )
}
