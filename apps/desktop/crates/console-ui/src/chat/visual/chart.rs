//! Chart frame: title, canvas, legend, axis notes, and the one-line summary
//! (GPUI has no accessibility tree, so the summary is visible text).

use console_core::{VisualChart, describe_chart};
use gpui::{AnyElement, FontWeight, IntoElement, ParentElement, Styled, canvas, div, px};

use super::paint;
use super::select::Selectable;
use crate::theme::Theme;

pub(super) fn chart_block(chart: &VisualChart, theme: Theme, sel: &Selectable) -> AnyElement {
    let mut col = div().flex().flex_col().gap(px(8.0));
    if let Some(title) = &chart.title {
        col = col.child(div().text_size(px(12.0)).child(sel.text(
            title.clone(),
            FontWeight::MEDIUM,
            theme.text_secondary,
            true,
        )));
    }
    let painted = chart.clone();
    col = col.child(
        canvas(
            |_, _, _| (),
            move |bounds, _, window, cx| paint::paint_chart(&painted, bounds, theme, window, cx),
        )
        .w_full()
        .h(px(190.0)),
    );
    if chart.series.len() > 1 {
        // Legend wraps below the plot, never over it.
        col = col.child(
            div()
                .flex()
                .flex_wrap()
                .gap_x(px(14.0))
                .gap_y(px(4.0))
                .children(chart.series.iter().enumerate().map(|(i, s)| {
                    div()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .child(
                            div()
                                .size(px(8.0))
                                .rounded_full()
                                .bg(paint::series_color(theme, i)),
                        )
                        .child(div().text_size(px(11.0)).child(sel.text(
                            s.name.clone(),
                            FontWeight::NORMAL,
                            theme.text_secondary,
                            false,
                        )))
                })),
        );
    }
    // GPUI exposes no accessibility tree, so the summary is visible text.
    let mut notes: Vec<String> = Vec::new();
    if let Some(x) = &chart.x_label {
        notes.push(format!("x: {x}"));
    }
    if let Some(y) = &chart.y_label {
        notes.push(format!("y: {y}"));
    }
    if !notes.is_empty() {
        col = col.child(div().text_size(px(11.0)).child(sel.text(
            notes.join("   "),
            FontWeight::NORMAL,
            theme.text_tertiary,
            false,
        )));
    }
    col.child(div().text_size(px(11.0)).child(sel.text(
        describe_chart(chart),
        FontWeight::NORMAL,
        theme.text_ghost,
        false,
    )))
    .into_any_element()
}
