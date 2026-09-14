//! Tests for model picker display formatting.

use console_ui::common::model_picker::format_context_window;

#[test]
fn test_million_plus_windows_show_m() {
    assert_eq!(format_context_window(1_000_000), "1M");
    assert_eq!(format_context_window(1_048_576), "1M");
}

#[test]
fn test_smaller_windows_show_k() {
    assert_eq!(format_context_window(272_000), "272k");
    assert_eq!(format_context_window(200_000), "200k");
    assert_eq!(format_context_window(131_072), "131k");
}
