//! Follow-up suggestion pills shown above the composer once a run settles.
//! Clicking one sends it as the next prompt.

use std::rc::Rc;

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement, Styled,
    Window, div, px,
};

use crate::common::centered_stripe;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub fn suggestion_chips(
    items: Vec<String>,
    on_pick: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    theme: Theme,
) -> impl IntoElement {
    centered_stripe(
        div()
            .w_full()
            .max_w(px(728.0))
            .flex()
            .flex_row()
            .flex_wrap()
            .gap(px(8.0))
            .children(items.into_iter().enumerate().map(|(index, text)| {
                let on_pick = on_pick.clone();
                let picked = text.clone();
                div()
                    .id(format!("suggestion-chip-{index}"))
                    .max_w(px(280.0))
                    .pl(px(6.0))
                    .pr(px(14.0))
                    .py(px(5.0))
                    .rounded(px(999.0))
                    .bg(theme.composer)
                    .border_1()
                    .border_color(theme.border_strong)
                    .flex()
                    .flex_row()
                    .items_center()
                    .gap(px(8.0))
                    .cursor_pointer()
                    .hover(|style| style.bg(theme.overlay))
                    .active(|style| style.bg(theme.overlay_strong))
                    .on_click(move |_, window, cx| (on_pick)(picked.clone(), window, cx))
                    .child(
                        div()
                            .flex_shrink_0()
                            .size(px(22.0))
                            .rounded(px(999.0))
                            .bg(theme.accent)
                            .flex()
                            .items_center()
                            .justify_center()
                            .child(app_icon(IconName::ArrowUp, 13.0, theme.on_inverse)),
                    )
                    .child(
                        div()
                            .min_w(px(0.0))
                            .text_size(px(13.0))
                            .text_color(theme.text)
                            .truncate()
                            .child(text),
                    )
            })),
        6.0,
    )
}
