use gpui::{App, Entity, IntoElement, RenderOnce, Window, div, prelude::*};

#[derive(IntoElement)]
pub struct FileViewer {
    view: Entity<editor_ui::EditorView>,
}

impl FileViewer {
    pub fn new(view: Entity<editor_ui::EditorView>) -> Self {
        Self { view }
    }
}

impl RenderOnce for FileViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        div().size_full().child(self.view)
    }
}
