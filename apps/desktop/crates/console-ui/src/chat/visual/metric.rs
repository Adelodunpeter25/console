//! Metric tile: label, big value, optional delta with a trend arrow.

use gpui::{AnyElement, FontWeight, IntoElement, ParentElement, Styled, div, px};

use super::select::Selectable;
use crate::theme::Theme;

pub(super) fn metric(
    label: &str,
    value: &str,
    delta: &Option<String>,
    trend: &Option<String>,
    caption: &Option<String>,
    theme: Theme,
    sel: &Selectable,
) -> AnyElement {
    let mut col = div()
        .min_w(px(96.0))
        .flex()
        .flex_col()
        .gap(px(2.0))
        .child(
            div()
                .text_size(px(12.0))
                .child(sel.text(label.to_string(), FontWeight::NORMAL, theme.text_secondary, true)),
        )
        .child(
            div()
                .text_size(px(22.0))
                .child(sel.text(value.to_string(), FontWeight::SEMIBOLD, theme.text, false)),
        );
    if let Some(delta) = delta {
        // Arrow glyph plus color so the trend is never color-only.
        let (arrow, tint) = match trend.as_deref() {
            Some("up") => ("▲ ", theme.success),
            Some("down") => ("▼ ", theme.danger),
            Some("flat") => ("▬ ", theme.text_secondary),
            _ => ("", theme.text_secondary),
        };
        col = col.child(
            div()
                .text_size(px(12.0))
                .child(sel.text(format!("{arrow}{delta}"), FontWeight::MEDIUM, tint, false)),
        );
    }
    if let Some(caption) = caption {
        col = col.child(
            div()
                .text_size(px(11.0))
                .child(sel.text(caption.clone(), FontWeight::NORMAL, theme.text_tertiary, false)),
        );
    }
    col.into_any_element()
}
