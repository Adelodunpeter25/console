//! The workspace's empty state: shown in a pane's content area when no chat
//! tab is open. A quiet, centered welcome — no composer, no footer — with a
//! single "New Chat" action to get started.

use std::rc::Rc;

use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// Centered empty state for a pane with no open chat tab.
#[derive(IntoElement)]
pub struct EmptyChatState {
    on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl EmptyChatState {
    pub fn new(on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>) -> Self {
        Self { on_new_chat }
    }
}

impl RenderOnce for EmptyChatState {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_new_chat = self.on_new_chat;

        div()
            .id("workspace-empty-state")
            .flex_1()
            .min_h_0()
            .w_full()
            .bg(theme.chat_canvas)
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .gap_y(px(6.0))
            .child(
                div()
                    .size(px(44.0))
                    .rounded(px(12.0))
                    .bg(theme.chat_canvas)
                    .border_1()
                    .border_color(theme.border)
                    .flex()
                    .items_center()
                    .justify_center()
                    .child(app_icon(IconName::ChatRoundLine, 24.0, theme.accent)),
            )
            .child(
                div()
                    .mt(px(14.0))
                    .text_size(px(20.0))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(theme.text)
                    .child("No chat open"),
            )
            .child(
                div()
                    .max_w(px(360.0))
                    .text_center()
                    .text_size(px(12.5))
                    .line_height(px(19.0))
                    .text_color(theme.text_tertiary)
                    .child("Pick a task from the sidebar or start a new chat to begin."),
            )
            .child(
                div()
                    .id("empty-state-new-chat")
                    .mt(px(20.0))
                    .h(px(34.0))
                    .px(px(16.0))
                    .rounded(px(8.0))
                    .bg(theme.chat_canvas)
                    .border_1()
                    .border_color(theme.border)
                    .flex()
                    .items_center()
                    .gap_x(px(8.0))
                    .cursor_default()
                    .hover(|s| s.border_color(theme.accent).opacity(0.9))
                    .active(|s| s.opacity(0.8))
                    .on_click(move |_, window, cx| {
                        (on_new_chat)(window, cx);
                    })
                    .child(app_icon(IconName::Compose, 14.0, theme.on_inverse))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.on_inverse)
                            .child("New Chat"),
                    ),
            )
    }
}
