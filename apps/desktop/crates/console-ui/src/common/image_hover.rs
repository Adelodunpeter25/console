//! The picture card shown above an `[Image N]` chip while the pointer rests on it.

use std::sync::Arc;
use std::time::Duration;

use console_core::ImageAttachment;
use gpui::{
    AnyElement, Context, Entity, InteractiveElement, IntoElement, ParentElement, Styled,
    StyledImage, Window, div, img, px,
};

use crate::primitives::menu::{FloatingSurface, MenuAlign};
use crate::theme::Theme;

/// Pause before the card opens, so a pointer passing over a chip does not
/// flash it. Matches the changed-file chips' diff popover.
pub(crate) const IMAGE_HOVER_DELAY: Duration = Duration::from_millis(300);

/// Which image chip the pointer is on, and whether its card is open yet.
/// One per chip surface (a composer field, a sent bubble); the surface only
/// supplies where its chips are.
#[derive(Default)]
pub(crate) struct ImageChipHover {
    hovered: Option<usize>,
    shown: Option<usize>,
    /// Bumped on every hover change so a superseded delayed open does nothing.
    generation: u64,
    /// The decoded picture for the last card: (number, payload length, image).
    cache: Option<(usize, usize, Arc<gpui::Image>)>,
}

impl ImageChipHover {
    /// The pointer is now on image chip `number` (1-based), or on none. The
    /// card opens after [`IMAGE_HOVER_DELAY`] if the pointer is still there.
    pub fn hover(&mut self, number: Option<usize>, cx: &mut Context<Self>) {
        if self.hovered == number {
            return;
        }
        self.hovered = number;
        self.shown = None;
        self.generation += 1;
        let generation = self.generation;
        cx.notify();
        let Some(number) = number else { return };
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(IMAGE_HOVER_DELAY).await;
            let _ = this.update(cx, |s, cx| {
                if s.generation == generation && s.hovered == Some(number) {
                    s.shown = Some(number);
                    cx.notify();
                }
            });
        })
        .detach();
    }

    /// Close the card and forget the hover, for a click or a scroll.
    pub fn dismiss(&mut self, cx: &mut Context<Self>) {
        if self.shown.is_some() || self.hovered.is_some() {
            self.shown = None;
            self.hovered = None;
            self.generation += 1;
            cx.notify();
        }
    }

    /// The open card, anchored to the chip rect `rect_for` finds for the
    /// shown image. `None` while closed or when the image cannot be decoded.
    pub fn card(
        &mut self,
        attachments: &[ImageAttachment],
        rect_for: impl FnOnce(usize) -> Option<gpui::Bounds<gpui::Pixels>>,
        theme: Theme,
    ) -> Option<AnyElement> {
        let number = self.shown?;
        let attachment = attachments.get(number.checked_sub(1)?)?;
        let rect = rect_for(number)?;
        let image = match self
            .cache
            .as_ref()
            .filter(|(n, len, _)| *n == number && *len == attachment.data.len())
        {
            Some((_, _, image)) => image.clone(),
            None => {
                let image = crate::common::attachment_image(attachment)?;
                self.cache = Some((number, attachment.data.len(), image.clone()));
                image
            }
        };
        Some(image_hover_card(image, rect, theme))
    }
}

/// Close `state`'s card when the wheel turns. The transcript scrolls under a
/// still pointer, which sends no move event, so the card would hang over
/// other text.
pub(crate) fn dismiss_on_scroll(window: &mut Window, state: Entity<ImageChipHover>) {
    window.on_mouse_event(move |_: &gpui::ScrollEvent, phase, _window, cx| {
        if phase == gpui::DispatchPhase::Bubble {
            state.update(cx, |s, cx| s.dismiss(cx));
        }
    });
}

/// The card, anchored above `chip`. Callers defer it so it paints over
/// everything, including an expanded inline webview.
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
    FloatingSurface::new(
        card.into_any_element(),
        chip,
        MenuAlign::AboveLeft,
        px(6.0),
        px(8.0),
    )
    .into_any_element()
}
