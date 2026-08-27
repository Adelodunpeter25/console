//! High-performance virtualized Code & Diff Viewer with single-pass TextRun
//! GPU text shaping, syntax highlighting, and virtualized list scrolling.

use std::rc::Rc;

use gpui::{
    App, ElementId, Font, FontWeight, Hsla, IntoElement, ListState, ParentElement, RenderOnce,
    Styled, StyledText, TextRun, Window, div, list, prelude::*, px,
};

use crate::markdown::highlight::{self, Carry, lang_for_tag, lang_tag_for_path};
use crate::markdown::render::{MONO_FAMILY, Palette};
use crate::theme::Theme;

pub const CODE_LINE_HEIGHT: f32 = 18.0;

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
    pub tokens: Vec<highlight::Token>,
}

#[derive(IntoElement)]
pub struct CodeViewer {
    id: String,
    lines: Rc<Vec<CodeViewerLine>>,
    list_state: ListState,
    empty_message: Option<String>,
}

impl CodeViewer {
    pub fn new(id: impl Into<String>, list_state: ListState) -> Self {
        Self {
            id: id.into(),
            lines: Rc::new(Vec::new()),
            list_state,
            empty_message: None,
        }
    }

    pub fn lines(mut self, lines: Vec<CodeViewerLine>) -> Self {
        self.lines = Rc::new(lines);
        self
    }

    pub fn rc_lines(mut self, lines: Rc<Vec<CodeViewerLine>>) -> Self {
        self.lines = lines;
        self
    }

    pub fn empty_message(mut self, msg: impl Into<String>) -> Self {
        self.empty_message = Some(msg.into());
        self
    }
}

/// Helper to tokenize a slice of code lines once for a given file path.
pub fn build_file_lines(path: &str, content: &str) -> Vec<CodeViewerLine> {
    let lang = lang_tag_for_path(path).and_then(lang_for_tag);
    let mut carry = Carry::None;

    content
        .lines()
        .enumerate()
        .map(|(idx, line)| {
            let tokens = if let Some(l) = lang {
                let (t, next) = highlight::tokenize_line(l, line, carry);
                carry = next;
                t
            } else {
                Vec::new()
            };

            CodeViewerLine {
                line_no: Some(idx + 1),
                old_line_no: None,
                new_line_no: None,
                gutter: None,
                gutter_color: None,
                bg_color: None,
                text_color: None,
                text: line.to_string(),
                tokens,
            }
        })
        .collect()
}

