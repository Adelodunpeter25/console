//! Full-page Git Diff Viewer component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use console_core::DiffResult;
use gpui::{App, IntoElement, ListState, RenderOnce, Window};

use super::code_viewer::{CodeViewer, SelectionState, build_diff_lines};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DiffViewer {
    path: String,
    diff: DiffResult,
    #[allow(dead_code)]
    raw_diff: String,
    list_state: ListState,
    selection_state: Option<Rc<RefCell<SelectionState>>>,
}

impl DiffViewer {
    pub fn new(
        path: impl Into<String>,
        diff: DiffResult,
        raw_diff: impl Into<String>,
        list_state: ListState,
    ) -> Self {
        Self {
            path: path.into(),
            diff,
            raw_diff: raw_diff.into(),
            list_state,
            selection_state: None,
        }
    }

    pub fn selection_state(mut self, state: Rc<RefCell<SelectionState>>) -> Self {
        self.selection_state = Some(state);
        self
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let lines = build_diff_lines(&self.path, &self.diff, &theme);

        let mut viewer = CodeViewer::new(format!("diff-{}", self.path), self.list_state)
            .lines(lines)
            .empty_message("No differences detected");

        if let Some(selection_state) = self.selection_state {
            viewer = viewer.selection_state(selection_state);
        }

        viewer
    }
}
