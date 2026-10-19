//! Rendering of the server-computed output throughput shown in the assistant
//! message footer. The Go server owns the measurement, the gating and the
//! arithmetic (see `loop/turnOnce`); this only turns the number into a label.

/// Format a turn's output throughput as `"42.1 tok/s"`.
///
/// `None` when the server could not measure the turn — a provider that
/// reported no usage, a turn with no output, or a stream too short to average
/// over. The footer omits the metric entirely in that case rather than
/// showing a misleading zero.
pub fn format_tokens_per_second(tok_per_sec: Option<f64>) -> Option<String> {
    let rate = tok_per_sec?;
    if !rate.is_finite() || rate <= 0.0 {
        return None;
    }
    Some(format!("{rate:.1} tok/s"))
}
