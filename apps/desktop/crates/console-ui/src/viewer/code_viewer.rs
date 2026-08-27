//! Centralized code and diff viewer primitive with syntax highlighting,
//! line numbering, and scrollable container.

use gpui::{
    App, ElementId, FontWeight, Hsla, IntoElement, ParentElement, RenderOnce, Styled, Window, div,
    prelude::*, px,
};

use crate::markdown::highlight::{self, Carry, Lang, lang_for_tag, lang_tag_for_path};
use crate::markdown::render::{MONO_FAMILY, Palette};
use crate::theme::Theme;

#[derive(Clone, Debug)]
pub struct CodeViewerLine {
    pub line_no: Option<usize>,
    pub old_line_no: Option<usize>,
    pub new_line_no: Option<usize>,
    pub gutter: Option<&'static str>,
    pub gutter_color: Option<Hsla>,
    pub bg_color: Option<Hsla>,
    pub text_color: Option<Hsla>,
    pub text: String,
}

#[derive(IntoElement)]
pub struct CodeViewer {
    id: String,
    path: Option<String>,
    lang: Option<Lang>,
    lines: Vec<CodeViewerLine>,
    empty_message: Option<String>,
}

impl CodeViewer {
    pub fn new(id: impl Into<String>) -> Self {
        Self {
            id: id.into(),
            path: None,
            lang: None,
            lines: Vec::new(),
            empty_message: None,
        }
    }

    pub fn for_path(mut self, path: impl Into<String>) -> Self {
        let p = path.into();
        self.lang = lang_tag_for_path(&p).and_then(lang_for_tag);
        self.path = Some(p);
        self
    }

    pub fn lang(mut self, lang: Lang) -> Self {
        self.lang = Some(lang);
        self
    }

    pub fn lines(mut self, lines: Vec<CodeViewerLine>) -> Self {
        self.lines = lines;
        self
    }

    pub fn empty_message(mut self, msg: impl Into<String>) -> Self {
        self.empty_message = Some(msg.into());
        self
    }
}

