use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{
    App, IntoElement, MouseButton, ParentElement, RenderOnce, SharedString, Styled, Window, div,
    img, prelude::*, px,
};

#[derive(IntoElement)]
pub struct ImageViewerModal {
    /// Decoded image bytes — gpui renders these directly, unlike a `data:`
    /// URI string which the img element would try to HTTP-GET.
    pub image: std::sync::Arc<gpui::Image>,
    pub alt: SharedString,
    on_close: Box<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ImageViewerModal {
    pub fn new(
        image: std::sync::Arc<gpui::Image>,
        alt: impl Into<SharedString>,
        on_close: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            image,
            alt: alt.into(),
            on_close: Box::new(on_close),
        }
    }
}

impl RenderOnce for ImageViewerModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        div()
            .id("image-viewer-modal")
            .absolute()
            .inset_0()
            .bg(gpui::hsla(0.0, 0.0, 0.0, 0.85))
            .flex()
            .items_center()
            .justify_center()
            .p(px(24.0))
            .on_mouse_down(MouseButton::Left, {
                let close = self.on_close;
                move |_, window, cx| (close)(window, cx)
            })
            // Close × button
            .child(
                div()
                    .absolute()
                    .top(px(20.0))
                    .right(px(20.0))
                    .id("image-viewer-close")
                    .size(px(32.0))
                    .rounded_full()
                    .bg(gpui::hsla(0.0, 0.0, 0.0, 0.6))
                    .flex()
                    .items_center()
                    .justify_center()
                    .cursor_default()
                    .hover(|s| s.bg(gpui::hsla(0.0, 0.0, 0.0, 0.85)))
                    .child(app_icon(IconName::X, 14.0, theme.text)),
            )
            // Image preview: the img is constrained directly (max size +
            // ScaleDown, like the transcript's images) so it always has a
            // concrete layout size — a zero-sized wrapper would collapse it.
            .child(
                div()
                    .id("image-viewer-content")
                    .max_w(px(960.0))
                    .max_h(px(640.0))
                    .rounded(px(12.0))
                    .overflow_hidden()
                    .shadow_xl()
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.surface)
                    .on_mouse_down(MouseButton::Left, |_, _, _| {})
                    .child(
                        img(self.image.clone())
                            .max_w(px(936.0))
                            .max_h(px(616.0))
                            .object_fit(gpui::ObjectFit::ScaleDown),
                    ),
            )
    }
}
