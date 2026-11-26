//! The picture card shown above an `[Image N]` chip while the pointer rests on it.

use std::sync::Arc;
use std::time::Duration;

use gpui::{
    AnyElement, InteractiveElement, IntoElement, ParentElement, Styled, StyledImage, deferred, div,
    img, px,
};

use crate::primitives::menu::{FloatingSurface, MenuAlign};
use crate::theme::Theme;

/// Pause before the card opens, so a pointer passing over a chip does not
/// flash it. Matches the changed-file chips' diff popover.
pub(crate) const IMAGE_HOVER_DELAY: Duration = Duration::from_millis(300);

/// Which image chip the pointer is on, and whether its card is open yet.
#[derive(Default)]
pub(crate) struct ImageChipHover {
    pub hovered: Option<usize>,
    pub shown: Option<usize>,
    /// Bumped on every hover change so a superseded delayed open does nothing.
    pub generation: u64,
    /// The decoded picture for the last card: (number, payload length, image).
    pub cache: Option<(usize, usize, Arc<gpui::Image>)>,
}

/// The card, anchored above `chip`. Deferred so it paints over everything,
/// including an expanded inline webview.
pub(crate) fn image_hover_card(
    image: Arc<gpui::Image>,
    chip: gpui::Bounds<gpui::Pixels>,
    theme: Theme,
) -> AnyElement {
    let card = div()
        .id("image-chip-hover-card")
        .p(px(4.0))
        .rounded(px(10.0))
        .bg(theme.raised)
        .border_1()
        .border_color(theme.border_strong)
        .shadow_lg()
        .child(
            div().rounded(px(7.0)).overflow_hidden().child(
                img(image)
                    .max_w(px(320.0))
                    .max_h(px(240.0))
                    .object_fit(gpui::ObjectFit::ScaleDown),
            ),
        );
    deferred(FloatingSurface::new(
        card.into_any_element(),
        chip,
        MenuAlign::AboveLeft,
        px(6.0),
        px(8.0),
    ))
    .with_priority(100)
    .into_any_element()
}
