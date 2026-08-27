//! Full-page File Viewer component for workspace tabs.

use gpui::{App, IntoElement, ListState, RenderOnce, Window};

use super::code_viewer::{CodeViewer, build_file_lines};

#[derive(IntoElement)]
pub struct FileViewer {
    path: String,
    content: String,
    list_state: ListState,
}

impl FileViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>, list_state: ListState) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
            list_state,
        }
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let lines = build_file_lines(&self.path, &self.content);

        CodeViewer::new(format!("file-{}", self.path), self.list_state)
            .lines(lines)
            .empty_message("Empty file")
    }
}
