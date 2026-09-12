//! Tests for SVG rasterization helper.

use console_ui::viewer::rasterize_svg;

#[test]
fn test_rasterize_valid_svg() {
    let svg_data = br#"<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50">
        <rect width="100" height="50" fill="red"/>
    </svg>"#;

    let result = rasterize_svg(svg_data, 2048);
    assert!(result.is_some(), "Expected valid SVG to rasterize");

    let (png_bytes, width, height) = result.unwrap();
    // 2x retina scale for 100x50 gives 200x100
    assert_eq!(width, 200);
    assert_eq!(height, 100);
    // Valid PNG signature: \x89PNG\r\n\x1a\n
    assert!(png_bytes.len() > 8);
    assert_eq!(
        &png_bytes[0..8],
        &[0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]
    );
}

#[test]
fn test_rasterize_malformed_svg() {
    let bad_data = b"This is not XML or SVG";
    let result = rasterize_svg(bad_data, 2048);
    assert!(result.is_none(), "Expected malformed data to return None");
}

#[test]
fn test_rasterize_empty_data() {
    let result = rasterize_svg(b"", 2048);
    assert!(result.is_none(), "Expected empty data to return None");
}

#[test]
fn test_rasterize_scaling_bounds() {
    let svg_data = br#"<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="500">
        <circle cx="500" cy="250" r="200" fill="blue"/>
    </svg>"#;

    let max_dim = 200;
    let result = rasterize_svg(svg_data, max_dim);
    assert!(result.is_some());

    let (_, width, height) = result.unwrap();
    assert!(
        width <= max_dim,
        "width {} exceeds max_dim {}",
        width,
        max_dim
    );
    assert!(
        height <= max_dim,
        "height {} exceeds max_dim {}",
        height,
        max_dim
    );
    assert_eq!(width, 200);
    assert_eq!(height, 100);
}
