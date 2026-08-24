//! The agent notice banner: a neutral strip for transient status notices such
//! as a context-compaction summary.

use gpui::{IntoElement, ParentElement, Styled, div, px};

use crate::common::centered_stripe;
use crate::theme::Theme;

pub fn notice_banner(message: String, theme: Theme) -> impl IntoElement {
    centered_stripe(
        div()
            .w_full()
            .max_w(px(768.0))
            .px(px(10.0))
            .py(px(6.0))
            .rounded(px(6.0))
            .bg(theme.overlay)
            .text_size(px(11.5))
            .text_color(theme.text_tertiary)
            .child(message),
        6.0,
    )
}
