use gpui::{Hsla, IntoElement, ParentElement, Styled, div, px};

use crate::theme::Theme;

/// Icon size for the inline file-mention chip. Shared by the composer paint
/// (`input/element.rs`) and the static bubble (`chat/message_bubble.rs`) so
/// the two stay visually identical.
pub const FILE_MENTION_ICON_SIZE: f32 = 11.0;

/// Corner radius for the file-mention pill. Matches the composer's painted quad.
pub const FILE_MENTION_RADIUS: f32 = 4.0;

/// Background, border, and label colors for the file-mention pill.
/// This is the composer look: accent-tinted wash with an accent label.
pub fn file_mention_colors(theme: Theme) -> (Hsla, Hsla, Hsla) {
    (
        theme.accent.opacity(0.10),
        theme.accent.opacity(0.28),
        theme.accent,
    )
}

/// Centralised inline file pill: file-type icon + filename label.
///
/// Used by the user message bubble. The composer cannot embed a `div` inside
/// its `StyledText`, so it paints the same colors/metrics via
/// [`file_mention_colors`], [`FILE_MENTION_ICON_SIZE`], and
/// [`FILE_MENTION_RADIUS`] instead.
pub fn file_mention_chip(
    path: &str,
    label: impl Into<String>,
    theme: Theme,
) -> impl IntoElement {
    let (bg, border, text) = file_mention_colors(theme);
    div()
        .flex()
        .items_center()
        .gap(px(4.0))
        .px(px(6.0))
        .py(px(1.0))
        .rounded(px(FILE_MENTION_RADIUS))
        .border_1()
        .border_color(border)
        .bg(bg)
        .child(crate::primitives::file_type_icon(
            path,
            FILE_MENTION_ICON_SIZE,
        ))
        .child(
            div()
                .text_size(px(12.5))
                .text_color(text)
                .child(label.into()),
        )
}
