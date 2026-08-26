//! Time-related formatting helpers shared across the UI.

/// Normalize a Unix timestamp from seconds, milliseconds, microseconds, or
/// nanoseconds to seconds. The backend commonly uses milliseconds while local
/// optimistic messages use seconds.
pub fn normalize_unix_timestamp(timestamp: i64) -> Option<i64> {
    let magnitude = timestamp.unsigned_abs();
    if magnitude >= 1_000_000_000_000_000_000 {
        timestamp.checked_div(1_000_000_000)
    } else if magnitude >= 1_000_000_000_000_000 {
        timestamp.checked_div(1_000_000)
    } else if magnitude >= 1_000_000_000_000 {
        timestamp.checked_div(1_000)
    } else {
        Some(timestamp)
    }
}

/// Format a message timestamp for its hover footer in the user's local time.
/// Accepts the timestamp units used by both the backend and optimistic UI.
pub fn format_message_time(timestamp: Option<i64>) -> Option<String> {
    let timestamp = normalize_unix_timestamp(timestamp?)?;
    chrono::DateTime::from_timestamp(timestamp, 0).map(|date| {
        date.with_timezone(&chrono::Local)
            .format("%H:%M")
            .to_string()
    })
}

/// Compact live-elapsed label for the transcript's working indicator:
/// "37s", "1m 5s", "1h 2m" — prose-free so it can tick every second beside
/// the pulsing dots.
pub fn format_working_elapsed(seconds: u64) -> String {
    match seconds {
        0..=59 => format!("{}s", seconds),
        60..=3_599 => {
            let minutes = seconds / 60;
            match seconds % 60 {
                0 => format!("{}m", minutes),
                rest => format!("{}m {}s", minutes, rest),
            }
        }
        _ => {
            let hours = seconds / 3_600;
            match (seconds % 3_600) / 60 {
                0 => format!("{}h", hours),
                minutes => format!("{}h {}m", hours, minutes),
            }
        }
    }
}

/// Compact "how long ago" for the sidebar and other history lists: "just now",
/// then one coarse unit — "5m", "3h", "2d", and a date like "Aug 12" for
/// anything older than a week.
///
/// Accepts Unix timestamps in **seconds or milliseconds** (the backend sends
/// milliseconds), normalizing to seconds internally.
pub fn format_time_ago(timestamp: i64) -> String {
    let ts = normalize_unix_timestamp(timestamp).unwrap_or(timestamp);
    let now = chrono::Utc::now().timestamp();
    let seconds = (now - ts).max(0) as u64;
    match seconds {
        0..=59 => "just now".into(),
        60..=3_599 => format!("{}m", seconds / 60),
        3_600..=86_399 => format!("{}h", seconds / 3_600),
        86_400..=604_799 => format!("{}d", seconds / 86_400),
        _ => {
            if let Some(dt) = chrono::DateTime::from_timestamp(ts, 0) {
                dt.format("%b %d").to_string()
            } else {
                format!("{}d", seconds / 86_400)
            }
        }
    }
}
