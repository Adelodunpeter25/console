//! The settings modal shell: dark backdrop, centered card, title bar with a
//! close button, and a footer slot for the action row. Every settings dialog
//! (server add/edit, pair-a-phone, account rename) renders inside this shell,
//! so a new dialog gets the same chrome instead of hand-rolling its own.

use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct Modal {
    pub title: String,
    /// Card width in pixels. Most dialogs read best narrow (360); forms with
    /// several labelled fields go wider (520).
    pub width: f32,
    /// The body between the title bar and the footer.
    pub content: AnyElement,
    /// The action row at the card's bottom, built by the caller.
    pub footer: AnyElement,
    pub on_close: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for Modal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close.clone();
        let backdrop_close = self.on_close.clone();

        // The backdrop closes the modal; the card stops the click so
        // clicking inside it does not dismiss.
        div()
            .absolute()
            .inset_0()
            .bg(gpui::hsla(0.0, 0.0, 0.0, 0.6))
            .flex()
            .items_center()
            .justify_center()
            .p(px(24.0))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                (backdrop_close)(window, cx);
            })
            .child(
                div()
                    .w(px(self.width))
                    .max_h_full()
                    .p(px(16.0))
                    .rounded(px(10.0))
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.surface)
                    .shadow_lg()
                    .flex()
                    .flex_col()
                    .gap(px(12.0))
                    .on_mouse_down(MouseButton::Left, |_, _, cx| {
                        cx.stop_propagation();
                    })
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .justify_between()
                            .child(
                                div()
                                    .text_size(px(14.0))
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .text_color(theme.text)
                                    .child(self.title),
                            )
                            .child(
                                div()
                                    .id("btn-modal-close")
                                    .cursor_pointer()
                                    .on_mouse_down(MouseButton::Left, {
                                        move |_event, window, cx| {
                                            cx.stop_propagation();
                                            (on_close)(window, cx);
                                        }
                                    })
                                    .child(app_icon(IconName::Close, 14.0, theme.text_secondary)),
                            ),
                    )
                    .child(self.content)
                    .child(self.footer),
            )
    }
}
