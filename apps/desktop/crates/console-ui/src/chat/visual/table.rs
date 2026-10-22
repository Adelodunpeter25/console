//! Summary table. Numeric-looking columns right-align; wide tables scroll
//! horizontally inside the card, never the transcript.

use console_core::looks_numeric;
use gpui::{
    AnyElement, FontWeight, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, div, px,
};

use super::select::Selectable;
use crate::theme::Theme;

pub(super) fn table(
    caption: &Option<String>,
    columns: &[String],
    rows: &[Vec<String>],
    theme: Theme,
    sel: &Selectable,
) -> AnyElement {
    let numeric: Vec<bool> = (0..columns.len())
        .map(|c| {
            !rows.is_empty()
                && rows
                    .iter()
                    .all(|r| r.get(c).is_some_and(|cell| looks_numeric(cell)))
        })
        .collect();
    // Cells register in paint order (header, then row by row), so a copy
    // reads in table order, one cell per line.
    let cell = |text: &str, right: bool, header: bool| {
        let (weight, color) = if header {
            (FontWeight::MEDIUM, theme.text_secondary)
        } else {
            (FontWeight::NORMAL, theme.text)
        };
        let mut d = div()
            .min_w(px(84.0))
            .px(px(8.0))
            .py(px(6.0))
            .text_size(px(12.0))
            .child(sel.text(text.to_string(), weight, color, false));
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
                .child(sel.text(caption.clone(), FontWeight::MEDIUM, theme.text_secondary, true)),
        );
    }
    col.child(grid).into_any_element()
}
