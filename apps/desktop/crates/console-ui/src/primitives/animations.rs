//! Animation curves, interpolation utilities, and animated scrolling helpers.

use gpui::{ListOffset, Pixels, px};
use std::time::Duration;

/// Default duration for quick, responsive scroll animations.
pub const DEFAULT_SCROLL_DURATION: Duration = Duration::from_millis(180);

/// Interval between animation ticks (~60 fps).
pub const ANIMATION_FRAME_INTERVAL: Duration = Duration::from_millis(16);

/// Cubic ease-out curve: starts quickly and smoothly decelerates to the end.
pub fn ease_out_cubic(t: f32) -> f32 {
    let t = t.clamp(0.0, 1.0);
    1.0 - (1.0 - t).powi(3)
}

/// Quadratic ease-out curve.
pub fn ease_out_quad(t: f32) -> f32 {
    let t = t.clamp(0.0, 1.0);
    1.0 - (1.0 - t).powi(2)
}

/// Cubic ease-in-out curve.
pub fn ease_in_out_cubic(t: f32) -> f32 {
    let t = t.clamp(0.0, 1.0);
    if t < 0.5 {
        4.0 * t * t * t
    } else {
        1.0 - (-2.0 * t + 2.0).powi(3) / 2.0
    }
}

/// Linear interpolation between two floats.
pub fn lerp(start: f32, end: f32, t: f32) -> f32 {
    start + (end - start) * t.clamp(0.0, 1.0)
}

/// Linear interpolation between two pixel values.
pub fn lerp_px(start: Pixels, end: Pixels, t: f32) -> Pixels {
    let s = f32::from(start);
    let e = f32::from(end);
    px(lerp(s, e, t))
}

/// Compute intermediate `ListOffset` between a start offset and an end item index.
pub fn interpolate_list_offset(
    start: ListOffset,
    target_item_ix: usize,
    target_offset_in_item: Pixels,
    progress: f32,
) -> ListOffset {
    let progress = ease_out_cubic(progress);

    let start_ix = start.item_ix as f32;
    let target_ix = target_item_ix as f32;
    let current_ix = lerp(start_ix, target_ix, progress).round() as usize;

    let offset = lerp_px(start.offset_in_item, target_offset_in_item, progress);

    ListOffset {
        item_ix: current_ix,
        offset_in_item: offset,
    }
}
