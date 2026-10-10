//! Line-level diff view for file-edit tool calls.
//!
//! Renders a [`DiffResult`] as colored +/- lines inside a scrollable block,
//! matching the visual language of the existing `section()` helper in
//! `toolcalls.rs`. Each line gets a gutter character (`+` / `-` / space),
//! an optional line-number column, and a colored background wash.

use std::rc::Rc;

use console_core::{DiffLine, DiffLineKind, DiffResult};
use gpui::{
    App, ElementId, FontWeight, IntoElement, ParentElement, RenderOnce, ScrollHandle, SharedString,
    Styled, StyledText, TextRun, Window, div, font, prelude::*, px,
};

use crate::markdown::highlight::{lang_for_tag, lang_tag_for_path};
use crate::markdown::render::{MONO_FAMILY, Palette, code_runs};
use crate::primitives::{base_name, file_type_icon};
use crate::theme::Theme;

const MAX_RENDER_LINES: usize = 500;

/// A scrollable, colored diff block.
#[derive(IntoElement)]
pub struct DiffView {
    id: String,
    diff: DiffResult,
    /// Target file path, when the diff came from a file tool call. Renders the
    /// file's type icon and basename in the header instead of a bare label.
    file_path: Option<String>,
    scroll_handle: ScrollHandle,
    /// When true, renders all lines without max-height or truncation caps (used in review tabs).
    full_height: bool,
    /// When true, suppresses the internal header row (file icon, name, summary) if parent handles it.
    hide_header: bool,
    /// Per-line syntax-highlight runs from [`highlight_diff`]. `None` keeps
    /// the plain add/remove coloring.
    highlights: Option<Rc<Vec<Vec<TextRun>>>>,
}

/// Syntax-highlight a diff's lines for the file at `path`. Removed lines are
/// highlighted as the old file's text and added/context lines as the new
/// file's, so multi-line strings and comments color correctly. Returns `None`
/// when the file's language is unknown.
pub fn highlight_diff(
    diff: &DiffResult,
    path: &str,
    theme: &Theme,
) -> Option<Rc<Vec<Vec<TextRun>>>> {
    let lang = lang_for_tag(lang_tag_for_path(path)?)?;
    let palette = Palette::from_theme(theme);
    let mut code_font = font(MONO_FAMILY);
    code_font.weight = FontWeight::NORMAL;

    let mut old_side = String::new();
    let mut new_side = String::new();
    // (is_old_side, start, end) of each rendered line within its side.
    let mut spans: Vec<(bool, usize, usize)> = Vec::new();
    for line in diff.lines.iter().take(MAX_RENDER_LINES) {
        let is_old = line.kind == DiffLineKind::Removed;
        let side = if is_old { &mut old_side } else { &mut new_side };
        let start = side.len();
        side.push_str(&line.text);
        spans.push((is_old, start, side.len()));
        side.push('\n');
    }

    let old_runs = code_runs(&old_side, Some(lang), &code_font, &palette);
    let new_runs = code_runs(&new_side, Some(lang), &code_font, &palette);
    Some(Rc::new(
        spans
            .into_iter()
            .map(|(is_old, start, end)| {
                clip_runs(if is_old { &old_runs } else { &new_runs }, start, end)
            })
            .collect(),
    ))
}

/// The part of `runs` (which tile one string) covering bytes `start..end`.
fn clip_runs(runs: &[TextRun], start: usize, end: usize) -> Vec<TextRun> {
    let mut out = Vec::new();
    let mut position = 0;
    for run in runs {
        let run_start = position;
        position += run.len;
        if position <= start {
            continue;
        }
        if run_start >= end {
            break;
        }
        let len = position.min(end) - run_start.max(start);
        if len > 0 {
            let mut clipped = run.clone();
            clipped.len = len;
            out.push(clipped);
        }
    }
    out
}

impl DiffView {
    pub fn new(id: impl Into<String>, diff: DiffResult) -> Self {
        Self {
            id: id.into(),
            diff,
            file_path: None,
            scroll_handle: ScrollHandle::new(),
            full_height: false,
            hide_header: false,
            highlights: None,
        }
    }

    pub fn file_path(mut self, path: impl Into<String>) -> Self {
        self.file_path = Some(path.into());
        self
    }

    pub fn full_height(mut self, full_height: bool) -> Self {
        self.full_height = full_height;
        self
    }

    pub fn hide_header(mut self, hide_header: bool) -> Self {
        self.hide_header = hide_header;
        self
    }

    /// Use a caller-owned scroll handle so the scroll position survives
    /// re-renders (a fresh handle is created per render otherwise).
    pub fn scroll_handle(mut self, handle: ScrollHandle) -> Self {
        self.scroll_handle = handle;
        self
    }

    /// Syntax-highlight the code text with runs from [`highlight_diff`].
    pub fn highlights(mut self, highlights: Option<Rc<Vec<Vec<TextRun>>>>) -> Self {
        self.highlights = highlights;
        self
    }
}

