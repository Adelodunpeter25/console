//! Full-page Git Diff Viewer component for workspace tabs.

use console_core::DiffResult;
use gpui::{App, IntoElement, RenderOnce, Window};

use super::code_viewer::{CodeViewer, build_diff_lines};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DiffViewer {
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
        let lines = build_diff_lines(&self.path, &self.diff, &theme);

        CodeViewer::new(format!("diff-{}", self.path))
            .lines(lines)
            .empty_message("No differences detected")
    }
}
