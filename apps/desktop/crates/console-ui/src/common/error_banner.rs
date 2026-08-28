//! The selectable error banner shown above the composer.
//!
//! The message text is selectable — drag to select, double click for a word,
//! triple click for a line — and carries a copy button, mirroring how
//! transcript text takes part in selection. The caller owns the
//! [`TranscriptSelection`] state so the selection survives repaints. Copying
//! the message dismisses the banner via `on_copied`.

use std::rc::Rc;

use gpui::{App, IntoElement, ParentElement, Styled, div, px};

use crate::common::{centered_stripe, copy_button_with_action};
use crate::markdown::render::{
    self as markdown, Ctx as MarkdownCtx, Metrics, Palette, TranscriptSelection,
};
use crate::theme::Theme;

/// The error banner: a rounded danger-tinted card whose text is selectable and
/// copyable. Render it wherever the app surfaces a transient error.
pub fn error_banner(
    message: String,
    theme: Theme,
    selection: TranscriptSelection,
    on_copied: Option<Rc<dyn Fn(&mut App) + 'static>>,
    cx: &mut App,
) -> impl IntoElement {
    let palette = Palette::from_theme(&theme);
    let ctx = MarkdownCtx::new(
        "error-banner",
        &palette,
        Metrics::COMPACT,
        selection.clone(),
    );
    let selectable = markdown::plain_text(
        message.clone(),
        markdown::SANS_FAMILY,
        gpui::FontWeight::NORMAL,
        theme.danger,
        &ctx,
    );

    centered_stripe(
        div()
            .w_full()
            .max_w(px(768.0))
            .px(px(10.0))
            .py(px(7.0))
            .rounded(px(6.0))
            .bg(theme.composer)
            .border_1()
            .border_color(theme.danger.opacity(0.35))
            .flex()
            .items_center()
            .gap(px(6.0))
            .relative()
            // Paint order matters for selection: clear this frame's registry
            // first, then the text registers itself, then the listeners go
            // in for the frame.
            .child(markdown::frame_reset(selection.clone()))
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .text_size(px(11.5))
                    .child(selectable),
            )
            .child(copy_button_with_action(
                "error-copy".into(),
                message.clone(),
                theme,
                on_copied,
                cx,
            ))
            .child(selection_input(selection)),
        6.0,
    )
}

/// A zero-size canvas that installs the frame's selection mouse listeners for
/// the banner's text, matching the transcript's own input plumbing.
fn selection_input(selection: TranscriptSelection) -> impl IntoElement {
    gpui::canvas(
        |_, _, _| {},
        move |_, _, window, _| markdown::install_selection_input(window, &selection),
    )
    .absolute()
    .w(px(0.0))
    .h(px(0.0))
}
