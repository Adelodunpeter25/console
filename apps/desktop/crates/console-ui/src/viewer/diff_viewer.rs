//! Full-page Git Diff Viewer component for workspace tabs.

use console_core::{DiffLineKind, DiffResult};
use gpui::{App, IntoElement, RenderOnce, Window};

use super::code_viewer::{CodeViewer, CodeViewerLine};
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

        let lines: Vec<CodeViewerLine> = self
            .diff
            .lines
            .into_iter()
            .map(|line| {
                let (gutter, gutter_color, bg_color) = match line.kind {
                    DiffLineKind::Added => (
                        "+",
                        theme.success,
                        Some(gpui::hsla(145.0 / 360.0, 0.50, 0.66, 0.08)),
                    ),
                    DiffLineKind::Removed => (
                        "-",
                        theme.danger,
                        Some(gpui::hsla(4.0 / 360.0, 0.55, 0.63, 0.08)),
                    ),
                    DiffLineKind::Context => (" ", theme.text_tertiary, None),
                };

                CodeViewerLine {
                    line_no: None,
                    old_line_no: line.old_no,
                    new_line_no: line.new_no,
                    gutter: Some(gutter),
                    gutter_color: Some(gutter_color),
                    bg_color,
                    text_color: None,
                    text: line.text,
                }
            })
            .collect();

        CodeViewer::new(format!("diff-{}", self.path))
            .for_path(self.path)
            .lines(lines)
            .empty_message("No differences detected")
    }
}