impl RenderOnce for CodeViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let palette = Palette::from_theme(&theme);

        if self.lines.is_empty() {
            return div()
                .id(ElementId::Name(format!("code-viewer-{}", self.id).into()))
                .size_full()
                .min_h_0()
                .min_w_0()
                .flex()
                .items_center()
                .justify_center()
                .py(px(48.0))
                .bg(theme.canvas)
                .text_size(px(12.0))
                .text_color(theme.text_tertiary)
                .child(self.empty_message.unwrap_or_else(|| "Empty".to_string()))
                .into_any_element();
        }

        // Determine max line number for gutter sizing
        let max_line = self
            .lines
            .iter()
            .map(|l| {
                l.line_no
                    .or(l.new_no_or_old())
                    .unwrap_or(0)
            })
            .max()
            .unwrap_or(1);
        let line_num_width = format!("{max_line}").len() * 8 + 20;

        let has_dual_line_numbers = self
            .lines
            .iter()
            .any(|l| l.old_line_no.is_some() || l.new_line_no.is_some());

        // Tokenize lines for syntax highlighting
        let lang = self.lang;
        let mut carry = Carry::None;
        let tokenized: Vec<Vec<highlight::Token>> = if let Some(l) = lang {
            self.lines
                .iter()
                .map(|line| {
                    let (tokens, next_carry) = highlight::tokenize_line(l, &line.text, carry);
                    carry = next_carry;
                    tokens
                })
                .collect()
        } else {
            vec![Vec::new(); self.lines.len()]
        };

        div()
            .id(ElementId::Name(format!("code-viewer-{}", self.id).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .overflow_y_scroll()
            .bg(theme.canvas)
            .py(px(8.0))
            .children(
                self.lines
                    .into_iter()
                    .zip(tokenized)
                    .map(|(line, tokens)| {
                        let bg = line.bg_color.unwrap_or(gpui::transparent_black());
                        let default_text_color = line.text_color.unwrap_or(theme.text);

                        let gutter_view = if has_dual_line_numbers {
                            let old_str = line.old_line_no.map_or(String::new(), |n| n.to_string());
                            let new_str = line.new_line_no.map_or(String::new(), |n| n.to_string());
                            div()
                                .flex()
                                .items_center()
                                .flex_none()
                                .child(
                                    div()
                                        .w(px(32.0))
                                        .flex_none()
                                        .text_align(gpui::TextAlign::Right)
                                        .pr(px(6.0))
                                        .font_family(MONO_FAMILY)
                                        .text_size(px(10.0))
                                        .text_color(theme.text_ghost)
                                        .child(old_str),
                                )
                                .child(
                                    div()
                                        .w(px(32.0))
                                        .flex_none()
                                        .text_align(gpui::TextAlign::Right)
                                        .pr(px(8.0))
                                        .font_family(MONO_FAMILY)
                                        .text_size(px(10.0))
                                        .text_color(theme.text_ghost)
                                        .child(new_str),
                                )
                                .when_some(line.gutter, |el, g| {
                                    let fg = line.gutter_color.unwrap_or(default_text_color);
                                    el.child(
                                        div()
                                            .w(px(14.0))
                                            .flex_none()
                                            .text_size(px(11.0))
                                            .font_weight(FontWeight::BOLD)
                                            .text_color(fg)
                                            .child(g),
                                    )
                                })
                                .into_any_element()
                        } else {
                            let num_str = line.line_no.map_or(String::new(), |n| n.to_string());
                            div()
                                .w(px(line_num_width as f32))
                                .flex_none()
                                .text_align(gpui::TextAlign::Right)
                                .pr(px(12.0))
                                .font_family(MONO_FAMILY)
                                .text_size(px(11.0))
                                .text_color(theme.text_ghost)
                                .child(num_str)
                                .into_any_element()
                        };

                        let code_content = if tokens.is_empty() {
                            div()
                                .flex_1()
                                .min_w_0()
                                .font_family(MONO_FAMILY)
                                .text_size(px(12.0))
                                .line_height(px(18.0))
                                .text_color(default_text_color)
                                .child(if line.text.is_empty() {
                                    " ".to_string()
                                } else {
                                    line.text
                                })
                                .into_any_element()
                        } else {
                            render_highlighted_tokens(&line.text, &tokens, default_text_color, &palette)
                                .into_any_element()
                        };

                        div()
                            .flex()
                            .items_start()
                            .min_h(px(20.0))
                            .w_full()
                            .px(px(6.0))
                            .bg(bg)
                            .hover(|s| s.bg(theme.overlay))
                            .child(gutter_view)
                            .child(code_content)
                    }),
            )
            .into_any_element()
    }
}

impl CodeViewerLine {
    fn new_no_or_old(&self) -> Option<usize> {
        self.new_line_no.or(self.old_line_no)
    }
}

fn render_highlighted_tokens(
    text: &str,
    tokens: &[highlight::Token],
    fallback_color: Hsla,
    palette: &Palette,
) -> impl IntoElement {
    let mut spans: Vec<gpui::AnyElement> = Vec::new();
    let mut cursor = 0;

    for token in tokens {
        if token.range.start > cursor {
            let slice = &text[cursor..token.range.start];
            spans.push(
                div()
                    .text_color(fallback_color)
                    .child(slice.to_string())
                    .into_any_element(),
            );
        }
        let token_slice = &text[token.range.clone()];
        let color = palette.token(token.class);
        spans.push(
            div()
                .text_color(color)
                .child(token_slice.to_string())
                .into_any_element(),
        );
        cursor = token.range.end;
    }

    if cursor < text.len() {
        let slice = &text[cursor..];
        spans.push(
            div()
                .text_color(fallback_color)
                .child(slice.to_string())
                .into_any_element(),
        );
    }

    if spans.is_empty() {
        spans.push(
            div()
                .text_color(fallback_color)
                .child(if text.is_empty() { " ".to_string() } else { text.to_string() })
                .into_any_element(),
        );
    }

    div()
        .flex_1()
        .min_w_0()
        .flex()
        .flex_wrap()
        .font_family(MONO_FAMILY)
        .text_size(px(12.0))
        .line_height(px(18.0))
        .children(spans)
}
