//! Panel displayed when a file preview is blocked or failed to load.

use std::rc::Rc;

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, Styled, Window, div, prelude::*, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct BlockedFilePanel {
    title: String,
    message: String,
    icon: IconName,
    on_retry: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl BlockedFilePanel {
    pub fn new(title: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            title: title.into(),
            message: message.into(),
            icon: IconName::File,
            on_retry: None,
        }
    }

    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = icon;
        self
    }

    pub fn on_retry(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_retry = Some(Rc::new(handler));
        self
    }
}

impl RenderOnce for BlockedFilePanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_retry = self.on_retry;

        div()
            .id("blocked-file-panel")
            .size_full()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .p(px(24.0))
            .bg(theme.canvas)
            .child(
                div()
                    .max_w(px(460.0))
                    .w_full()
                    .p(px(24.0))
                    .rounded(px(8.0))
                    .bg(theme.surface)
                    .border_1()
                    .border_color(theme.border)
                    .flex()
                    .flex_col()
                    .items_center()
                    .text_center()
                    .gap(px(12.0))
                    .child(
                        div()
                            .p(px(8.0))
                            .rounded_full()
                            .bg(theme.canvas)
                            .child(app_icon(self.icon, 28.0, theme.text_tertiary)),
                    )
                    .child(
                        div()
                            .text_size(px(14.0))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child(self.title),
                    )
                    .child(
                        div()
                            .text_size(px(12.0))
                            .text_color(theme.text_secondary)
                            .line_height(px(18.0))
                            .child(self.message),
                    )
                    .when_some(on_retry, |panel, retry_cb| {
                        panel.child(
                            div()
                                .id("blocked-file-retry-btn")
                                .mt(px(4.0))
                                .px(px(14.0))
                                .py(px(6.0))
                                .rounded(px(6.0))
                                .bg(theme.inverse)
                                .text_color(theme.on_inverse)
                                .text_size(px(12.0))
                                .font_weight(FontWeight::MEDIUM)
                                .cursor_pointer()
                                .hover(|s| s.opacity(0.9))
                                .on_click(move |_, window, cx| (retry_cb)(window, cx))
                                .child("Retry"),
                        )
                    }),
            )
    }
}
