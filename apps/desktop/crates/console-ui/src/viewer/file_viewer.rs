//! File viewer: the `editor-ui` code view plus host-owned overlays.
//!
//! Comments are host elements (they own the input entities and callbacks), so
//! the viewer only positions them. `EditorView::comment_anchor` reports the
//! geometry of the virtualized rows, including the live scroll offset, which
//! lets the cards ride scrolling without being inserted into the list.

use gpui::{
    AnyElement, App, DispatchPhase, Entity, IntoElement, ParentElement, RenderOnce,
    ScrollWheelEvent, Styled, Window, canvas, div, px,
};
use crate::primitives::file_type_icon;
use crate::theme::Theme;
use std::rc::Rc;

/// Room kept free under a file/diff viewer so its last lines can scroll up
/// above the floating composer's resting bar (bar height + its padding).
pub const FLOATING_COMPOSER_INSET: f32 = 78.0;

/// Invisible overlay that reports wheel scrolling anywhere over its bounds.
/// Listens in the capture phase because the scrolling list stops propagation
/// of the events it consumes.
pub fn scroll_watch(on_scroll: impl Fn(&mut Window, &mut App) + 'static) -> impl IntoElement {
    let on_scroll = Rc::new(on_scroll);
    canvas(
        |_, _, _| (),
        move |bounds, _, window, _cx| {
            let on_scroll = on_scroll.clone();
            window.on_mouse_event(move |event: &ScrollWheelEvent, phase, window, cx| {
                if phase == DispatchPhase::Capture && bounds.contains(&event.position) {
                    on_scroll(window, cx);
                }
            });
        },
    )
    .absolute()
    .size_full()
}

/// The vertical room reserved when a commented range sits near the bottom of
/// the viewport. Cards are clamped above this line so they stay reachable.
const FILE_COMMENT_CARD_ESTIMATE: f32 = 150.0;

/// How far above the viewport a card's anchor may sit before the card is
/// dropped. The viewer clips overflow, so a card straddling the top edge keeps
/// scrolling out smoothly until it is fully gone.
const FILE_COMMENT_CARD_HIDE_ABOVE: f32 = FILE_COMMENT_CARD_ESTIMATE;

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
    path: Option<String>,
}

impl FileViewer {
    pub fn new(view: Entity<editor_ui::EditorView>) -> Self {
        Self {
            view,
            comments: Vec::new(),
            path: None,
        }
    }

    /// Shows this (already display-ready) path in a header row above the code.
    pub fn path(mut self, path: impl Into<String>) -> Self {
        self.path = Some(path.into());
        self
    }

    pub fn comments(mut self, comments: Vec<FileCommentOverlay>) -> Self {
        self.comments = comments;
        self
    }
}

impl RenderOnce for FileViewer {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let Self {
            view,
            comments,
            path,
        } = self;
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
                let raw_top = anchor.top_padding
                    + anchor.line_height * anchor.row_after as f32
                    + anchor.scroll_y;
                // Cards ride scrolling: once the range they hang under has
                // scrolled out of view (above, or below the fold) the card is
                // dropped instead of being pinned to the viewport edge.
                if anchor.viewport_height > px(0.0) {
                    if raw_top < px(-FILE_COMMENT_CARD_HIDE_ABOVE)
                        || raw_top >= anchor.viewport_height
                    {
                        return None;
                    }
                } else if raw_top < px(-FILE_COMMENT_CARD_HIDE_ABOVE) {
                    return None;
                }
                let top = if anchor.viewport_height > px(0.0) {
                    let max_top =
                        (anchor.viewport_height - px(FILE_COMMENT_CARD_ESTIMATE)).max(px(0.0));
                    raw_top.min(max_top)
                } else {
                    raw_top
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

        let body = div()
            .relative()
            .flex_1()
            .min_h_0()
            .w_full()
            .overflow_hidden()
            .bg(gpui::rgb(0x000000))
            .child(
                div()
                    .absolute()
                    .top_0()
                    .left_0()
                    .right_0()
                    .bottom(px(FLOATING_COMPOSER_INSET))
                    .child(view),
            )
            .children(overlays);

        let header = path.map(|path| {
            let theme = Theme::current(cx);
            let (dir, name) = match path.rfind('/') {
                Some(i) => (path[..=i].to_string(), path[i + 1..].to_string()),
                None => (String::new(), path.clone()),
            };
            div()
                .flex_none()
                .w_full()
                .px(px(12.0))
                .py(px(8.0))
                .bg(gpui::rgb(0x000000))
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(8.0))
                        .max_w_full()
                        .px(px(10.0))
                        .py(px(4.0))
                        .rounded(px(8.0))
                        .border_1()
                        .border_color(theme.border)
                        .child(file_type_icon(&path, 16.0))
                        .child(
                            div()
                                .flex()
                                .min_w_0()
                                .overflow_hidden()
                                .text_size(px(13.0))
                                .whitespace_nowrap()
                                .child(div().text_color(theme.text_tertiary).child(dir))
                                .child(div().flex_none().text_color(theme.text).child(name)),
                        ),
                )
        });

        div()
            .size_full()
            .flex()
            .flex_col()
            .children(header)
            .child(body)
    }
}
