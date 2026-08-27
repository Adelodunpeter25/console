//! Full-page Git Diff Viewer component for workspace tabs.

use console_core::{DiffLine, DiffLineKind, DiffResult};
use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, ScrollHandle, SharedString, Styled,
    Window, div, prelude::*, px,
};

use crate::common::copy_button::copy_button;
use crate::markdown::render::MONO_FAMILY;
use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DiffViewer {
    path: String,
    diff: DiffResult,
    raw_diff: String,
    scroll_handle: ScrollHandle,
}

impl DiffViewer {
    pub fn new(path: impl Into<String>, diff: DiffResult, raw_diff: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            diff,
            raw_diff: raw_diff.into(),
            scroll_handle: ScrollHandle::new(),
        }
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let file_name = std::path::Path::new(&self.path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&self.path)
            .to_string();

        let added = self.diff.added;
        let removed = self.diff.removed;
        let copy_btn_id = format!("diff-copy-btn-{}", self.path);

        div()
            .id("diff-viewer-container")
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.canvas)
            // Header bar
            .child(
                div()
                    .h(px(36.0))
                    .flex_none()
                    .flex()
                    .items_center()
                    .justify_between()
                    .px(px(14.0))
                    .border_b_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap(px(8.0))
                            .child(file_icon(file_icon_for_name(&file_name), 15.0))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(theme.text)
                                    .child(self.path.clone()),
                            ),
                    )
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap(px(12.0))
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(6.0))
                                    .child(
                                        div()
                                            .text_size(px(11.0))
                                            .font_weight(FontWeight::SEMIBOLD)
                                            .text_color(theme.success)
                                            .child(format!("+{added}")),
                                    )
                                    .child(
                                        div()
                                            .text_size(px(11.0))
                                            .font_weight(FontWeight::SEMIBOLD)
                                            .text_color(theme.danger)
                                            .child(format!("-{removed}")),
                                    ),
                            )
                            .child(copy_button(copy_btn_id, self.raw_diff.clone(), theme, cx)),
                    ),
            )
            // Diff lines body
            .child(
                div()
                    .id("diff-viewer-body")
                    .flex_1()
                    .w_full()
                    .overflow_y_scroll()
                    .track_scroll(&self.scroll_handle)
                    .py(px(8.0))
                    .child(if self.diff.lines.is_empty() {
                        div()
                            .flex_1()
                            .flex()
                            .items_center()
                            .justify_center()
                            .py(px(48.0))
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .child("No differences detected")
                            .into_any_element()
                    } else {
                        div()
                            .flex()
                            .flex_col()
                            .children(self.diff.lines.iter().map(|line| diff_row(line, &theme)))
                            .into_any_element()
                    }),
            )
    }
}

fn diff_row(line: &DiffLine, theme: &Theme) -> impl IntoElement {
    let (gutter, fg, bg) = match line.kind {
        DiffLineKind::Added => (
            "+",
            theme.success,
            gpui::hsla(145.0 / 360.0, 0.50, 0.66, 0.08),
        ),
        DiffLineKind::Removed => (
            "-",
            theme.danger,
            gpui::hsla(4.0 / 360.0, 0.55, 0.63, 0.08),
        ),
        DiffLineKind::Context => (" ", theme.text_tertiary, gpui::transparent_black()),
    };

    let old_no_str = line.old_no.map_or(String::new(), |n| n.to_string());
    let new_no_str = line.new_no.map_or(String::new(), |n| n.to_string());

    let display_text: SharedString = if line.text.is_empty() {
        " ".into()
    } else {
        line.text.as_str().into()
    };

    div()
        .flex()
        .items_center()
        .h(px(20.0))
        .w_full()
        .px(px(10.0))
        .bg(bg)
        .hover(|s| s.bg(theme.overlay))
        .child(
            div()
                .w(px(32.0))
                .flex_none()
                .text_align(gpui::TextAlign::Right)
                .pr(px(6.0))
                .font_family(MONO_FAMILY)
                .text_size(px(10.0))
                .text_color(theme.text_ghost)
                .child(old_no_str),
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
                .child(new_no_str),
        )
        .child(
            div()
                .w(px(14.0))
                .flex_none()
                .text_size(px(11.0))
                .font_weight(FontWeight::BOLD)
                .text_color(fg)
                .child(gutter),
        )
        .child(
            div()
                .flex_1()
                .min_w_0()
                .font_family(MONO_FAMILY)
                .text_size(px(12.0))
                .text_color(fg)
                .child(display_text),
        )
}
