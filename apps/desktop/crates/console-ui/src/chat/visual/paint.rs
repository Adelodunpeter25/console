//! Canvas painter: grid, axis labels, and the line/area/bar/scatter series.
//! Axis math and point bucketing come from `console-core`.

use console_core::{VisualChart, format_compact, min_max_buckets, value_axis};
use gpui::{
    App, Bounds, FontWeight, Hsla, PathBuilder, Pixels, SharedString, TextAlign, TextRun, Window,
    point, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::theme::Theme;

pub(super) fn series_color(theme: Theme, i: usize) -> Hsla {
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

fn stroke_path(pts: &[(f32, f32)], width: f32, color: Hsla, window: &mut Window) {
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

pub(super) fn paint_chart(
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
    let label_w = y_labels
        .iter()
        .map(|l| l.chars().count())
        .max()
        .unwrap_or(0) as f32
        * 6.2;
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
        stroke_path(
            &[(ox + left, y), (ox + left + pw, y)],
            1.0,
            theme.border,
            window,
        );
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
        ax.ticks
            .iter()
            .map(|t| (px_of(*t), format_compact(*t)))
            .collect()
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
                    let cx_ =
                        px_of(p.x) + (si as f32 - (chart.series.len() as f32 - 1.0) / 2.0) * bw;
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
