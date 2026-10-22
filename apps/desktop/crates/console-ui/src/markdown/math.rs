//! LaTeX → image through RaTeX (<https://github.com/erweixin/RaTeX>).
//!
//! RaTeX is a pure-Rust KaTeX-compatible engine: parse → layout → display list
//! → raster. The same engine backs the Android client, so a formula typesets
//! identically on both. The raster is a transparent PNG already painted in the
//! theme's text colour, which `gpui` decodes and draws as an ordinary image.
//!
//! Rasterizing is pure CPU, so results are cached by formula, mode, size and
//! colour: a settled message re-renders every frame and must not re-typeset.

use std::collections::HashMap;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::sync::{Arc, Mutex, OnceLock};

use gpui::{Image, ImageFormat};
use ratex_layout::{LayoutOptions, layout, to_display_list};
use ratex_parser::parse;
use ratex_render::{RenderOptions, render_to_png};
use ratex_types::{Color, MathStyle};

/// Pixel density formulas are rasterized at. gpui scales the image down to its
/// logical size, so this only controls sharpness on high-DPI displays.
const RASTER_DPR: f32 = 2.0;

/// Formulas kept before the cache is dropped wholesale. A transcript rarely
/// holds more than a few dozen distinct formulas.
const CACHE_LIMIT: usize = 256;

/// Inline formulas sit in a line of text, display formulas in their own block.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum MathMode {
    Inline,
    Display,
}

/// A typeset formula and its logical (pre-DPR) size in px.
#[derive(Debug)]
pub struct MathImage {
    pub image: Arc<Image>,
    pub width: f32,
    pub height: f32,
}

type CacheKey = (String, MathMode, u32, [u8; 3]);

fn cache() -> &'static Mutex<HashMap<CacheKey, Option<Arc<MathImage>>>> {
    static CACHE: OnceLock<Mutex<HashMap<CacheKey, Option<Arc<MathImage>>>>> = OnceLock::new();
    CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

/// Typeset `latex` at `font_size` px/em in `rgb` (0–255). `None` when RaTeX
/// cannot parse or lay it out — notably a half-streamed formula — so the caller
/// can fall back to the source text.
pub fn render(latex: &str, mode: MathMode, font_size: f32, rgb: [u8; 3]) -> Option<Arc<MathImage>> {
    let latex = latex.trim();
    if latex.is_empty() {
        return None;
    }
    let key = (latex.to_owned(), mode, font_size.to_bits(), rgb);
    if let Some(hit) = cache().lock().ok()?.get(&key) {
        return hit.clone();
    }
    // The engine is third-party code running on the UI thread; a layout panic
    // must degrade to the source text, never take the window down.
    let rendered = catch_unwind(AssertUnwindSafe(|| rasterize(latex, mode, font_size, rgb)))
        .ok()
        .flatten()
        .map(Arc::new);
    let mut cache = cache().lock().ok()?;
    if cache.len() >= CACHE_LIMIT {
        cache.clear();
    }
    cache.insert(key, rendered.clone());
    rendered
}

fn rasterize(latex: &str, mode: MathMode, font_size: f32, rgb: [u8; 3]) -> Option<MathImage> {
    let color = Color::new(
        f32::from(rgb[0]) / 255.0,
        f32::from(rgb[1]) / 255.0,
        f32::from(rgb[2]) / 255.0,
        1.0,
    );
    let nodes = parse(latex).ok()?;
    let options = LayoutOptions {
        style: match mode {
            MathMode::Inline => MathStyle::Text,
            MathMode::Display => MathStyle::Display,
        },
        color,
        ..LayoutOptions::default()
    };
    let display_list = to_display_list(&layout(&nodes, &options));
    let render_options = RenderOptions {
        font_size,
        padding: match mode {
            MathMode::Inline => 1.0,
            MathMode::Display => 4.0,
        },
        // Transparent, so the formula blends into any theme.
        background_color: Color::new(0.0, 0.0, 0.0, 0.0),
        device_pixel_ratio: RASTER_DPR,
        ..RenderOptions::default()
    };
    let png = render_to_png(&display_list, &render_options).ok()?;
    let (width, height) = png_size(&png)?;
    Some(MathImage {
        image: Arc::new(Image::from_bytes(ImageFormat::Png, png)),
        width: width as f32 / RASTER_DPR,
        height: height as f32 / RASTER_DPR,
    })
}

/// Pixel size from a PNG's IHDR chunk (big-endian u32s at byte 16 and 20).
fn png_size(png: &[u8]) -> Option<(u32, u32)> {
    if png.len() < 24 || &png[1..4] != b"PNG" {
        return None;
    }
    let width = u32::from_be_bytes(png[16..20].try_into().ok()?);
    let height = u32::from_be_bytes(png[20..24].try_into().ok()?);
    (width > 0 && height > 0).then_some((width, height))
}

/// Reserved text for an inline formula `width` px wide at `em` px/em.
///
/// The text layout has no notion of an inline image, so — like the link icons —
/// the flattener reserves a run of em/en spaces and the renderer paints the
/// formula over it. An em space is one em wide and an en space half of that, so
/// the width is rounded *up* to the next half em: a little slack after the
/// formula is better than it overlapping the next word.
pub fn gap_for_width(width: f32, em: f32) -> String {
    let half_em = (em.max(1.0)) / 2.0;
    let halves = ((width / half_em).ceil() as usize).max(1);
    let mut gap = "\u{2003}".repeat(halves / 2);
    if halves % 2 == 1 {
        gap.push('\u{2002}');
    }
    gap
}
