//! The "comment for the AI" card opened from an editor's comment gutter.
//!
//! `editor-ui` reports the selected 1-based line range and leaves the UI to
//! the host; this is that UI. It is a scrim-backed card with one prompt field:
//! type the comment, press Enter (or Send), and the owning app turns it into a
//! chat prompt that references `<path>:<lines>`. Purely presentational — the
//! app owns the draft, the input entity and the submit path.

use std::rc::Rc;

use gpui::{
    App, Entity, FontWeight, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::input::ComposerInput;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// The comment card. Rendered as a window-level overlay so it is not clipped
/// by the editor's scroll viewport.
#[derive(IntoElement)]
pub struct FileCommentComposer {
    path: String,
    start_line: u32,
    end_line: u32,
    input: Entity<ComposerInput>,
    on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl FileCommentComposer {
    pub fn new(
        path: impl Into<String>,
        start_line: u32,
        end_line: u32,
        input: Entity<ComposerInput>,
        on_submit: impl Fn(String, &mut Window, &mut App) + 'static,
        on_cancel: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            path: path.into(),
            start_line,
            end_line,
            input,
            on_submit: Rc::new(on_submit),
            on_cancel: Rc::new(on_cancel),
        }
    }
}

impl RenderOnce for FileCommentComposer {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // `12–31` for a range, `12` for a single row, so a one-line comment
        // reads as `src/main.rs:12` rather than `src/main.rs:12–12`.
        let lines = if self.start_line == self.end_line {
            self.start_line.to_string()
        } else {
            format!("{}–{}", self.start_line, self.end_line)
        };
        let target = format!("{}:{}", self.path, lines);
        let can_submit = !self.input.read(cx).content().trim().is_empty();

        let on_submit = self.on_submit.clone();
        let submit_input = self.input.clone();
        let on_cancel = self.on_cancel.clone();
        let dismiss = on_cancel.clone();
        let cancel_click = on_cancel.clone();

        let send_button = div()
            .id("file-comment-send")
            .h(px(28.0))
            .px(px(12.0))
            .rounded(px(7.0))
            .flex()
            .items_center()
            .gap(px(6.0))
            .cursor_default()
            .when(can_submit, |el| el.bg(theme.text))
            .when(!can_submit, |el| el.opacity(0.4))
            // Keep the press from reading as a scrim click.
            .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                cx.stop_propagation()
            })
            .on_click(move |_, window, cx| {
                if !can_submit {
                    return;
                }
                let text = submit_input.read(cx).content().to_string();
                on_submit(text, window, cx);
            })
            .child(
                div()
                    .text_size(px(12.5))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(if can_submit {
                        theme.canvas
                    } else {
                        theme.text_tertiary
                    })
                    .child("Send"),
            )
            .child(
                div()
                    .text_size(px(11.0))
                    .text_color(if can_submit {
                        theme.canvas.opacity(0.6)
                    } else {
                        theme.text_ghost
                    })
                    .child("↵"),
            );

        let cancel_button = div()
            .id("file-comment-cancel")
            .h(px(28.0))
            .px(px(10.0))
            .rounded(px(7.0))
            .flex()
            .items_center()
            .cursor_default()
            .hover(|style| style.bg(theme.overlay))
            .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                cx.stop_propagation()
            })
            .on_click(move |_, window, cx| cancel_click(window, cx))
            .child(
                div()
                    .text_size(px(12.5))
                    .text_color(theme.text_secondary)
                    .child("Cancel"),
            );

        let input = self.input.clone();
        let focus = self.input.read(cx).focus();
        let key_cancel = on_cancel.clone();

        div()
            .absolute()
            .inset_0()
            .bg(gpui::black().opacity(0.28))
            .flex()
            .justify_center()
            .items_end()
            .pb(px(56.0))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                dismiss(window, cx);
            })
            .child(
                div()
                    .occlude()
                    .key_context("FileCommentComposer")
                    .track_focus(&focus)
                    .on_key_down(move |event: &gpui::KeyDownEvent, window, cx| {
                        if event.keystroke.key == "escape" {
                            key_cancel(window, cx);
                            cx.stop_propagation();
                        }
                    })
                    .on_mouse_down(MouseButton::Left, |_, _: &mut Window, cx| {
                        cx.stop_propagation()
                    })
                    .w(px(620.0))
                    .rounded(px(12.0))
                    .bg(theme.composer)
                    .border_1()
                    .border_color(theme.border_strong)
                    .shadow_lg()
                    .overflow_hidden()
                    // header
                    .child(
                        div()
                            .px(px(14.0))
                            .pt(px(11.0))
                            .pb(px(9.0))
                            .flex()
                            .flex_col()
                            .gap(px(3.0))
                            .border_b_1()
                            .border_color(theme.border)
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(7.0))
                                    .child(app_icon(
                                        IconName::ChatRoundLine,
                                        13.0,
                                        theme.text_secondary,
                                    ))
                                    .child(
                                        div()
                                            .text_size(px(12.5))
                                            .font_weight(FontWeight::MEDIUM)
                                            .text_color(theme.text)
                                            .child("Comment for the AI"),
                                    ),
                            )
                            .child(
                                div()
                                    .pl(px(20.0))
                                    .text_size(px(11.0))
                                    .text_color(theme.text_tertiary)
                                    .truncate()
                                    .child(target),
                            ),
                    )
                    // prompt
                    .child(
                        div()
                            .w_full()
                            .min_h(px(84.0))
                            .px(px(14.0))
                            .py(px(10.0))
                            .child(input),
                    )
                    // footer
                    .child(
                        div()
                            .px(px(12.0))
                            .pt(px(4.0))
                            .pb(px(10.0))
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(
                                div()
                                    .flex_1()
                                    .text_size(px(10.5))
                                    .text_color(theme.text_ghost)
                                    .child("Sent to the active chat with the file and line range."),
                            )
                            .child(cancel_button)
                            .child(send_button),
                    ),
            )
    }
}
