//! Short forms of large counts for tight spots like the changed-files pill.

/// 999 -> "999", 1000 -> "1k", 4279 -> "4.3k", 1_250_000 -> "1.3M". Rounds to
/// one decimal, half up, and drops a trailing ".0".
pub fn format_compact_count(n: u32) -> String {
    if n < 1_000 {
        return n.to_string();
    }
    let (tenths, suffix) = if n < 1_000_000 {
        ((n + 50) / 100, "k")
    } else {
        ((n + 50_000) / 100_000, "M")
    };
    // 999_950 rounds up to 1000.0k, which reads better as 1M.
    let (tenths, suffix) = if suffix == "k" && tenths >= 10_000 {
        (10, "M")
    } else {
        (tenths, suffix)
    };
    let (whole, frac) = (tenths / 10, tenths % 10);
    if frac == 0 {
        format!("{whole}{suffix}")
    } else {
        format!("{whole}.{frac}{suffix}")
    }
}
