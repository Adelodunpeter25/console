//! Transparency checkerboard backdrop for image previews.
//!
//! Rasterized SVGs (and transparent PNGs) composite onto a transparent
//! pixmap, so dark artwork vanishes against dark surfaces in dark mode.
//! Painting the standard light checkerboard behind the image keeps every
//! pixel visible in both themes, and also communicates transparency to the
//! viewer — the same treatment Figma, Photoshop, and GitHub use.
//!
//! The grid is theme-independent (fixed light cells) by design: a
//! theme-derived backdrop just moves the invisibility problem to the other
//! theme. Opaque images cover the grid entirely, so it is safe to render
//! unconditionally with no alpha-channel sniffing.
//!
//! Usage: the parent must be `relative()` (the grid positions itself
//! absolute/inset-0 behind the image) and ideally `overflow_hidden()` with
//! matching rounding so cell edges clip to the card.

use gpui::{
    App, BorderStyle, Bounds, IntoElement, Pixels, Styled, Window, canvas, px, quad, rgb,
    transparent_black,
};

/// Checker cell size in pixels.
const CELL: f32 = 12.0;

/// Paint the checkerboard backdrop. Add as the first child of a `relative()`
/// image card, before the `img` element.
pub fn transparency_grid() -> impl IntoElement {
    canvas(
        |_, _, _| (),
        move |bounds: Bounds<Pixels>, _, window: &mut Window, _: &mut App| {
            let origin_x = f32::from(bounds.origin.x);
            let origin_y = f32::from(bounds.origin.y);
            let width = f32::from(bounds.size.width);
            let height = f32::from(bounds.size.height);
            if width <= 0.0 || height <= 0.0 {
                return;
            }

            let cols = (width / CELL).ceil() as i32;
            let rows = (height / CELL).ceil() as i32;
            // Base coat is the gray cell (via the element's own bg); only the
            // light cells are painted, halving the quad count.
            let light = rgb(0xf5f5f5);

            for row in 0..rows {
                for col in 0..cols {
                    if (row + col) % 2 != 0 {
                        continue;
                    }
                    let x = origin_x + col as f32 * CELL;
                    let y = origin_y + row as f32 * CELL;
                    // Clamp the trailing cells to the bounds so no quad
                    // bleeds past the card (parents clip anyway, but a
                    // non-clipped ancestor would show the overhang).
                    let w = (x + CELL).min(origin_x + width) - x;
                    let h = (y + CELL).min(origin_y + height) - y;
                    if w <= 0.0 || h <= 0.0 {
                        continue;
                    }
                    window.paint_quad(quad(
                        Bounds::new(gpui::point(px(x), px(y)), gpui::size(px(w), px(h))),
                        px(0.0),
                        light,
                        px(0.0),
                        transparent_black(),
                        BorderStyle::default(),
                    ));
                }
            }
        },
    )
    .absolute()
    .inset_0()
    .bg(rgb(0xcccccc))
}
