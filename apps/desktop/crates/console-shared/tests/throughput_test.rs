//! Unit tests for the footer's throughput label. The measurement (tokens over
//! provider-active stream time) and its gating live on the Go server; this
//! covers only the number-to-text step.

use console_shared::throughput::format_tokens_per_second;

#[test]
fn formats_one_decimal_with_unit() {
    assert_eq!(
        format_tokens_per_second(Some(42.06)).as_deref(),
        Some("42.1 tok/s")
    );
    assert_eq!(
        format_tokens_per_second(Some(120.0)).as_deref(),
        Some("120.0 tok/s")
    );
}

#[test]
fn keeps_sub_one_rates_visible() {
    // A long, sparse turn is still a real measurement.
    assert_eq!(
        format_tokens_per_second(Some(0.4)).as_deref(),
        Some("0.4 tok/s")
    );
}

#[test]
fn omits_unmeasurable_turns() {
    assert_eq!(format_tokens_per_second(None), None);
    assert_eq!(format_tokens_per_second(Some(0.0)), None);
    assert_eq!(format_tokens_per_second(Some(-3.0)), None);
    assert_eq!(format_tokens_per_second(Some(f64::NAN)), None);
    assert_eq!(format_tokens_per_second(Some(f64::INFINITY)), None);
}
