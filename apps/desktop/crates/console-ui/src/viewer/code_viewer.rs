//! High-performance virtualized Code & Diff Viewer with single-pass TextRun
//! GPU text shaping, syntax highlighting, text selection, and virtualized list scrolling.

use std::cell::RefCell;
use std::cmp::{max, min};
use std::rc::Rc;

use gpui::{
    App, ClipboardItem, DispatchPhase, ElementId, FocusHandle, Font, FontWeight, Hsla,
    InteractiveElement, IntoElement, ListState, MouseButton, MouseMoveEvent, MouseUpEvent,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, StyledText, TextRun, Window,
    actions, canvas, div, list, prelude::*, px,
};

use crate::markdown::highlight::{self, Carry, lang_for_tag, lang_tag_for_path};
use crate::markdown::render::{MONO_FAMILY, Palette};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::theme::Theme;

pub const CODE_LINE_HEIGHT: f32 = 18.0;
pub const CHAR_WIDTH: f32 = 6.6;

actions!(code_viewer, [CopySelection, SelectAll]);

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct CodePosition {
    pub line: usize,
    pub col: usize,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CodeSelection {
    pub anchor: CodePosition,
    pub head: CodePosition,
}

impl CodeSelection {
    pub fn new(anchor: CodePosition, head: CodePosition) -> Self {
        Self { anchor, head }
    }

    pub fn start(&self) -> CodePosition {
        min(self.anchor, self.head)
    }

    pub fn end(&self) -> CodePosition {
        max(self.anchor, self.head)
    }

    pub fn is_empty(&self) -> bool {
        self.anchor == self.head
    }

    pub fn line_col_range(&self, line_idx: usize, line_len: usize) -> Option<(usize, usize)> {
        let start = self.start();
        let end = self.end();

        if line_idx < start.line || line_idx > end.line || self.is_empty() {
            return None;
        }

        let start_col = if line_idx == start.line {
            min(start.col, line_len)
        } else {
            0
        };

        let end_col = if line_idx == end.line {
            min(end.col, line_len)
        } else {
            line_len
        };

        if start_col <= end_col {
            Some((start_col, end_col))
        } else {
            None
        }
    }
}

#[derive(Default)]
pub struct SelectionState {
    pub selection: Option<CodeSelection>,
    pub is_dragging: bool,
}

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
    selection_state: Rc<RefCell<SelectionState>>,
    scrollbar_state: Option<Rc<ScrollbarState>>,
    empty_message: Option<String>,
    focus_handle: Option<FocusHandle>,
}

