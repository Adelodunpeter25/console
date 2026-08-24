use std::cell::RefCell;
use std::rc::Rc;

use crate::markdown::render::{
    self as markdown, Ctx as MarkdownCtx, MarkdownView, Metrics, Palette, TranscriptSelection,
};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{
    AnyElement, App, ElementId, FontWeight, IntoElement, ParentElement, RenderOnce, Styled, Window,
    div, prelude::*, px,
};

fn thinking_word_count(text: &str) -> usize {
    text.split_whitespace().count()
}

fn render_markdown(
    text: &str,
    view: Option<&Rc<RefCell<MarkdownView>>>,
    ctx: &MarkdownCtx<'_>,
    mend: bool,
) -> AnyElement {
    let Some(view) = view else {
        return div().child(text.to_owned()).into_any_element();
    };
    let mut view = view.borrow_mut();
    view.set_text(text, mend);
    markdown::markdown(&view, ctx)
        .unwrap_or_else(|| div().child(text.to_owned()).into_any_element())
}

/// A collapsible assistant reasoning section.
///
/// The caller owns the disclosure state so it survives transcript
/// virtualization. This component only renders the stable header/content pair
/// and reports the requested expanded state when the header is clicked.
#[derive(IntoElement)]
pub struct ThinkingBlock {
    id: String,
    text: String,
    collapsed: bool,
    markdown_view: Option<Rc<RefCell<MarkdownView>>>,
    selection: TranscriptSelection,
    is_streaming: bool,
    on_toggle: Option<Rc<dyn Fn(bool, &mut Window, &mut App) + 'static>>,
}

impl ThinkingBlock {
    pub fn new(id: impl Into<String>, text: impl Into<String>, collapsed: bool) -> Self {
        Self {
            id: id.into(),
            text: text.into(),
            collapsed,
            markdown_view: None,
            selection: TranscriptSelection::default(),
            is_streaming: false,
            on_toggle: None,
        }
    }

    pub fn markdown_view(mut self, view: Rc<RefCell<MarkdownView>>) -> Self {
        self.markdown_view = Some(view);
        self
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = selection;
        self
    }

    pub fn streaming(mut self, streaming: bool) -> Self {
        self.is_streaming = streaming;
        self
    }

    pub fn on_toggle(mut self, handler: impl Fn(bool, &mut Window, &mut App) + 'static) -> Self {
        self.on_toggle = Some(Rc::new(handler));
        self
    }
}

impl RenderOnce for ThinkingBlock {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        // Reasoning is secondary content — paint it a step quieter than the
        // assistant's answer text.
        let mut palette = Palette::from_theme(&theme);
        palette.text = theme.text_secondary;
        let markdown_ctx =
            MarkdownCtx::new(self.id.clone(), &palette, Metrics::BODY, self.selection);
        let collapsed = self.collapsed;
        let on_toggle = self.on_toggle;
        let id = self.id;
        let word_count = thinking_word_count(&self.text);
        let text = self.text;
        let markdown_view = self.markdown_view;
        let is_streaming = self.is_streaming;
        let header_id = format!("thinking-header-{id}");
        let block_id = format!("thinking-{id}");

        let header = div()
            .id(ElementId::Name(header_id.into()))
            .w_full()
            .min_h(px(16.0))
            .flex()
            .items_center()
            .justify_between()
            .gap(px(6.0))
            .cursor_pointer()
            .on_click(move |_, window, cx| {
                if let Some(on_toggle) = &on_toggle {
                    // `collapsed` is the current collapsed flag, which is
                    // exactly the next value for the `expanded` callback. The
                    // previous implementation sent `!collapsed` and made
                    // every click request the current state again.
                    on_toggle(collapsed, window, cx);
                }
            })
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .child(app_icon(IconName::Sparkle, 12.0, theme.accent))
                    .child(
                        div()
                            .text_size(px(11.0))
                            .font_weight(FontWeight::BOLD)
                            .text_color(theme.text_tertiary)
                            .child("THINKING"),
                    )
                    .when(word_count > 0, |element| {
                        element.child(
                            div()
                                .text_size(px(10.5))
                                .text_color(theme.text_ghost)
                                .child(format!(
                                    "({word_count} {})",
                                    if word_count == 1 { "word" } else { "words" }
                                )),
                        )
                    }),
            )
            .child(app_icon(
                if collapsed {
                    IconName::ChevronDown
                } else {
                    IconName::ChevronUp
                },
                12.0,
                theme.text_ghost,
            ));

        div()
            .id(ElementId::Name(block_id.into()))
            .px(px(12.0))
            .py(px(6.0))
            .rounded(px(9.0))
            .bg(theme.inset)
            .border_1()
            .border_color(theme.border)
            .flex()
            .flex_col()
            .gap(px(4.0))
            .child(header)
            .when(!collapsed, |element| {
                element.child(render_markdown(
                    &text,
                    markdown_view.as_ref(),
                    &markdown_ctx,
                    is_streaming,
                ))
            })
    }
}

#[cfg(test)]
mod tests {
    use super::thinking_word_count;

    #[test]
    fn counts_words_using_whitespace_boundaries() {
        assert_eq!(thinking_word_count("  first\nsecond\tthird  "), 3);
        assert_eq!(thinking_word_count("   "), 0);
    }
}
