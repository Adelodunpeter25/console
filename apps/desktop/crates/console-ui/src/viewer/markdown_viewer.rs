//! High-performance virtualized Markdown Preview component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{
    App, ClipboardItem, ElementId, IntoElement, KeyDownEvent, ListState, ParentElement, RenderOnce,
    Styled, Window, canvas, div, list, prelude::*, px,
};

use crate::markdown::render::{
    self, Ctx, MarkdownView, Metrics, Palette, TranscriptSelection, render_markdown_block,
};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct MarkdownViewer {
    path: String,
    view: Rc<RefCell<MarkdownView>>,
    list_state: ListState,
    selection: Option<TranscriptSelection>,
    scrollbar_state: Option<Rc<ScrollbarState>>,
}

impl MarkdownViewer {
    pub fn new(
        path: impl Into<String>,
        view: Rc<RefCell<MarkdownView>>,
        list_state: ListState,
    ) -> Self {
        Self {
            path: path.into(),
            view,
            list_state,
            selection: None,
            scrollbar_state: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = Some(selection);
        self
    }

    pub fn scrollbar_state(mut self, state: Rc<ScrollbarState>) -> Self {
        self.scrollbar_state = Some(state);
        self
    }
}

impl RenderOnce for MarkdownViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let block_count = self.view.borrow().block_count();
        if self.list_state.item_count() != block_count {
            self.list_state.reset(block_count);
        }
        let selection = self.selection.unwrap_or_default();
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

        let view_rc = self.view.clone();
        let selection_for_list = selection.clone();
        let scrollbar = self
            .scrollbar_state
            .as_ref()
            .map(|s| scrollbar::vertical(&self.list_state, s));

        div()
            .id(ElementId::Name(format!("md-viewer-{}", self.path).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .relative()
            .flex()
            .flex_col()
            .bg(theme.canvas)
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
                list(self.list_state.clone(), move |block_ix, _window, cx| {
                    let theme = Theme::current(cx);
                    let palette = Palette::from_theme(&theme);
                    let ctx = Ctx::new(
                        "md-block",
                        &palette,
                        Metrics::BODY,
                        selection_for_list.clone(),
                    );
                    let view = view_rc.borrow();
                    if let Some(el) = render_markdown_block(&view, &ctx, block_ix) {
                        div()
                            .w_full()
                            .child(
                                div()
                                    .max_w(px(820.0))
                                    .w_full()
                                    .mx_auto()
                                    .px(px(32.0))
                                    .pt(if block_ix == 0 { px(24.0) } else { px(0.0) })
                                    .pb(px(ctx.metrics.block_gap))
                                    .child(el),
                            )
                            .into_any_element()
                    } else {
                        div().into_any_element()
                    }
                })
                .flex_1()
                .size_full(),
            )
            .children(scrollbar)
    }
}
