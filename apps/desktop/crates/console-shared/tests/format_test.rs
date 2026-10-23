//! Display formatting for model metadata.

use console_shared::format::{format_context_window, format_model_name};

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

#[test]
fn model_names_are_split_and_capitalised() {
    assert_eq!(format_model_name("claude-sonnet-4_5"), "Claude Sonnet 4 5");
    assert_eq!(format_model_name("gpt-5"), "Gpt 5");
    assert_eq!(format_model_name(""), "");
}
