//! Full-page File Viewer component for workspace tabs.

use gpui::{
    App, IntoElement, ParentElement, RenderOnce, ScrollHandle, Styled, Window, div,
    prelude::*, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct FileViewer {
    #[allow(dead_code)]
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
        let lines: Vec<&str> = self.content.lines().collect();
        let total_lines = lines.len();
        let line_num_width = format!("{}", total_lines.max(1)).len() * 8 + 24;

        div()
            .id("file-viewer-container")
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.canvas)
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