/// Helper to tokenize diff lines once for a given file path.
pub fn build_diff_lines(
    path: &str,
    diff: &console_core::DiffResult,
    theme: &Theme,
) -> Vec<CodeViewerLine> {
    let lang = lang_tag_for_path(path).and_then(lang_for_tag);
    let mut carry = Carry::None;

    diff.lines
        .iter()
        .map(|line| {
            let tokens = if let Some(l) = lang {
                let (t, next) = highlight::tokenize_line(l, &line.text, carry);
                carry = next;
                t
            } else {
                Vec::new()
            };

            let (gutter, gutter_color, bg_color) = match line.kind {
                console_core::DiffLineKind::Added => (
                    "+",
                    theme.success,
                    Some(gpui::hsla(145.0 / 360.0, 0.50, 0.66, 0.08)),
                ),
                console_core::DiffLineKind::Removed => (
                    "-",
                    theme.danger,
                    Some(gpui::hsla(4.0 / 360.0, 0.55, 0.63, 0.08)),
                ),
                console_core::DiffLineKind::Context => (" ", theme.text_tertiary, None),
            };

            CodeViewerLine {
                line_no: None,
                old_line_no: line.old_no,
                new_line_no: line.new_no,
                gutter: Some(gutter),
                gutter_color: Some(gutter_color),
                bg_color,
                text_color: None,
                text: line.text.clone(),
                tokens,
            }
        })
        .collect()
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
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child(self.empty_message.unwrap_or_else(|| "Empty".to_string()))
                .into_any_element();
        }

        // Determine max line number for gutter sizing
        let max_line = self
            .lines
            .iter()
            .map(|l| l.line_no.or(l.new_no_or_old()).unwrap_or(0))
            .max()
            .unwrap_or(1);
        let line_num_width = format!("{max_line}").len() * 7 + 18;

        let has_dual_line_numbers = self
            .lines
            .iter()
            .any(|l| l.old_line_no.is_some() || l.new_line_no.is_some());

        let code_font = Font {
            family: MONO_FAMILY.into(),
            features: Default::default(),
            weight: FontWeight::NORMAL,
            style: Default::default(),
            ..Default::default()
        };

        let lines_rc = self.lines.clone();

        div()
            .id(ElementId::Name(format!("code-viewer-container-{}", self.id).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .bg(theme.canvas)
            .child(
                list(self.list_state, move |index, _window, _cx| {
                    let Some(line) = lines_rc.get(index) else {
                        return div().into_any_element();
                    };

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
                                    .w(px(28.0))
                                    .flex_none()
                                    .text_align(gpui::TextAlign::Right)
                                    .pr(px(5.0))
                                    .font_family(MONO_FAMILY)
                                    .text_size(px(9.5))
                                    .text_color(theme.text_ghost)
                                    .child(old_str),
                            )
                            .child(
                                div()
                                    .w(px(28.0))
                                    .flex_none()
                                    .text_align(gpui::TextAlign::Right)
                                    .pr(px(6.0))
                                    .font_family(MONO_FAMILY)
                                    .text_size(px(9.5))
                                    .text_color(theme.text_ghost)
                                    .child(new_str),
                            )
                            .when_some(line.gutter, |el, g| {
                                let fg = line.gutter_color.unwrap_or(default_text_color);
                                el.child(
                                    div()
                                        .w(px(12.0))
                                        .flex_none()
                                        .text_size(px(10.5))
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
                            .pr(px(10.0))
                            .font_family(MONO_FAMILY)
                            .text_size(px(10.0))
                            .text_color(theme.text_ghost)
                            .child(num_str)
                            .into_any_element()
                    };

                    let display_str = if line.text.is_empty() {
                        " "
                    } else {
                        line.text.as_str()
                    };

                    let runs = code_runs_for_tokens(
                        display_str,
                        &line.tokens,
                        default_text_color,
                        &code_font,
                        &palette,
                    );

                    div()
                        .id(ElementId::Name(format!("code-row-{}", index).into()))
                        .flex()
                        .items_center()
                        .h(px(CODE_LINE_HEIGHT))
                        .w_full()
                        .px(px(6.0))
                        .bg(bg)
                        .child(gutter_view)
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .font_family(MONO_FAMILY)
                                .text_size(px(11.0))
                                .line_height(px(CODE_LINE_HEIGHT))
                                .whitespace_nowrap()
                                .child(StyledText::new(display_str.to_string()).with_runs(runs)),
                        )
                        .into_any_element()
                })
                .flex_1()
                .size_full(),
            )
            .into_any_element()
    }
}

impl CodeViewerLine {
    fn new_no_or_old(&self) -> Option<usize> {
        self.new_line_no.or(self.old_line_no)
    }
}

fn code_runs_for_tokens(
    text: &str,
    tokens: &[highlight::Token],
    default_color: Hsla,
    code_font: &Font,
    palette: &Palette,
) -> Vec<TextRun> {
    if text.is_empty() {
        return Vec::new();
    }

    let mut runs: Vec<TextRun> = Vec::new();
    let push = |runs: &mut Vec<TextRun>, len: usize, color: Hsla| {
        if len == 0 {
            return;
        }
        match runs.last_mut() {
            Some(last) if last.color == color => last.len += len,
            _ => runs.push(TextRun {
                len,
                font: code_font.clone(),
                color,
                background_color: None,
                underline: None,
                strikethrough: None,
            }),
        }
    };

    let mut cursor = 0;
    for token in tokens {
        let start = token.range.start.min(text.len());
        let end = token.range.end.min(text.len());
        if start > cursor {
            push(&mut runs, start - cursor, default_color);
            cursor = start;
        }
        if end > cursor {
            let color = palette.token(token.class);
            push(&mut runs, end - cursor, color);
            cursor = end;
        }
    }

    if cursor < text.len() {
        push(&mut runs, text.len() - cursor, default_color);
    }

    // Strict validator: ensure total run length exactly matches text.len()
    let total_len: usize = runs.iter().map(|r| r.len).sum();
    if total_len != text.len() {
        return vec![TextRun {
            len: text.len(),
            font: code_font.clone(),
            color: default_color,
            background_color: None,
            underline: None,
            strikethrough: None,
        }];
    }

    runs
}
