//! Full-page Git Diff Viewer component for workspace tabs.

use gpui::{App, Entity, IntoElement, RenderOnce, Window, div, prelude::*};

#[derive(IntoElement)]
pub struct DiffViewer {
    view: Entity<editor_ui::DiffView>,
}

impl DiffViewer {
    pub fn new(view: Entity<editor_ui::DiffView>) -> Self {
        Self { view }
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        div().size_full().bg(gpui::rgb(0x1e1e2e)).child(self.view)
    }
}
