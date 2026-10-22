//! Summary table. Numeric-looking columns right-align; wide tables scroll
//! horizontally inside the card, never the transcript.

use console_core::looks_numeric;
use gpui::{
    AnyElement, FontWeight, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, div, prelude::*, px,
};

use crate::theme::Theme;

pub(super) fn table(
    caption: &Option<String>,
    columns: &[String],
    rows: &[Vec<String>],
    theme: Theme,
) -> AnyElement {
    let numeric: Vec<bool> = (0..columns.len())
        .map(|c| {
            !rows.is_empty()
                && rows
                    .iter()
                    .all(|r| r.get(c).is_some_and(|cell| looks_numeric(cell)))
        })
        .collect();
    let cell = |text: &str, right: bool, header: bool| {
        let mut d = div()
            .min_w(px(84.0))
            .px(px(8.0))
            .py(px(6.0))
            .text_size(px(12.0))
            .text_color(if header { theme.text_secondary } else { theme.text })
            .when(header, |d| d.font_weight(FontWeight::MEDIUM))
            .child(text.to_string());
        if right {
            d = d.flex().justify_end();
        }
        d
    };
    let mut grid = div()
        .id("visual-table")
        .overflow_x_scroll()
        .flex()
        .flex_col()
        .child(
            div()
                .flex()
                .border_b_1()
                .border_color(theme.border)
                .children(
                    columns
                        .iter()
                        .enumerate()
                        .map(|(c, name)| cell(name, numeric[c], true)),
                ),
        );
    for row in rows {
        grid = grid.child(div().flex().children(row.iter().enumerate().map(|(c, v)| {
            cell(v, numeric.get(c).copied().unwrap_or(false), false)
        })));
    }
    let mut col = div().flex().flex_col().gap(px(6.0));
    if let Some(caption) = caption {
        col = col.child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(caption.clone()),
        );
    }
    col.child(grid).into_any_element()
}
