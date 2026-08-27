//! Full-page File Viewer component for workspace tabs.

use gpui::{App, IntoElement, RenderOnce, Window};

use super::code_viewer::{CodeViewer, CodeViewerLine};

#[derive(IntoElement)]
pub struct FileViewer {
    path: String,
    content: String,
}

impl FileViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
        }
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let lines: Vec<CodeViewerLine> = self
            .content
            .lines()
            .enumerate()
            .map(|(idx, line)| CodeViewerLine {
                line_no: Some(idx + 1),
                old_line_no: None,
                new_line_no: None,
                gutter: None,
                gutter_color: None,
                bg_color: None,
                text_color: None,
                text: line.to_string(),
            })
            .collect();

        CodeViewer::new(format!("file-{}", self.path))
            .for_path(self.path)
            .lines(lines)
            .empty_message("Empty file")
    }
}
