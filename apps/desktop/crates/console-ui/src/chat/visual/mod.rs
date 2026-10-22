//! Native painter for `visualize` tool results (docs/plan/visualize.md §6).
//!
//! The spec is data; these files only paint it. Parsing, axis math, and the
//! point-bucketing safety net live in `console-core` so they are testable
//! without GPUI. A card blends into the transcript (no border, chat
//! background); an unsupported or malformed spec shows a compact fallback
//! instead of failing the row.
//!
//! - `mod.rs`: the card shell, title block, and block dispatch
//! - `metric.rs`: label/value/delta tiles
//! - `table.rs`: summary table with right-aligned numeric columns
//! - `chart.rs`: chart frame, legend, axis notes, accessibility summary
//! - `paint.rs`: the canvas painter (grid, ticks, series)

mod chart;
mod metric;
mod paint;
mod table;

use console_core::{VisualArtifact, VisualBlock, VisualParse, VisualSpec, parse_visual};
use gpui::{
    AnyElement, App, FontWeight, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px,
};

use crate::theme::Theme;

/// One card for a `visualize` tool result.
#[derive(IntoElement)]
pub struct VisualCard {
    artifact: VisualArtifact,
}

impl VisualCard {
    pub fn new(artifact: VisualArtifact) -> Self {
        Self { artifact }
    }
}

impl RenderOnce for VisualCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let body = match parse_visual(&self.artifact) {
            VisualParse::Ok(spec) => card_body(&spec, theme),
            VisualParse::Unsupported => fallback("This visual needs a newer version of Console.", theme),
            VisualParse::Invalid => fallback("Visual unavailable", theme),
        };
        div()
            .w_full()
            .py(px(6.0))
            .px(px(4.0))
            .flex()
            .flex_col()
            .gap(px(12.0))
            .child(body)
    }
}

fn fallback(message: &str, theme: Theme) -> AnyElement {
    div()
        .text_size(px(12.5))
        .text_color(theme.text_tertiary)
        .child(message.to_string())
        .into_any_element()
}

fn card_body(spec: &VisualSpec, theme: Theme) -> AnyElement {
    let mut col = div().w_full().flex().flex_col().gap(px(12.0));
    if spec.title.is_some() || spec.subtitle.is_some() {
        let mut head = div().flex().flex_col().gap(px(2.0));
        if let Some(t) = &spec.title {
            head = head.child(
                div()
                    .text_size(px(14.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.text)
                    .child(t.clone()),
            );
        }
        if let Some(s) = &spec.subtitle {
            head = head.child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_secondary)
                    .child(s.clone()),
            );
        }
        col = col.child(head);
    }

    // Consecutive metrics share a wrapping row; everything else stacks.
    let mut i = 0;
    while i < spec.blocks.len() {
        if matches!(spec.blocks[i], VisualBlock::Metric { .. }) {
            let mut row = div().flex().flex_wrap().gap_x(px(28.0)).gap_y(px(12.0));
            while i < spec.blocks.len() {
                let VisualBlock::Metric {
                    label,
                    value,
                    delta,
                    trend,
                    caption,
                } = &spec.blocks[i]
                else {
                    break;
                };
                row = row.child(metric::metric(label, value, delta, trend, caption, theme));
                i += 1;
            }
            col = col.child(row);
        } else {
            col = col.child(match &spec.blocks[i] {
                VisualBlock::Text { text } => div()
                    .text_size(px(13.0))
                    .text_color(theme.text_secondary)
                    .child(text.clone())
                    .into_any_element(),
                VisualBlock::Table {
                    caption,
                    columns,
                    rows,
                } => table::table(caption, columns, rows, theme),
                VisualBlock::Chart(chart) => chart::chart_block(chart, theme),
                VisualBlock::Metric { .. } => div().into_any_element(),
            });
            i += 1;
        }
    }
    col.into_any_element()
}
