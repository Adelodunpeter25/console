//! Line-level diff view for file-edit tool calls.
//!
//! Renders a [`DiffResult`] as colored +/- lines inside a scrollable block,
//! matching the visual language of the existing `section()` helper in
//! `toolcalls.rs`. Each line gets a gutter character (`+` / `-` / space),
//! an optional line-number column, and a colored background wash.

use console_core::{DiffLine, DiffLineKind, DiffResult};
use gpui::{
    App, ElementId, FontWeight, IntoElement, ParentElement, RenderOnce, ScrollHandle, SharedString,
    Styled, Window, div, prelude::*, px,
};

use crate::markdown::render::MONO_FAMILY;
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
}

impl DiffView {
    pub fn new(id: impl Into<String>, diff: DiffResult) -> Self {
        Self {
            id: id.into(),
            diff,
            file_path: None,
            scroll_handle: ScrollHandle::new(),
        }
    }

    pub fn file_path(mut self, path: impl Into<String>) -> Self {
        self.file_path = Some(path.into());
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

        let lines: Vec<&DiffLine> = self.diff.lines.iter().take(MAX_RENDER_LINES).collect();
        let truncated = self.diff.lines.len() > MAX_RENDER_LINES;

        let body = div()
            .id(ElementId::Name(format!("diff-body-{}", self.id).into()))
            .max_h(px(240.0))
            .overflow_y_scroll()
            .track_scroll(&self.scroll_handle)
            .rounded(px(5.0))
            .bg(theme.inset)
            .py(px(4.0))
            .children(lines.iter().map(|line| diff_line_row(line, &theme)))
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

        div()
            .flex()
            .flex_col()
            .gap(px(4.0))
            .child(
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
            .child(body)
    }
}

fn diff_line_row(line: &DiffLine, theme: &Theme) -> impl IntoElement {
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
                .child(display_text),
        )
}