impl CodeViewer {
    pub fn new(id: impl Into<String>, list_state: ListState) -> Self {
        Self {
            id: id.into(),
            lines: Rc::new(Vec::new()),
            list_state,
            selection_state: Rc::new(RefCell::new(SelectionState::default())),
            scrollbar_state: None,
            empty_message: None,
            focus_handle: None,
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

    pub fn selection_state(mut self, selection_state: Rc<RefCell<SelectionState>>) -> Self {
        self.selection_state = selection_state;
        self
    }

    pub fn scrollbar_state(mut self, scrollbar_state: Rc<ScrollbarState>) -> Self {
        self.scrollbar_state = Some(scrollbar_state);
        self
    }

    pub fn focus_handle(mut self, focus_handle: FocusHandle) -> Self {
        self.focus_handle = Some(focus_handle);
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

fn copy_selection_to_clipboard(lines: &[CodeViewerLine], selection: CodeSelection, cx: &mut App) {
    if selection.is_empty() {
        return;
    }
    let start = selection.start();
    let end = selection.end();

    let mut result = String::new();
    for line_idx in start.line..=min(end.line, lines.len().saturating_sub(1)) {
        let line_text = &lines[line_idx].text;
        let line_len = line_text.len();

        let s_col = if line_idx == start.line {
            min(start.col, line_len)
        } else {
            0
        };

        let e_col = if line_idx == end.line {
            min(end.col, line_len)
        } else {
            line_len
        };

        if s_col <= e_col && s_col <= line_len {
            let slice = &line_text[s_col..min(e_col, line_len)];
            result.push_str(slice);
        }

        if line_idx < end.line {
            result.push('\n');
        }
    }

    if !result.is_empty() {
        cx.write_to_clipboard(ClipboardItem::new_string(result));
    }
}

impl RenderOnce for CodeViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let palette = Palette::from_theme(&theme);

        if self.lines.is_empty() {
            return div()
                .id(ElementId::Name(
                    format!("code-viewer-empty-{}", self.id).into(),
                ))
                .size_full()
                .flex()
                .items_center()
                .justify_center()
                .bg(theme.canvas)
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child(self.empty_message.unwrap_or_else(|| "Empty".to_string()))
                .into_any_element();
        }

        // Determine max line number for gutter sizing (O(1) from line count)
        let max_line = self.lines.len().max(1);
        let line_num_width = format!("{max_line}").len() * 7 + 18;

        let has_dual_line_numbers = self
            .lines
            .first()
            .map(|l| l.old_line_no.is_some() || l.new_line_no.is_some())
            .unwrap_or(false);

        let code_font = Font {
            family: MONO_FAMILY.into(),
            features: Default::default(),
            weight: FontWeight::NORMAL,
            style: Default::default(),
            ..Default::default()
        };

        let gutter_offset = if has_dual_line_numbers {
            28.0 + 28.0 + 12.0 + 12.0
        } else {
            line_num_width as f32 + 12.0
        };

        let max_cols = self.lines.iter().map(|l| l.text.len()).max().unwrap_or(80);
        let content_width = gutter_offset + (max_cols as f32 * CHAR_WIDTH) + 64.0;
        let list_state_for_items = self.list_state.clone();

        let lines_rc = self.lines.clone();
        let selection_rc = self.selection_state.clone();
        let selection_for_copy = self.selection_state.clone();
        let lines_for_copy = self.lines.clone();
        let selection_for_mouse_up = self.selection_state.clone();

        let selection_bg = theme.selection;

        // Window-level mouse tracking runs during the paint phase via canvas, satisfying
        // GPUI's requirement that window.on_mouse_event may only be called during paint.
        let mouse_tracker = {
            let selection_state = self.selection_state.clone();
            let list_state = self.list_state.clone();
            let lines_len = self.lines.len();

            canvas(
                |_, _, _| (),
                move |_bounds, _, window, _| {
                    window.on_mouse_event({
                        let selection_state = selection_state.clone();
                        let list_state = list_state.clone();
                        move |event: &MouseMoveEvent, phase, window, _cx| {
                            if phase == DispatchPhase::Bubble {
                                let mut state = selection_state.borrow_mut();
                                if state.is_dragging {
                                    if let Some(mut sel) = state.selection {
                                        let viewport = list_state.viewport_bounds();
                                        let scroll_offset =
                                            -list_state.scroll_px_offset_for_scrollbar().y;
                                        let rel_y =
                                            event.position.y - viewport.origin.y + scroll_offset;
                                        let line_idx = if rel_y > px(0.0) {
                                            ((rel_y / px(CODE_LINE_HEIGHT)).floor() as usize)
                                                .min(lines_len.saturating_sub(1))
                                        } else {
                                            0
                                        };
                                        let rel_x = f32::from(event.position.x - viewport.origin.x)
                                            - gutter_offset;
                                        let col = if rel_x > 0.0 {
                                            (rel_x / CHAR_WIDTH).round() as usize
                                        } else {
                                            0
                                        };
                                        let target_pos = CodePosition {
                                            line: line_idx,
                                            col,
                                        };
                                        if sel.head != target_pos {
                                            sel.head = target_pos;
                                            state.selection = Some(sel);
                                            window.refresh();
                                        }
                                    }
                                }
                            }
                        }
                    });

                    window.on_mouse_event({
                        let selection_state = selection_state.clone();
                        move |_: &MouseUpEvent, phase, window, _cx| {
                            if phase == DispatchPhase::Bubble {
                                let mut state = selection_state.borrow_mut();
                                if state.is_dragging {
                                    state.is_dragging = false;
                                    window.refresh();
                                }
                            }
                        }
                    });
                },
            )
            .absolute()
            .w(px(0.0))
            .h(px(0.0))
        };

        let mut container = div()
            .id(ElementId::Name(
                format!("code-viewer-container-{}", self.id).into(),
            ))
            .size_full()
            .min_h_0()
            .min_w_0()
            .overflow_x_scroll()
            .bg(theme.canvas)
            .on_mouse_up(MouseButton::Left, move |_, window, _cx| {
                let mut state = selection_for_mouse_up.borrow_mut();
                if state.is_dragging {
                    state.is_dragging = false;
                    window.refresh();
                }
            })
            .on_action(move |_: &CopySelection, _window, cx| {
                let state = selection_for_copy.borrow();
                if let Some(sel) = state.selection {
                    copy_selection_to_clipboard(&lines_for_copy, sel, cx);
                }
            });

        if let Some(focus_handle) = self.focus_handle {
            container = container.track_focus(&focus_handle);
        }

        let scrollbar = self
            .scrollbar_state
            .as_ref()
            .map(|s| scrollbar::vertical(&self.list_state, s));

        div()
            .id(ElementId::Name(
                format!("code-viewer-root-{}", self.id).into(),
            ))
            .relative()
            .size_full()
            .min_h_0()
            .min_w_0()
            .bg(theme.canvas)
            .child(mouse_tracker)
            .child(
                container.child(
                    div()
                        .relative()
                        .min_w(px(content_width))
                        .w_full()
                        .h_full()
                        .min_h_0()
                        .child(
                            list(self.list_state, move |index, _window, _cx| {
                                let Some(line) = lines_rc.get(index) else {
                                    return div().into_any_element();
                                };

                                let bg = line.bg_color.unwrap_or(gpui::transparent_black());
                                let default_text_color = line.text_color.unwrap_or(theme.text);

                                let gutter_view = if has_dual_line_numbers {
                                    let old_str =
                                        line.old_line_no.map_or(String::new(), |n| n.to_string());
                                    let new_str =
                                        line.new_line_no.map_or(String::new(), |n| n.to_string());
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
                                            let fg =
                                                line.gutter_color.unwrap_or(default_text_color);
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
                                    let num_str =
                                        line.line_no.map_or(String::new(), |n| n.to_string());
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

                                let line_sel_range = selection_rc
                                    .borrow()
                                    .selection
                                    .and_then(|sel| sel.line_col_range(index, line.text.len()));

                                let runs = code_runs_for_tokens(
                                    display_str,
                                    &line.tokens,
                                    default_text_color,
                                    &code_font,
                                    &palette,
                                    line_sel_range,
                                    selection_bg,
                                );

                                let sel_mouse_down = selection_rc.clone();
                                let sel_mouse_move = selection_rc.clone();
                                let ls_down = list_state_for_items.clone();
                                let ls_move = list_state_for_items.clone();
                                let line_len = line.text.len();

                                div()
                                    .id(ElementId::Name(format!("code-row-{}", index).into()))
                                    .flex()
                                    .items_center()
                                    .h(px(CODE_LINE_HEIGHT))
                                    .min_w_full()
                                    .w_auto()
                                    .px(px(6.0))
                                    .bg(bg)
                                    .cursor_text()
                                    .on_mouse_down(MouseButton::Left, move |event, window, _cx| {
                                        let x = f32::from(
                                            event.position.x - ls_down.viewport_bounds().origin.x,
                                        ) - gutter_offset;
                                        let col = if x > 0.0 {
                                            min((x / CHAR_WIDTH).round() as usize, line_len)
                                        } else {
                                            0
                                        };
                                        let mut state = sel_mouse_down.borrow_mut();
                                        let pos = CodePosition { line: index, col };
                                        state.selection = Some(CodeSelection::new(pos, pos));
                                        state.is_dragging = true;
                                        window.refresh();
                                    })
                                    .on_mouse_move(move |event, window, _cx| {
                                        let mut state = sel_mouse_move.borrow_mut();
                                        if state.is_dragging {
                                            if let Some(mut sel) = state.selection {
                                                let x = f32::from(
                                                    event.position.x
                                                        - ls_move.viewport_bounds().origin.x,
                                                ) - gutter_offset;
                                                let col = if x > 0.0 {
                                                    min((x / CHAR_WIDTH).round() as usize, line_len)
                                                } else {
                                                    0
                                                };
                                                let target_pos = CodePosition { line: index, col };
                                                if sel.head != target_pos {
                                                    sel.head = target_pos;
                                                    state.selection = Some(sel);
                                                    window.refresh();
                                                }
                                            }
                                        }
                                    })
                                    .child(gutter_view)
                                    .child(
                                        div()
                                            .flex_none()
                                            .font_family(MONO_FAMILY)
                                            .text_size(px(11.0))
                                            .line_height(px(CODE_LINE_HEIGHT))
                                            .whitespace_nowrap()
                                            .child(
                                                StyledText::new(display_str.to_string())
                                                    .with_runs(runs),
                                            ),
                                    )
                                    .into_any_element()
                            })
                            .flex_1()
                            .size_full(),
                        ),
                ),
            )
            .children(scrollbar)
            .into_any_element()
    }
}

fn code_runs_for_tokens(
    text: &str,
    tokens: &[highlight::Token],
    default_color: Hsla,
    code_font: &Font,
    palette: &Palette,
    selection_range: Option<(usize, usize)>,
    selection_bg: Hsla,
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
        runs = vec![TextRun {
            len: text.len(),
            font: code_font.clone(),
            color: default_color,
            background_color: None,
            underline: None,
            strikethrough: None,
        }];
    }

    // Apply selection highlighting if this line overlaps active selection
    if let Some((sel_start, sel_end)) = selection_range {
        if sel_start < sel_end && sel_start < text.len() {
            let mut highlighted_runs = Vec::new();
            let mut current_offset = 0;

            for run in runs {
                let run_start = current_offset;
                let run_end = current_offset + run.len;
                current_offset = run_end;

                if run_end <= sel_start || run_start >= sel_end {
                    // Entirely outside selection
                    highlighted_runs.push(run);
                } else if run_start >= sel_start && run_end <= sel_end {
                    // Entirely inside selection
                    let mut sel_run = run;
                    sel_run.background_color = Some(selection_bg);
                    highlighted_runs.push(sel_run);
                } else {
                    // Partially overlapping selection - split run
                    let overlap_start = max(run_start, sel_start);
                    let overlap_end = min(run_end, sel_end);

                    if run_start < overlap_start {
                        let mut before = run.clone();
                        before.len = overlap_start - run_start;
                        highlighted_runs.push(before);
                    }

                    let mut inside = run.clone();
                    inside.len = overlap_end - overlap_start;
                    inside.background_color = Some(selection_bg);
                    highlighted_runs.push(inside);

                    if overlap_end < run_end {
                        let mut after = run;
                        after.len = run_end - overlap_end;
                        highlighted_runs.push(after);
                    }
                }
            }
            return highlighted_runs;
        }
    }

    runs
}
