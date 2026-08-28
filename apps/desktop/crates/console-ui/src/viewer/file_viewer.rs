//! Full-page File Viewer component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, IntoElement, ListState, RenderOnce, Window};

use super::code_viewer::{CodeViewer, SelectionState, build_file_lines};

#[derive(IntoElement)]
pub struct FileViewer {
    path: String,
    content: String,
    list_state: ListState,
    selection_state: Option<Rc<RefCell<SelectionState>>>,
}

impl FileViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>, list_state: ListState) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
            list_state,
            selection_state: None,
        }
    }

    pub fn selection_state(mut self, state: Rc<RefCell<SelectionState>>) -> Self {
        self.selection_state = Some(state);
        self
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let lines = build_file_lines(&self.path, &self.content);

        let mut viewer = CodeViewer::new(format!("file-{}", self.path), self.list_state)
            .lines(lines)
            .empty_message("Empty file");

        if let Some(selection_state) = self.selection_state {
            viewer = viewer.selection_state(selection_state);
        }

        viewer
    }
}
