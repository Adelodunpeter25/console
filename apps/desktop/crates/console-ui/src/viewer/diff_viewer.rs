//! Full-page Git Diff Viewer component for workspace tabs.

use console_core::{DiffLine, DiffLineKind, DiffResult};
use gpui::{
    App, ElementId, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled,
    Window, div, prelude::*, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DiffViewer {
    #[allow(dead_code)]
    path: String,
    diff: DiffResult,
    #[allow(dead_code)]
    raw_diff: String,
}

impl DiffViewer {
    pub fn new(path: impl Into<String>, diff: DiffResult, raw_diff: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            diff,
            raw_diff: raw_diff.into(),
        }
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        div()
            .id(ElementId::Name(format!("diff-viewer-{}", self.path).into()))
            .size_full()
            .min_h_0()
            .min_w_0()
            .overflow_y_scroll()
            .bg(theme.canvas)
            .py(px(8.0))
            .child(if self.diff.lines.is_empty() {
                div()
                    .size_full()
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
                    .w_full()
                    .children(self.diff.lines.iter().map(|line| diff_row(line, &theme)))
                    .into_any_element()
            })
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
        .items_start()
        .min_h(px(20.0))
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
                .pt(px(1.0))
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
                .pt(px(1.0))
                .font_family(MONO_FAMILY)
                .text_size(px(10.0))
                .text_color(theme.text_ghost)
                .child(new_no_str),
        )
        .child(
            div()
                .w(px(14.0))
                .flex_none()
                .pt(px(1.0))
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
                .line_height(px(18.0))
                .text_color(fg)
                .child(display_text),
        )
}
