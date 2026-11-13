//! Inline "comment for the AI" card and its composer chip.
//!
//! `editor-ui` owns the gutter interaction and reports the selected 1-based
//! line range; the host mounts this card directly below that range. A draft
//! becomes a staged comment (chip + card) that stays attached to the message
//! until the user edits or deletes it.

use std::rc::Rc;

use gpui::{
    App, Entity, FontWeight, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::input::ComposerInput;
use crate::primitives::file_type_icon;
use crate::theme::Theme;

/// Which controls the card shows.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum FileCommentCardState {
    /// A new range; typing has not been committed yet.
    Draft,
    /// The comment is staged and included in the next message.
    Added,
    /// An already staged comment being changed.
    Editing,
}

/// A compact card anchored under a commented line range.
#[derive(IntoElement)]
pub struct FileCommentCard {
    id: u64,
    path: String,
    start_line: u32,
    end_line: u32,
    state: FileCommentCardState,
    /// Committed text, shown in the `Added` state.
    text: String,
    input: Entity<ComposerInput>,
    on_commit: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_edit: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_remove: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_send: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    /// Already attached to the next message; the send action then takes it back.
    staged: bool,
}

impl FileCommentCard {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        id: u64,
        path: impl Into<String>,
        start_line: u32,
        end_line: u32,
        state: FileCommentCardState,
        text: impl Into<String>,
        input: Entity<ComposerInput>,
        on_commit: impl Fn(&mut Window, &mut App) + 'static,
        on_cancel: impl Fn(&mut Window, &mut App) + 'static,
        on_edit: impl Fn(&mut Window, &mut App) + 'static,
        on_remove: impl Fn(&mut Window, &mut App) + 'static,
        on_send: impl Fn(&mut Window, &mut App) + 'static,
        staged: bool,
    ) -> Self {
        Self {
            id,
            path: path.into(),
            start_line,
            end_line,
            state,
            text: text.into(),
            input,
            on_commit: Rc::new(on_commit),
            on_cancel: Rc::new(on_cancel),
            on_edit: Rc::new(on_edit),
            on_remove: Rc::new(on_remove),
            on_send: Rc::new(on_send),
            staged,
        }
    }

    /// `+12-31` for a range, `+12` for a single line.
    fn line_label(&self) -> String {
        if self.start_line == self.end_line {
            format!("+{}", self.start_line)
        } else {
            format!("+{}-{}", self.start_line, self.end_line)
        }
    }
}

