//! The full-width strip that centers a card on the 768px content column.
//!
//! Shared by every banner and interaction card above the composer so they all
//! align with the transcript and composer instead of each re-declaring the
//! wrapper.

use gpui::{IntoElement, ParentElement, Styled, div, px};

/// Wrap `child` in the standard centered strip with the given vertical
/// padding, matching the composer and transcript alignment.
pub fn centered_stripe(child: impl IntoElement, padding_y: f32) -> impl IntoElement {
    div()
        .w_full()
        .px(px(20.0))
        .py(px(padding_y))
        .flex()
        .justify_center()
        .child(child)
}
