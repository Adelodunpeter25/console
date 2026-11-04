//! File viewer: the `editor-ui` code view plus host-owned overlays.
//!
//! Comments are host elements (they own the input entities and callbacks), so
//! the viewer only positions them. `EditorView::comment_anchor` reports the
//! geometry of the virtualized rows, including the live scroll offset, which
//! lets the cards ride scrolling without being inserted into the list.

use gpui::{AnyElement, App, Entity, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

/// The vertical room reserved when a commented range sits near the bottom of
/// the viewport. Cards are clamped above this line so they stay reachable.
const FILE_COMMENT_CARD_ESTIMATE: f32 = 150.0;

/// One host-rendered comment card plus the 1-based range it belongs under.
pub struct FileCommentOverlay {
    pub start_line: u32,
    pub end_line: u32,
    pub element: AnyElement,
}

impl FileCommentOverlay {
    pub fn new(start_line: u32, end_line: u32, element: impl IntoElement) -> Self {
        Self {
            start_line,
            end_line,
            element: element.into_any_element(),
        }
    }
}

#[derive(IntoElement)]
pub struct FileViewer {
    view: Entity<editor_ui::EditorView>,
    comments: Vec<FileCommentOverlay>,
}

impl FileViewer {
    pub fn new(view: Entity<editor_ui::EditorView>) -> Self {
        Self {
            view,
            comments: Vec::new(),
        }
    }

    pub fn comments(mut self, comments: Vec<FileCommentOverlay>) -> Self {
        self.comments = comments;
        self
    }
}

impl RenderOnce for FileViewer {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let Self { view, comments } = self;
        let anchors = {
            let editor = view.read(cx);
            comments
                .iter()
                .map(|comment| {
                    editor.comment_anchor(
                        editor_ui::CommentTarget {
                            start_line: comment.start_line,
                            end_line: comment.end_line,
                        },
                        window,
                        cx,
                    )
                })
                .collect::<Vec<_>>()
        };

        let overlays = comments
            .into_iter()
            .zip(anchors)
            .filter_map(|(comment, anchor)| {
                let anchor = anchor?;
                let raw_top =
                    anchor.top_padding + anchor.line_height * anchor.row_after as f32 + anchor.scroll_y;
                let top = if anchor.viewport_height > px(0.0) {
                    let max_top = (anchor.viewport_height - px(FILE_COMMENT_CARD_ESTIMATE))
                        .max(px(0.0));
                    raw_top.max(px(0.0)).min(max_top)
                } else {
                    raw_top.max(px(0.0))
                };
                Some(
                    div()
                        .absolute()
                        .top(top)
                        .left(anchor.content_x)
                        .right(px(24.0))
                        .child(comment.element),
                )
            });

        div()
            .relative()
            .size_full()
            .overflow_hidden()
            .bg(gpui::rgb(0x000000))
            .child(div().absolute().inset_0().child(view))
            .children(overlays)
    }
}