impl RenderOnce for FileCommentCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let is_composer = matches!(
            self.state,
            FileCommentCardState::Draft | FileCommentCardState::Editing
        );
        let can_commit = !self.input.read(cx).content().trim().is_empty();
        let basename = std::path::Path::new(&self.path)
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or(&self.path)
            .to_string();
        let line_label = self.line_label();

        let commit = self.on_commit.clone();
        let commit_for_click = commit.clone();
        let cancel = self.on_cancel.clone();
        let cancel_for_click = cancel.clone();
        let cancel_for_escape = cancel.clone();
        let edit = self.on_edit.clone();
        let edit_for_click = edit.clone();
        let remove = self.on_remove.clone();
        let send = self.on_send.clone();
        let staged = self.staged;

        let header = div()
            .px(px(11.0))
            .pt(px(8.0))
            .pb(px(6.0))
            .flex()
            .items_center()
            .gap(px(5.0))
            .child(file_type_icon(&self.path, 12.0))
            .child(
                div()
                    .min_w_0()
                    .truncate()
                    .text_size(px(12.0))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(theme.text)
                    .child(basename),
            )
            .child(
                div()
                    .flex_none()
                    .text_size(px(11.0))
                    .text_color(theme.text_tertiary)
                    .child(line_label),
            )
            .child(div().flex_1());

        let body = match self.state {
            FileCommentCardState::Draft | FileCommentCardState::Editing => div()
                .w_full()
                .min_h(px(50.0))
                .max_h(px(150.0))
                .px(px(11.0))
                .pb(px(6.0))
                .child(self.input.clone()),
            FileCommentCardState::Added => div()
                .px(px(11.0))
                .pb(px(8.0))
                .text_size(px(12.5))
                .line_height(px(18.0))
                .text_color(theme.text_secondary)
                .child(self.text.clone()),
        };

        let footer = match self.state {
            FileCommentCardState::Draft | FileCommentCardState::Editing => div()
                .px(px(9.0))
                .pb(px(8.0))
                .flex()
                .items_center()
                .justify_end()
                .gap(px(4.0))
                .child(
                    div()
                        .id(("file-comment-cancel", self.id))
                        .h(px(26.0))
                        .px(px(9.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .cursor_default()
                        .hover(|style| style.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                            cx.stop_propagation()
                        })
                        .on_click(move |_, window, cx| (cancel_for_click)(window, cx))
                        .child(
                            div()
                                .text_size(px(12.0))
                                .text_color(theme.text_secondary)
                                .child("Cancel"),
                        ),
                )
                .child(
                    div()
                        .id(("file-comment-commit", self.id))
                        .h(px(26.0))
                        .px(px(11.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .cursor_default()
                        .when(can_commit, |el| el.bg(theme.text))
                        .when(!can_commit, |el| el.opacity(0.4))
                        .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                            cx.stop_propagation()
                        })
                        .on_click(move |_, window, cx| {
                            if can_commit {
                                (commit_for_click)(window, cx);
                            }
                        })
                        .child(
                            div()
                                .text_size(px(12.0))
                                .font_weight(FontWeight::MEDIUM)
                                .text_color(if can_commit {
                                    theme.canvas
                                } else {
                                    theme.text_tertiary
                                })
                                .child(if self.state == FileCommentCardState::Editing {
                                    "Save"
                                } else {
                                    "Add"
                                }),
                        ),
                ),
            FileCommentCardState::Added => div()
                .px(px(7.0))
                .pb(px(6.0))
                .flex()
                .items_center()
                .gap(px(2.0))
                .child(
                    div()
                        .id(("file-comment-edit", self.id))
                        .h(px(24.0))
                        .px(px(8.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .cursor_default()
                        .hover(|style| style.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                            cx.stop_propagation()
                        })
                        .on_click(move |_, window, cx| (edit_for_click)(window, cx))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child("Edit"),
                        ),
                )
                .child(
                    div()
                        .id(("file-comment-remove", self.id))
                        .h(px(24.0))
                        .px(px(8.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .cursor_default()
                        .hover(|style| style.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                            cx.stop_propagation()
                        })
                        .on_click(move |_, window, cx| (remove)(window, cx))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .text_color(theme.text_tertiary)
                                .child("Delete"),
                        ),
                )
                .child(div().flex_1())
                .child(
                    div()
                        .id(("file-comment-send", self.id))
                        .h(px(24.0))
                        .px(px(8.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .cursor_default()
                        .hover(|style| style.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                            cx.stop_propagation()
                        })
                        .on_click(move |_, window, cx| (send)(window, cx))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .font_weight(FontWeight::MEDIUM)
                                .text_color(if staged {
                                    theme.text_tertiary
                                } else {
                                    theme.text
                                })
                                .child(if staged { "Added to chat" } else { "Send to AI" }),
                        ),
                ),
        };

        div()
            .id(("file-comment-card", self.id))
            .occlude()
            .w_full()
            .rounded(px(10.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.border_strong)
            .shadow_md()
            .overflow_hidden()
            .when(is_composer, |el| {
                el.on_key_down(move |event: &gpui::KeyDownEvent, window, cx| {
                    if event.keystroke.key == "escape" {
                        (cancel_for_escape)(window, cx);
                        cx.stop_propagation();
                    }
                })
            })
            .child(header)
            .child(body)
            .child(footer)
    }
}