impl RenderOnce for DiffView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let added = self.diff.added;
        let removed = self.diff.removed;
        let file_path = self.file_path.clone();

        // Summary badge: +N -M
        let summary = div()
            .flex()
            .items_center()
            .gap(px(8.0))
            .child(
                div()
                    .text_size(px(10.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.success)
                    .child(format!("+{added}")),
            )
            .child(
                div()
                    .text_size(px(10.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.danger)
                    .child(format!("-{removed}")),
            );

        let (lines, truncated) = if self.full_height {
            (self.diff.lines.iter().collect::<Vec<_>>(), false)
        } else {
            (
                self.diff.lines.iter().take(MAX_RENDER_LINES).collect::<Vec<_>>(),
                self.diff.lines.len() > MAX_RENDER_LINES,
            )
        };

        let body = div()
            .id(ElementId::Name(format!("diff-body-{}", self.id).into()))
            .when(!self.full_height, |el| el.max_h(px(240.0)).overflow_y_scroll().track_scroll(&self.scroll_handle))
            .rounded(px(5.0))
            .bg(theme.inset)
            .py(px(4.0))
            .children(lines.iter().enumerate().map(|(index, line)| {
                let runs = self
                    .highlights
                    .as_ref()
                    .and_then(|all| all.get(index))
                    .filter(|runs| !runs.is_empty());
                diff_line_row(line, runs, &theme)
            }))
            .when(truncated, |el| {
                el.child(
                    div()
                        .px(px(10.0))
                        .py(px(3.0))
                        .text_size(px(10.0))
                        .text_color(theme.text_ghost)
                        .child(format!(
                            "… {} more lines not shown",
                            self.diff.lines.len() - MAX_RENDER_LINES
                        )),
                )
            });

        let show_header = !self.hide_header;

        div()
            .flex()
            .flex_col()
            .gap(px(4.0))
            .when(show_header, |el| {
                el.child(
                    div()
                        .flex()
                        .items_center()
                        .justify_between()
                        .child(match &file_path {
                            Some(path) => div()
                                .flex()
                                .items_center()
                                .gap(px(6.0))
                                .min_w_0()
                                .child(file_type_icon(path, 13.0))
                                .child(
                                    div()
                                        .font_family(MONO_FAMILY)
                                        .text_size(px(10.5))
                                        .font_weight(FontWeight::MEDIUM)
                                        .text_color(theme.text_secondary)
                                        .truncate()
                                        .child(base_name(path).to_owned()),
                                ),
                            None => div()
                                .text_size(px(10.0))
                                .font_weight(FontWeight::SEMIBOLD)
                                .text_color(theme.text_ghost)
                                .child("DIFF"),
                        })
                        .child(summary),
                )
            })
            .child(body)
    }
}

fn diff_line_row(line: &DiffLine, runs: Option<&Vec<TextRun>>, theme: &Theme) -> impl IntoElement {
    let (gutter, fg, bg) = match line.kind {
        DiffLineKind::Added => (
            "+",
            theme.success,
            gpui::hsla(145.0 / 360.0, 0.50, 0.66, 0.08),
        ),
        DiffLineKind::Removed => ("-", theme.danger, gpui::hsla(4.0 / 360.0, 0.55, 0.63, 0.08)),
        DiffLineKind::Context => (" ", theme.text_tertiary, gpui::transparent_black()),
    };

    let line_no = match line.kind {
        DiffLineKind::Added => line.new_no,
        DiffLineKind::Removed => line.old_no,
        DiffLineKind::Context => line.new_no,
    };

    let display_text: SharedString = if line.text.is_empty() {
        " ".into()
    } else {
        line.text.as_str().into()
    };

    div()
        .flex()
        .items_center()
        .w_full()
        .px(px(10.0))
        .bg(bg)
        .child(
            div()
                .w(px(32.0))
                .flex_none()
                .text_size(px(9.5))
                .font_family(MONO_FAMILY)
                .text_color(theme.text_ghost)
                .child(line_no.map_or(String::new(), |n| n.to_string())),
        )
        .child(
            div()
                .w(px(14.0))
                .flex_none()
                .text_size(px(10.5))
                .font_weight(FontWeight::BOLD)
                .text_color(fg)
                .child(gutter),
        )
        .child(
            div()
                .min_w_0()
                .flex_1()
                .font_family(MONO_FAMILY)
                .text_size(px(12.0))
                .line_height(px(17.0))
                .text_color(fg)
                .child(match runs {
                    // Highlighted text keeps its token colors; the row's
                    // green/red wash and gutter sign still mark the change.
                    Some(runs) => StyledText::new(display_text)
                        .with_runs(runs.clone())
                        .into_any_element(),
                    None => display_text.into_any_element(),
                }),
        )
}
