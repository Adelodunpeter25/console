//! Full-page Git Diff Viewer component for workspace tabs.

use std::cell::RefCell;
use std::rc::Rc;

use console_core::DiffResult;
use gpui::{App, IntoElement, ListState, RenderOnce, Window};

use super::code_viewer::{CodeViewer, CodeViewerLine, SelectionState, build_diff_lines};
use crate::primitives::scrollbar::ScrollbarState;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DiffViewer {
    path: String,
    diff: DiffResult,
    #[allow(dead_code)]
    raw_diff: String,
    list_state: ListState,
    selection_state: Option<Rc<RefCell<SelectionState>>>,
    scrollbar_state: Option<Rc<ScrollbarState>>,
    rc_lines: Option<Rc<Vec<CodeViewerLine>>>,
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
            scrollbar_state: None,
            rc_lines: None,
        }
    }

    pub fn rc_lines(mut self, lines: Rc<Vec<CodeViewerLine>>) -> Self {
        self.rc_lines = Some(lines);
        self
    }

    pub fn selection_state(mut self, state: Rc<RefCell<SelectionState>>) -> Self {
        self.selection_state = Some(state);
        self
    }

    pub fn scrollbar_state(mut self, state: Rc<ScrollbarState>) -> Self {
        self.scrollbar_state = Some(state);
        self
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let lines = self.rc_lines.unwrap_or_else(|| {
            let theme = Theme::current(cx);
            Rc::new(build_diff_lines(&self.path, &self.diff, &theme))
        });

        let mut viewer = CodeViewer::new(format!("diff-{}", self.path), self.list_state)
            .rc_lines(lines)
            .empty_message("No differences detected");

        if let Some(selection_state) = self.selection_state {
            viewer = viewer.selection_state(selection_state);
        }

        if let Some(scrollbar_state) = self.scrollbar_state {
            viewer = viewer.scrollbar_state(scrollbar_state);
        }

        viewer
    }
}
