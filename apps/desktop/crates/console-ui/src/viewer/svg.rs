//! SVG rasterization using resvg and usvg.
//!
//! Converts SVG data to PNG bytes for display in `gpui::Image`.
//! Scripts and external network resources are inert / disabled by usvg construction.
//! Animated SVGs render their first frame only.

/// Rasterize SVG bytes into PNG bytes and dimensions (width, height).
///
/// Output dimensions are scaled to fit `max_dim` (e.g. 2048) preserving aspect ratio,
/// rendering at up to 2x for retina crispness.
/// Returns `None` if parsing or rasterizing fails.
pub fn rasterize_svg(bytes: &[u8], max_dim: u32) -> Option<(Vec<u8>, u32, u32)> {
    if bytes.is_empty() || max_dim == 0 {
        return None;
    }

    let opt = usvg::Options::default();
    let tree = usvg::Tree::from_data(bytes, &opt).ok()?;

    let orig_w = tree.size().width();
    let orig_h = tree.size().height();

    if orig_w <= 0.0 || orig_h <= 0.0 || !orig_w.is_finite() || !orig_h.is_finite() {
        return None;
    }

    // Attempt 2x rendering for retina crispness, capped at max_dim
    let retina_scale = 2.0f32;
    let max_orig = orig_w.max(orig_h);
    let scale = if max_orig * retina_scale > max_dim as f32 {
        (max_dim as f32) / max_orig
    } else {
        retina_scale
    };

    let out_w = ((orig_w * scale).round() as u32).clamp(1, max_dim);
    let out_h = ((orig_h * scale).round() as u32).clamp(1, max_dim);

    let mut pixmap = tiny_skia::Pixmap::new(out_w, out_h)?;
    let transform = tiny_skia::Transform::from_scale(out_w as f32 / orig_w, out_h as f32 / orig_h);

    resvg::render(&tree, transform, &mut pixmap.as_mut());

    let png_bytes = pixmap.encode_png().ok()?;
    Some((png_bytes, out_w, out_h))
}
