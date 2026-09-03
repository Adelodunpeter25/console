//! Full-page Markdown Preview component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{
    App, ClipboardItem, ElementId, IntoElement, KeyDownEvent, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, canvas, div, prelude::*, px,
};

use crate::markdown::render::{
    self, Ctx, MarkdownView, Metrics, Palette, TranscriptSelection, markdown,
};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct MarkdownViewer {
    path: String,
    view: Rc<RefCell<MarkdownView>>,
    selection: Option<TranscriptSelection>,
}

impl MarkdownViewer {
    pub fn new(path: impl Into<String>, view: Rc<RefCell<MarkdownView>>) -> Self {
        Self {
            path: path.into(),
            view,
            selection: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = Some(selection);
        self
    }
}

impl RenderOnce for MarkdownViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let palette = Palette::from_theme(&theme);
        let selection = self.selection.unwrap_or_default();
        let ctx = Ctx::new("md-block", &palette, Metrics::BODY, selection.clone());

        let selection_listener = {
            let selection = selection.clone();
            canvas(
                |_, _, _| (),
                move |_, _, window, _| render::install_selection_input(window, &selection),
            )
            .absolute()
            .w(px(0.0))
            .h(px(0.0))
        };

        div()
            .id(ElementId::Name(format!("md-viewer-{}", self.path).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .relative()
            .tab_index(0)
            .on_key_down({
                let selection = selection.clone();
                move |event: &KeyDownEvent, _, cx| {
                    if (event.keystroke.modifiers.platform || event.keystroke.modifiers.control)
                        && event.keystroke.key == "c"
                    {
                        if let Some(text) = selection.selection.borrow().selected_text() {
                            cx.write_to_clipboard(ClipboardItem::new_string(text));
                        }
                    }
                }
            })
            .child(render::frame_reset(selection))
            .child(selection_listener)
            .child(
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
                    ),
            )
    }
}
