use gpui::{
    AnyElement, App, Div, ElementId, InteractiveElement, Interactivity, ParentElement, RenderOnce,
    Stateful, StyleRefinement, Styled, Window, div, prelude::*, px,
};

use super::icon;
use crate::theme::Theme;

/// A simple styled text field wrapper — a fixed-height bordered input shell
/// with an optional leading icon and accent border while focused.
#[derive(IntoElement)]
pub struct TextField {
    base: Stateful<Div>,
    icon: Option<(&'static str, f32)>,
    placeholder: Option<String>,
    value: String,
    focused: bool,
}

impl TextField {
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            base: div().id(id),
            icon: None,
            placeholder: None,
            value: String::new(),
            focused: false,
        }
    }

    pub fn icon(mut self, path: &'static str, size: f32) -> Self {
        self.icon = Some((path, size));
        self
    }

    pub fn placeholder(mut self, text: impl Into<String>) -> Self {
        self.placeholder = Some(text.into());
        self
    }

    pub fn value(mut self, value: impl Into<String>) -> Self {
        self.value = value.into();
        self
    }

    pub fn focused(mut self, focused: bool) -> Self {
        self.focused = focused;
        self
    }
}

impl Styled for TextField {
    fn style(&mut self) -> &mut StyleRefinement {
        self.base.style()
    }
}

impl InteractiveElement for TextField {
    fn interactivity(&mut self) -> &mut Interactivity {
        self.base.interactivity()
    }
}

impl ParentElement for TextField {
    fn extend(&mut self, elements: impl IntoIterator<Item = AnyElement>) {
        self.base.extend(elements);
    }
}

impl RenderOnce for TextField {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let border_color = if self.focused {
            theme.accent
        } else {
            theme.border_strong
        };

        self.base
            .h(px(30.0))
            .px(px(8.0))
            .rounded(px(7.0))
            .border_1()
            .border_color(border_color)
            .bg(theme.inset)
            .flex()
            .items_center()
            .gap(px(6.0))
            .when_some(self.icon, |el, (path, size)| {
                el.child(icon(path, size, theme.text_ghost))
            })
            .child(
                div()
                    .flex_1()
                    .text_size(px(12.5))
                    .text_color(if self.value.is_empty() {
                        theme.text_ghost
                    } else {
                        theme.text
                    })
                    .truncate()
                    .child(if self.value.is_empty() {
                        self.placeholder.unwrap_or_default()
                    } else {
                        self.value
                    }),
            )
    }
}
