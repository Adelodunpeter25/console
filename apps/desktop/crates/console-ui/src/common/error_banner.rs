//! The selectable error banner shown above the composer.
//!
//! The message text is selectable — drag to select, double click for a word,
//! triple click for a line — and carries a copy button, mirroring how
//! transcript text takes part in selection. The caller owns the
//! [`TranscriptSelection`] state so the selection survives repaints. Copying
//! the message dismisses the banner via `on_copied`.

use std::rc::Rc;

use gpui::{
    App, IntoElement, InteractiveElement, ParentElement, StatefulInteractiveElement, Styled, div,
    px,
};

use crate::common::{centered_stripe, copy_button_with_action};
use crate::markdown::render::{
    self as markdown, Ctx as MarkdownCtx, Metrics, Palette, TranscriptSelection,
};
use crate::theme::Theme;

/// An optional labeled action rendered after the copy button (e.g. Retry on
/// a failed worktree launch). The caller owns what it does.
pub struct BannerAction {
    pub label: String,
    pub on_click: Rc<dyn Fn(&mut App) + 'static>,
}

/// The error banner: a rounded danger-tinted card whose text is selectable and
/// copyable. Render it wherever the app surfaces a transient error.
pub fn error_banner(
    message: String,
    theme: Theme,
    selection: TranscriptSelection,
    on_copied: Option<Rc<dyn Fn(&mut App) + 'static>>,
    actions: Vec<BannerAction>,
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
            .children(
                actions
                    .into_iter()
                    .map(|action| action_button(action, theme).into_any_element())
                    .collect::<Vec<_>>(),
            )
            .child(selection_input(selection)),
        6.0,
    )
}

/// A zero-size canvas that installs the frame's selection mouse listeners for
/// the banner's text, matching the transcript's own input plumbing.
fn selection_input(selection: TranscriptSelection) -> impl IntoElement {
    gpui::canvas(
        |_, _, _| {},
        move |_, _, window, _| markdown::install_selection_input(window, &selection, None),
    )
    .absolute()
    .w(px(0.0))
    .h(px(0.0))
}

/// The banner's labeled action, styled like a compact text button.
fn action_button(action: BannerAction, theme: Theme) -> impl IntoElement {
    let BannerAction { label, on_click } = action;
    div()
        .id(gpui::SharedString::from(format!("error-banner-{label}")))
        .cursor_pointer()
        .px(px(6.0))
        .py(px(2.0))
        .rounded(px(4.0))
        .text_size(px(11.0))
        .text_color(theme.text)
        .bg(theme.danger_soft)
        .hover(|style| style.bg(theme.danger.opacity(0.32)))
        .child(label)
        .on_click(move |_, _, cx| on_click(cx))
        .into_any_element()
}
