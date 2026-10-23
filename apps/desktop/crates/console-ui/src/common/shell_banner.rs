//! One compact row above the composer while background shells are running:
//! SHELLS, a count, and the latest command. Clicking it opens the Shells tab
//! in the right sidebar, where the output and stop buttons live.

use std::rc::Rc;

use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, Window, div, px,
};

use crate::common::centered_stripe;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub fn shell_banner(
    count: usize,
    latest_command: String,
    on_open: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: Theme,
) -> impl IntoElement {
    centered_stripe(
        div()
            .id("shell-banner")
            .w_full()
            .max_w(px(728.0))
            .px(px(9.0))
            .py(px(7.0))
            .rounded(px(8.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.border_strong)
            .shadow_sm()
            .flex()
            .items_center()
            .gap(px(8.0))
            .cursor_pointer()
            .hover(|s| s.bg(theme.overlay))
            .on_click(move |_, window, cx| on_open(window, cx))
            .child(app_icon(IconName::Terminal, 13.0, theme.accent))
            .child(
                div()
                    .text_size(px(11.0))
                    .font_weight(FontWeight::BOLD)
                    .text_color(theme.text_secondary)
                    .child("SHELLS"),
            )
            .child(
                div()
                    .px(px(5.0))
                    .py(px(1.0))
                    .rounded(px(4.0))
                    .bg(theme.overlay)
                    .text_size(px(10.5))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(theme.text_tertiary)
                    .child(count.to_string()),
            )
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .truncate()
                    .text_size(px(11.5))
                    .text_color(theme.text_secondary)
                    .child(format!("• {latest_command}")),
            )
            .child(app_icon(IconName::ChevronRight, 12.0, theme.text_tertiary)),
        6.0,
    )
}
