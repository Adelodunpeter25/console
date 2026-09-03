//! Full-page Markdown Preview component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{
    App, ElementId, IntoElement, ParentElement, RenderOnce, Styled, Window, div, prelude::*, px,
};

use crate::markdown::render::{Ctx, MarkdownView, Metrics, Palette, TranscriptSelection, markdown};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct MarkdownViewer {
    path: String,
    view: Rc<RefCell<MarkdownView>>,
}

impl MarkdownViewer {
    pub fn new(path: impl Into<String>, view: Rc<RefCell<MarkdownView>>) -> Self {
        Self {
            path: path.into(),
            view,
        }
    }
}

impl RenderOnce for MarkdownViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let palette = Palette::from_theme(&theme);
        let selection = TranscriptSelection::default();
        let ctx = Ctx::new("md-block", &palette, Metrics::BODY, selection);

        div()
            .id(ElementId::Name(format!("md-preview-scroll-{}", self.path).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .overflow_y_scroll()
            .bg(theme.canvas)
            .px(px(32.0))
            .py(px(24.0))
            .child(
                div()
                    .max_w(px(820.0))
                    .w_full()
                    .mx_auto()
                    .children(markdown(&self.view.borrow(), &ctx)),
            )
    }
}
