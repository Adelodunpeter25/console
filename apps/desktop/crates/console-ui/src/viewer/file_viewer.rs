use std::rc::Rc;

use gpui::{App, Entity, FocusHandle, IntoElement, ListState, RenderOnce, Window};

use super::code_viewer::{CodeViewer, CodeViewerLine, SelectionState, build_file_lines};
use crate::primitives::scrollbar::ScrollbarState;

#[derive(IntoElement)]
pub struct FileViewer {
    path: String,
    content: String,
    list_state: ListState,
    selection_state: Option<Entity<SelectionState>>,
    scrollbar_state: Option<Rc<ScrollbarState>>,
    focus_handle: Option<FocusHandle>,
    rc_lines: Option<Rc<Vec<CodeViewerLine>>>,
}

impl FileViewer {
    pub fn new(path: impl Into<String>, content: impl Into<String>, list_state: ListState) -> Self {
        Self {
            path: path.into(),
            content: content.into(),
            list_state,
            selection_state: None,
            scrollbar_state: None,
            focus_handle: None,
            rc_lines: None,
        }
    }

    pub fn rc_lines(mut self, lines: Rc<Vec<CodeViewerLine>>) -> Self {
        self.rc_lines = Some(lines);
        self
    }

    pub fn selection_state(mut self, state: Entity<SelectionState>) -> Self {
        self.selection_state = Some(state);
        self
    }

    pub fn scrollbar_state(mut self, state: Rc<ScrollbarState>) -> Self {
        self.scrollbar_state = Some(state);
        self
    }

    pub fn focus_handle(mut self, handle: FocusHandle) -> Self {
        self.focus_handle = Some(handle);
        self
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let lines = self
            .rc_lines
            .unwrap_or_else(|| Rc::new(build_file_lines(&self.path, &self.content)));

        let mut viewer = CodeViewer::new(format!("file-{}", self.path), self.list_state)
            .rc_lines(lines)
            .empty_message("Empty file");

        if let Some(selection_state) = self.selection_state {
            viewer = viewer.selection_state(selection_state);
        }

        if let Some(scrollbar_state) = self.scrollbar_state {
            viewer = viewer.scrollbar_state(scrollbar_state);
        }

        if let Some(focus_handle) = self.focus_handle {
            viewer = viewer.focus_handle(focus_handle);
        }

        viewer
    }
}
