//! Full-page Markdown Preview component for workspace tabs.

use gpui::{
    App, ElementId, IntoElement, ParentElement, RenderOnce, Styled, Window, div, prelude::*, px,
};

use crate::markdown::render::render_markdown;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct MarkdownViewer {
    path: String,
    content: String,
}

impl MarkdownViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
        }
    }
}

impl RenderOnce for MarkdownViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let file_name = std::path::Path::new(&self.path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&self.path)
            .to_string();

        let toolbar = div()
            .flex()
            .items_center()
            .justify_between()
            .h(px(32.0))
            .px(px(16.0))
            .border_b_1()
            .border_color(theme.border)
            .bg(theme.raised)
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .text_size(px(11.5))
                    .text_color(theme.text_secondary)
                    .child(app_icon(IconName::File, 12.0, theme.text_tertiary))
                    .child(file_name)
                    .child(
                        div()
                            .text_size(px(10.0))
                            .text_color(theme.accent)
                            .px(px(6.0))
                            .py(px(1.0))
                            .rounded(px(3.0))
                            .bg(theme.overlay)
                            .child("Preview"),
                    ),
            );

        let body = div()
            .id(ElementId::Name(format!("md-preview-scroll-{}", self.path).into()))
            .size_full()
            .min_h_0()
            .overflow_y_scroll()
            .bg(theme.canvas)
            .px(px(32.0))
            .py(px(24.0))
            .child(
                div()
                    .max_w(px(820.0))
                    .w_full()
                    .mx_auto()
                    .child(render_markdown(&self.content, &theme)),
            );

        div()
            .id(ElementId::Name(format!("md-viewer-{}", self.path).into()))
            .size_full()
            .flex()
            .flex_col()
            .min_h_0()
            .min_w_0()
            .child(toolbar)
            .child(body)
    }
}
