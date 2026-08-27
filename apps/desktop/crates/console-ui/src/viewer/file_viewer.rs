//! Full-page File Viewer component for workspace tabs.

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, ScrollHandle, Styled, Window, div,
    prelude::*, px,
};

use crate::common::copy_button::copy_button;
use crate::markdown::render::MONO_FAMILY;
use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct FileViewer {
    path: String,
    content: String,
    scroll_handle: ScrollHandle,
}

impl FileViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
            scroll_handle: ScrollHandle::new(),
        }
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let file_name = std::path::Path::new(&self.path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&self.path)
            .to_string();

        let lines: Vec<&str> = self.content.lines().collect();
        let total_lines = lines.len();
        let line_num_width = format!("{}", total_lines.max(1)).len() * 8 + 24;
        let copy_btn_id = format!("file-copy-btn-{}", self.path);

        div()
            .id("file-viewer-container")
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
                                    .text_size(px(11.0))
                                    .text_color(theme.text_tertiary)
                                    .child(format!("{total_lines} lines")),
                            )
                            .child(copy_button(copy_btn_id, self.content.clone(), theme, cx)),
                    ),
            )
            // Code lines body
            .child(
                div()
                    .id("file-viewer-body")
                    .flex_1()
                    .w_full()
                    .overflow_y_scroll()
                    .track_scroll(&self.scroll_handle)
                    .py(px(8.0))
                    .children(lines.into_iter().enumerate().map(|(idx, line)| {
                        div()
                            .flex()
                            .items_center()
                            .h(px(20.0))
                            .w_full()
                            .hover(|s| s.bg(theme.overlay))
                            .child(
                                div()
                                    .w(px(line_num_width as f32))
                                    .flex_none()
                                    .text_align(gpui::TextAlign::Right)
                                    .pr(px(12.0))
                                    .font_family(MONO_FAMILY)
                                    .text_size(px(11.0))
                                    .text_color(theme.text_ghost)
                                    .child(format!("{}", idx + 1)),
                            )
                            .child(
                                div()
                                    .flex_1()
                                    .min_w_0()
                                    .font_family(MONO_FAMILY)
                                    .text_size(px(12.0))
                                    .text_color(theme.text)
                                    .child(if line.is_empty() {
                                        " ".to_string()
                                    } else {
                                        line.to_string()
                                    }),
                            )
                    })),
            )
    }
}
