//! Selectable log viewer: wrapped mono lines, vertical scroll only.
//!
//! Extracted from the CodeViewer path for run output. Unlike CodeViewer there
//! is no line-number gutter and no horizontal scroll — long lines soft-wrap
//! at the viewport width. Selection reuses the transcript machinery
//! (`TranscriptSelection` + `plain_log_line`), which is geometry-based and
//! safe on multi-byte text, instead of CodeViewer's display-column
//! arithmetic.

use std::rc::Rc;

use gpui::{
    App, ClipboardItem, ElementId, IntoElement, KeyDownEvent, ListState, ParentElement, RenderOnce,
    Styled, Window, canvas, div, list, prelude::*, px,
};

use crate::markdown::render::{
    self, Ctx, Metrics, Palette, TranscriptSelection, plain_log_line,
};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::theme::Theme;

/// Estimated row height for the measured list. Rows wrap, so real heights
/// vary; the list measures items and this is only the initial estimate.
pub const ESTIMATED_LOG_ROW_HEIGHT: f32 = 17.0;

#[derive(IntoElement)]
pub struct LogsViewer {
    id: String,
    lines: Rc<Vec<String>>,
    list_state: ListState,
    selection: Option<TranscriptSelection>,
    scrollbar_state: Option<Rc<ScrollbarState>>,
}

impl LogsViewer {
    pub fn new(id: impl Into<String>, lines: Rc<Vec<String>>, list_state: ListState) -> Self {
        Self {
            id: id.into(),
            lines,
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

impl RenderOnce for LogsViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        if self.list_state.item_count() != self.lines.len() {
            self.list_state.reset(self.lines.len());
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

        let lines_rc = self.lines.clone();
        let selection_for_list = selection.clone();
        let viewer_id = self.id.clone();
        let scrollbar = self
            .scrollbar_state
            .as_ref()
            .map(|s| scrollbar::vertical(&self.list_state, s));

        div()
            .id(ElementId::Name(format!("logs-viewer-{}", self.id).into()))
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
                list(self.list_state.clone(), move |line_ix, _window, cx| {
                    let theme = Theme::current(cx);
                    let palette = Palette::from_theme(&theme);
                    let line = lines_rc.get(line_ix).map(String::as_str).unwrap_or("");
                    let shown = if line.is_empty() { " " } else { line };
                    let ctx = Ctx::new(
                        format!("{viewer_id}-line-{line_ix}"),
                        &palette,
                        Metrics::COMPACT,
                        selection_for_list.clone(),
                    );
                    div()
                        .w_full()
                        .px(px(8.0))
                        .text_size(px(ctx.metrics.code_text_size))
                        .line_height(px(ctx.metrics.code_line_height))
                        .child(plain_log_line(
                            shown,
                            theme.text_secondary,
                            &ctx,
                        ))
                        .into_any_element()
                })
                .flex_1()
                .size_full(),
            )
            .children(scrollbar)
    }
}
