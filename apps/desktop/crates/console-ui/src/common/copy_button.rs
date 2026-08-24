//! The copy button with copied-state feedback.
//!
//! On click it writes the content to the clipboard and swaps its icon from a
//! copy glyph to a checkmark for two seconds, fading back. Shared by message
//! bubbles and the error banner so every copy affordance behaves identically.

use std::collections::HashMap;
use std::rc::Rc;
use std::time::Duration;

use gpui::{
    Animation, AnimationExt, App, ClipboardItem, ElementId, Global, InteractiveElement,
    IntoElement, ParentElement, SharedString, StatefulInteractiveElement, Styled, div, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

#[derive(Default)]
struct CopyFeedback {
    generations: HashMap<String, u64>,
    next_generation: u64,
}

impl Global for CopyFeedback {}

fn mark_copied(id: String, cx: &mut App) {
    let generation = {
        let feedback = cx.default_global::<CopyFeedback>();
        feedback.next_generation = feedback.next_generation.wrapping_add(1);
        let generation = feedback.next_generation;
        feedback.generations.insert(id.clone(), generation);
        generation
    };
    cx.refresh_windows();

    cx.spawn(async move |cx| {
        cx.background_executor().timer(Duration::from_secs(2)).await;
        cx.update(|cx| {
            let feedback = cx.default_global::<CopyFeedback>();
            if feedback.generations.get(&id) == Some(&generation) {
                feedback.generations.remove(&id);
                cx.refresh_windows();
            }
        });
    })
    .detach();
}

/// A compact icon button that copies `content` and shows checkmark feedback
/// for two seconds. `id` must be stable per copy site so feedback does not
/// bleed between buttons.
pub fn copy_button(
    id: String,
    content: impl Into<SharedString>,
    theme: Theme,
    cx: &mut App,
) -> impl IntoElement {
    copy_button_with_action(id, content, theme, None, cx)
}

/// [`copy_button`] with an optional callback invoked after a successful copy
/// (used by the error banner to dismiss itself).
pub fn copy_button_with_action(
    id: String,
    content: impl Into<SharedString>,
    theme: Theme,
    on_copied: Option<Rc<dyn Fn(&mut App) + 'static>>,
    cx: &mut App,
) -> impl IntoElement {
    let content = content.into();
    let copied_generation = cx
        .default_global::<CopyFeedback>()
        .generations
        .get(&id)
        .copied();
    let copied = copied_generation.is_some();
    let animation_key = format!(
        "copy-feedback-{id}-{copied}-{}",
        copied_generation.unwrap_or_default()
    );
    let copy_id = id.clone();

    div()
        .id(ElementId::Name(id.into()))
        .size(px(27.0))
        .rounded(px(8.0))
        .flex()
        .items_center()
        .justify_center()
        .cursor_default()
        .hover(|element| element.bg(theme.overlay_strong))
        .on_click(move |_, _, cx| {
            cx.write_to_clipboard(ClipboardItem::new_string(content.to_string()));
            if let Some(callback) = &on_copied {
                (callback)(cx);
            }
            mark_copied(copy_id.clone(), cx);
        })
        .child(
            div()
                .size(px(14.0))
                .flex()
                .items_center()
                .justify_center()
                .child(app_icon(
                    if copied {
                        IconName::Check
                    } else {
                        IconName::Copy
                    },
                    14.0,
                    theme.text_ghost,
                ))
                .with_animation(
                    SharedString::from(animation_key),
                    Animation::new(Duration::from_millis(140)),
                    |element, delta| element.opacity(delta),
                ),
        )
}
