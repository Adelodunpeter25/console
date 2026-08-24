use gpui::{Context, IntoElement, ParentElement, Render, Styled, Window, div, px, SharedString};

use crate::theme::Theme;

/// Placeholder extra-keys bar — isolated, not wired to any app state.
/// Desktop has a physical keyboard; this is kept only as a stub for parity
/// experiments and is not shown in the app.
pub struct ExtraKeysBar;

impl ExtraKeysBar {
    pub fn new() -> Self {
        Self
    }
}

impl Render for ExtraKeysBar {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        div()
            .w_full()
            .flex()
            .items_center()
            .gap(px(6.0))
            .px(px(8.0))
            .py(px(6.0))
            .bg(theme.canvas)
            .border_t_1()
            .border_color(theme.border)
            .text_size(px(11.0))
            .text_color(theme.text_ghost)
            .child(SharedString::from("Extra keys (desktop stub)"))
    }
}
