//! Native painter for `visualize` tool results (docs/plan/visualize.md §6).
//!
//! The spec is data; this file only paints it. Parsing, axis math, and the
//! point-bucketing safety net live in `console-core` so they are testable
//! without GPUI. A card blends into the transcript (no border, chat
//! background) and an unsupported or malformed spec shows a compact
//! fallback instead of failing the row.

use console_core::{
    VisualArtifact, VisualBlock, VisualChart, VisualParse, VisualSpec, describe_chart,
    format_compact, looks_numeric, min_max_buckets, parse_visual, value_axis,
};
use gpui::{
    AnyElement, App, Bounds, FontWeight, Hsla, IntoElement, ParentElement, PathBuilder, Pixels,
    RenderOnce, SharedString, Styled, TextAlign, TextRun, Window, canvas, div, point, prelude::*,
    px,
};

use crate::markdown::render::MONO_FAMILY;
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
                row = row.child(metric(label, value, delta, trend, caption, theme));
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
                } => table(caption, columns, rows, theme),
                VisualBlock::Chart(chart) => chart_block(chart, theme),
                VisualBlock::Metric { .. } => div().into_any_element(),
            });
            i += 1;
        }
    }
    col.into_any_element()
}

fn metric(
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

fn table(
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

fn chart_block(chart: &VisualChart, theme: Theme) -> AnyElement {
    let mut col = div().flex().flex_col().gap(px(8.0));
    if let Some(title) = &chart.title {
        col = col.child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(title.clone()),
        );
    }
    let painted = chart.clone();
    col = col.child(
        canvas(
            |_, _, _| (),
            move |bounds, _, window, cx| paint_chart(&painted, bounds, theme, window, cx),
        )
        .w_full()
        .h(px(190.0)),
    );
    if chart.series.len() > 1 {
        // Legend wraps below the plot, never over it.
        col = col.child(
            div().flex().flex_wrap().gap_x(px(14.0)).gap_y(px(4.0)).children(
                chart.series.iter().enumerate().map(|(i, s)| {
                    div()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .child(
                            div()
                                .size(px(8.0))
                                .rounded_full()
                                .bg(series_color(theme, i)),
                        )
                        .child(
                            div()
                                .text_size(px(11.0))
                                .text_color(theme.text_secondary)
                                .child(s.name.clone()),
                        )
                }),
            ),
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
        col = col.child(
            div()
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child(notes.join("   ")),
        );
    }
    col.child(
        div()
            .text_size(px(11.0))
            .text_color(theme.text_ghost)
            .child(describe_chart(chart)),
    )
    .into_any_element()
}

fn series_color(theme: Theme, i: usize) -> Hsla {
    theme.chart[i % theme.chart.len()]
}

fn paint_text(
    text: &str,
    origin: gpui::Point<Pixels>,
    color: Hsla,
    window: &mut Window,
    cx: &mut App,
) -> Pixels {
    let run = TextRun {
        len: text.len(),
        font: gpui::Font {
            family: SharedString::from(MONO_FAMILY),
            weight: FontWeight::NORMAL,
            style: gpui::FontStyle::Normal,
            features: Default::default(),
            fallbacks: None,
        },
        color,
        background_color: None,
        underline: None,
        strikethrough: None,
    };
    let shaped = window.text_system().shape_line(
        SharedString::from(text.to_string()),
        px(10.0),
        &[run],
        None,
    );
    let width = shaped.width;
    let _ = shaped.paint(origin, px(12.0), TextAlign::Left, None, window, cx);
    width
}

fn stroke_path(
    pts: &[(f32, f32)],
    width: f32,
    color: Hsla,
    window: &mut Window,
) {
    let Some(first) = pts.first() else { return };
    let mut b = PathBuilder::stroke(px(width));
    b.move_to(point(px(first.0), px(first.1)));
    for p in &pts[1..] {
        b.line_to(point(px(p.0), px(p.1)));
    }
    if let Ok(path) = b.build() {
        window.paint_path(path, color);
    }
}

fn paint_chart(
    chart: &VisualChart,
    bounds: Bounds<Pixels>,
    theme: Theme,
    window: &mut Window,
    cx: &mut App,
) {
    let all: Vec<&console_core::VisualPoint> =
        chart.series.iter().flat_map(|s| s.points.iter()).collect();
    if all.is_empty() {
        return;
    }
    let scatter = chart.kind == "scatter";
    let bars = chart.kind == "bar";
    let include_zero = bars || chart.kind == "area";
    let y_lo = all.iter().map(|p| p.y).fold(f64::INFINITY, f64::min);
    let y_hi = all.iter().map(|p| p.y).fold(f64::NEG_INFINITY, f64::max);
    let y_axis = value_axis(y_lo, y_hi, include_zero, 5);
    let x_min = all.iter().map(|p| p.x).fold(f64::INFINITY, f64::min);
    let x_max = all.iter().map(|p| p.x).fold(f64::NEG_INFINITY, f64::max);
    let x_axis = scatter.then(|| value_axis(x_min, x_max, false, 5));

    let ox: f32 = bounds.origin.x.into();
    let oy: f32 = bounds.origin.y.into();
    let w: f32 = bounds.size.width.into();
    let h: f32 = bounds.size.height.into();

    // Measure y labels to size the left gutter.
    let y_labels: Vec<String> = y_axis.ticks.iter().map(|t| format_compact(*t)).collect();
    let label_w = y_labels.iter().map(|l| l.chars().count()).max().unwrap_or(0) as f32 * 6.2;
    let (left, right, top, bottom) = (label_w + 10.0, 6.0, 6.0, 18.0);
    let pw = w - left - right;
    let ph = h - top - bottom;
    if pw <= 0.0 || ph <= 0.0 {
        return;
    }
    let py = |v: f64| -> f32 {
        oy + top + ph - ((v - y_axis.min) / (y_axis.max - y_axis.min)) as f32 * ph
    };
    let slots = if bars {
        let mut xs: Vec<i64> = all.iter().map(|p| (p.x * 1000.0) as i64).collect();
        xs.sort_unstable();
        xs.dedup();
        xs.len().max(1)
    } else {
        0
    };
    let px_of = |x: f64| -> f32 {
        if let Some(ax) = &x_axis {
            return ox + left + ((x - ax.min) / (ax.max - ax.min)) as f32 * pw;
        }
        if x_max == x_min {
            return ox + left + pw / 2.0;
        }
        let inset = if bars { pw / slots as f32 / 2.0 } else { 0.0 };
        ox + left + inset + ((x - x_min) / (x_max - x_min)) as f32 * (pw - inset * 2.0)
    };

    // Grid + y labels.
    for (i, t) in y_axis.ticks.iter().enumerate() {
        let y = py(*t);
        stroke_path(&[(ox + left, y), (ox + left + pw, y)], 1.0, theme.border, window);
        let lw = y_labels[i].chars().count() as f32 * 6.2;
        paint_text(
            &y_labels[i],
            point(px(ox + left - 6.0 - lw), px(y - 6.0)),
            theme.text_tertiary,
            window,
            cx,
        );
    }

    // X tick labels: numeric ticks for scatter, point labels (thinned) otherwise.
    let ticks: Vec<(f32, String)> = if let Some(ax) = &x_axis {
        ax.ticks.iter().map(|t| (px_of(*t), format_compact(*t))).collect()
    } else {
        let reference = chart
            .series
            .iter()
            .find(|s| !s.points.is_empty())
            .map(|s| s.points.as_slice())
            .unwrap_or(&[]);
        let step = (reference.len() / 6).max(1);
        reference
            .iter()
            .enumerate()
            .filter(|(i, _)| i % step == 0)
            .map(|(_, p)| {
                (
                    px_of(p.x),
                    p.label.clone().unwrap_or_else(|| format_compact(p.x)),
                )
            })
            .collect()
    };
    let mut last_end = f32::MIN;
    for (x, text) in ticks {
        let tw = text.chars().count() as f32 * 6.2;
        let start = (x - tw / 2.0).clamp(ox, ox + w - tw);
        if start > last_end + 4.0 {
            paint_text(
                &text,
                point(px(start), px(oy + top + ph + 3.0)),
                theme.text_tertiary,
                window,
                cx,
            );
            last_end = start + tw;
        }
    }

    let zero_y = py(0f64.clamp(y_axis.min, y_axis.max));
    let buckets = pw as usize;
    for (si, s) in chart.series.iter().enumerate() {
        let color = series_color(theme, si);
        let pts = if scatter || bars {
            s.points.clone()
        } else {
            min_max_buckets(&s.points, buckets.max(1))
        };
        match chart.kind.as_str() {
            "scatter" => {
                for p in &pts {
                    let (cx_, cy_) = (px_of(p.x), py(p.y));
                    let r = 3.0;
                    window.paint_quad(
                        gpui::fill(
                            Bounds::new(
                                point(px(cx_ - r), px(cy_ - r)),
                                gpui::size(px(r * 2.0), px(r * 2.0)),
                            ),
                            color,
                        )
                        .corner_radii(px(r)),
                    );
                }
            }
            "bar" => {
                let per = pw / slots as f32;
                let bw = (24f32).min(per * 0.7 / chart.series.len() as f32);
                for p in &pts {
                    let cx_ = px_of(p.x)
                        + (si as f32 - (chart.series.len() as f32 - 1.0) / 2.0) * bw;
                    let y = py(p.y);
                    let (top_y, height) = (y.min(zero_y), (zero_y - y).abs().max(1.0));
                    window.paint_quad(gpui::fill(
                        Bounds::new(
                            point(px(cx_ - bw / 2.0), px(top_y)),
                            gpui::size(px(bw), px(height)),
                        ),
                        color,
                    ));
                }
            }
            _ => {
                let xy: Vec<(f32, f32)> = pts.iter().map(|p| (px_of(p.x), py(p.y))).collect();
                if xy.len() == 1 {
                    let (x, y) = xy[0];
                    window.paint_quad(
                        gpui::fill(
                            Bounds::new(
                                point(px(x - 3.0), px(y - 3.0)),
                                gpui::size(px(6.0), px(6.0)),
                            ),
                            color,
                        )
                        .corner_radii(px(3.0)),
                    );
                } else if xy.len() > 1 {
                    if chart.kind == "area" {
                        let mut b = PathBuilder::fill();
                        b.move_to(point(px(xy[0].0), px(zero_y)));
                        for (x, y) in &xy {
                            b.line_to(point(px(*x), px(*y)));
                        }
                        b.line_to(point(px(xy[xy.len() - 1].0), px(zero_y)));
                        b.close();
                        if let Ok(path) = b.build() {
                            window.paint_path(path, color.opacity(0.12));
                        }
                    }
                    stroke_path(&xy, 1.5, color, window);
                }
            }
        }
    }
}
