//! Full-page File Viewer component for workspace tabs.

use gpui::{App, IntoElement, RenderOnce, Window};

use super::code_viewer::{CodeViewer, build_file_lines};

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
        let lines = build_file_lines(&self.path, &self.content);

        CodeViewer::new(format!("file-{}", self.path))
            .lines(lines)
            .empty_message("Empty file")
    }
}
