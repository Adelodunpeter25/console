//! Metric tile: label, big value, optional delta with a trend arrow.

use gpui::{AnyElement, FontWeight, IntoElement, ParentElement, Styled, div, px};

use crate::theme::Theme;

pub(super) fn metric(
    label: &str,
    value: &str,
    delta: &Option<String>,
    trend: &Option<String>,
    caption: &Option<String>,
    theme: Theme,
) -> AnyElement {
    let mut col = div()
        .min_w(px(96.0))
        .flex()
        .flex_col()
        .gap(px(2.0))
        .child(
            div()
                .text_size(px(12.0))
                .text_color(theme.text_secondary)
                .child(label.to_string()),
        )
        .child(
            div()
                .text_size(px(22.0))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(theme.text)
                .child(value.to_string()),
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
                .font_weight(FontWeight::MEDIUM)
                .text_color(tint)
                .child(format!("{arrow}{delta}")),
        );
    }
    if let Some(caption) = caption {
        col = col.child(
            div()
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child(caption.clone()),
        );
    }
    col.into_any_element()
}
