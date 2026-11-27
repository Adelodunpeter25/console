//! A donut glyph: an optional faint track ring with coloured arcs drawn over
//! it, clockwise from 12 o'clock. Shared by the usage gauge (one arc) and the
//! changed-files pill (added / removed split).

use gpui::{Hsla, IntoElement, PathBuilder, Styled, canvas, point, px};

/// One arc of the ring. start and len are fractions of the full circle, with
/// 0.0 at 12 o'clock.
#[derive(Clone, Copy, Debug)]
pub struct RingArc {
    pub start: f32,
    pub len: f32,
    pub color: Hsla,
}

/// An arc this long is drawn as a closed circle, which an SVG-style arc
/// cannot express with a single segment.
const FULL_CIRCLE: f32 = 0.999;

/// Split a circle between parts sized by weights. Returns one (start, len)
/// pair per weight, in order. Zero weights get (0.0, 0.0). With two or more
/// non-zero parts, a gap separates them and every part keeps at least min, so
/// a tiny share stays visible. A single part fills the whole circle.
pub fn split_arcs(weights: &[f32], gap: f32, min: f32) -> Vec<(f32, f32)> {
    let live = weights.iter().filter(|weight| **weight > 0.0).count();
    let mut arcs = vec![(0.0, 0.0); weights.len()];
    match live {
        0 => return arcs,
        1 => {
            if let Some(index) = weights.iter().position(|weight| *weight > 0.0) {
                arcs[index] = (0.0, 1.0);
            }
            return arcs;
        }
        _ => {}
    }

    let total: f32 = weights.iter().filter(|weight| **weight > 0.0).sum();
    let usable = 1.0 - gap * live as f32;
    let lens: Vec<f32> = weights
        .iter()
        .map(|weight| {
            if *weight > 0.0 {
                (weight / total * usable).max(min)
            } else {
                0.0
            }
        })
        .collect();
    let scale = usable / lens.iter().sum::<f32>();

    let mut cursor = gap / 2.0;
    for (index, len) in lens.iter().enumerate() {
        if *len > 0.0 {
            let len = len * scale;
            arcs[index] = (cursor, len);
            cursor += len + gap;
        }
    }
    arcs
}

/// A size x size ring with the given stroke. The track, when given, is a full
/// circle painted under the arcs.
pub fn ring_chart(
    size: f32,
    stroke: f32,
    track: Option<Hsla>,
    arcs: Vec<RingArc>,
) -> impl IntoElement {
    canvas(
        |_, _, _| (),
        move |bounds, _, window, _| {
            let center = bounds.center();
            let radius = px((size - stroke) / 2.0);

            // A full circle is two 180 degree arcs.
            let full_circle = |builder: &mut PathBuilder| {
                builder.move_to(point(center.x + radius, center.y));
                builder.arc_to(
                    point(radius, radius),
                    px(0.0),
                    false,
                    true,
                    point(center.x - radius, center.y),
                );
                builder.arc_to(
                    point(radius, radius),
                    px(0.0),
                    false,
                    true,
                    point(center.x + radius, center.y),
                );
                builder.close();
            };

            if let Some(track) = track {
                let mut builder = PathBuilder::stroke(px(stroke));
                full_circle(&mut builder);
                if let Ok(path) = builder.build() {
                    window.paint_path(path, track);
                }
            }

            for arc in &arcs {
                let len = arc.len.clamp(0.0, 1.0);
                if len <= 0.0 {
                    continue;
                }
                let mut builder = PathBuilder::stroke(px(stroke));
                if len >= FULL_CIRCLE {
                    full_circle(&mut builder);
                } else {
                    let from = -std::f32::consts::FRAC_PI_2 + arc.start * std::f32::consts::TAU;
                    let to = from + len * std::f32::consts::TAU;
                    builder.move_to(point(
                        center.x + radius * from.cos(),
                        center.y + radius * from.sin(),
                    ));
                    builder.arc_to(
                        point(radius, radius),
                        px(0.0),
                        len > 0.5,
                        true,
                        point(center.x + radius * to.cos(), center.y + radius * to.sin()),
                    );
                }
                if let Ok(path) = builder.build() {
                    window.paint_path(path, arc.color);
                }
            }
        },
    )
    .w(px(size))
    .h(px(size))
    .flex_none()
}
