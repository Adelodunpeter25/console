//! Shared anatomy of a single-line sidebar row, used by session and draft
//! items so their height, padding, status slot and title style cannot drift.

use gpui::{
    Div, ElementId, FontWeight, InteractiveElement, ParentElement, Stateful, Styled, div,
    prelude::FluentBuilder, px,
};

use crate::theme::Theme;

const ROW_HEIGHT: f32 = 34.0;
const ROW_PADDING_X: f32 = 6.0;
const ROW_RADIUS: f32 = 6.0;
const ROW_GAP: f32 = 6.0;
const STATUS_SLOT: f32 = 16.0;
const TITLE_SIZE: f32 = 13.5;

/// The row shell: fixed height, highlight when active or hovered, children laid
/// out left to right.
pub fn row_container(id: ElementId, is_active: bool, theme: Theme) -> Stateful<Div> {
    div()
        .id(id)
        .w_full()
        .min_w_0()
        .h(px(ROW_HEIGHT))
        .px(px(ROW_PADDING_X))
        .rounded(px(ROW_RADIUS))
        .cursor_default()
        .flex()
        .flex_row()
        .items_center()
        .gap_x(px(ROW_GAP))
        .when(is_active, |s| s.bg(theme.sidebar_item_background))
        .when(!is_active, |s| {
            s.hover(|h| h.bg(theme.sidebar_item_background))
        })
}

/// Fixed-width leading slot. Empty on rows with no status, so titles stay
/// aligned with rows that show one.
pub fn status_slot() -> Div {
    div()
        .size(px(STATUS_SLOT))
        .flex_none()
        .flex()
        .items_center()
        .justify_center()
}

/// The title text box. One weight for both states: a heavier selected weight is
/// wider, so the title would reflow and look larger on selection.
pub fn title_label(is_active: bool, theme: Theme) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .text_size(px(TITLE_SIZE))
        .font_weight(FontWeight::MEDIUM)
        .text_color(if is_active {
            theme.text
        } else {
            theme.text_tertiary
        })
}

/// The "Draft" badge shown next to a title.
pub fn draft_badge(theme: Theme) -> Div {
    div()
        .flex_none()
        .px(px(4.0))
        .py(px(0.5))
        .rounded(px(3.0))
        .bg(theme.accent.opacity(0.15))
        .text_color(theme.accent)
        .text_size(px(TITLE_SIZE))
        .font_weight(FontWeight::MEDIUM)
        .child("Draft")
}
