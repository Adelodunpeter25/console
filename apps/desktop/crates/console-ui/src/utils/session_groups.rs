//! Calendar-period grouping for history lists (sessions, …).
//!
//! Mirrors Waku's sidebar bucketing: items are grouped by their **local
//! calendar day** relative to today — Today, Yesterday, This Week, This
//! Month, Older. Grouping is by calendar period, not by an hour cutoff, so
//! "13 hours ago" is Today when it falls on today's date (e.g. late evening)
//! and Yesterday when it crosses midnight (e.g. early morning).

use chrono::{DateTime, Datelike, Days, Local, NaiveDate, Utc};

/// The calendar bucket a history item falls into, in display order.
#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
pub enum SessionDateGroup {
    Today,
    Yesterday,
    ThisWeek,
    ThisMonth,
    Older,
}

impl SessionDateGroup {
    /// All buckets in display order: most recent first.
    pub const ALL: [Self; 5] = [
        Self::Today,
        Self::Yesterday,
        Self::ThisWeek,
        Self::ThisMonth,
        Self::Older,
    ];

    /// Position in [`Self::ALL`].
    pub fn index(self) -> usize {
        match self {
            Self::Today => 0,
            Self::Yesterday => 1,
            Self::ThisWeek => 2,
            Self::ThisMonth => 3,
            Self::Older => 4,
        }
    }

    /// Section-header label.
    pub fn label(self) -> &'static str {
        match self {
            Self::Today => "Today",
            Self::Yesterday => "Yesterday",
            Self::ThisWeek => "This Week",
            Self::ThisMonth => "This Month",
            Self::Older => "Older",
        }
    }
}

/// The group for a Unix timestamp (seconds or milliseconds) relative to
/// `today` in the local timezone.
pub fn session_date_group(timestamp: i64, today: NaiveDate) -> SessionDateGroup {
    // Backend timestamps arrive in milliseconds; normalize to seconds.
    let seconds = if timestamp > 10_000_000_000 {
        timestamp / 1000
    } else {
        timestamp
    };
    let session_date = DateTime::<Utc>::from_timestamp(seconds, 0)
        .map(|timestamp| timestamp.with_timezone(&Local).date_naive())
        .unwrap_or(today);
    session_date_group_for_dates(session_date, today)
}

/// The group for an item's local calendar date relative to `today`.
pub fn session_date_group_for_dates(session_date: NaiveDate, today: NaiveDate) -> SessionDateGroup {
    if session_date >= today {
        return SessionDateGroup::Today;
    }

    if today.pred_opt() == Some(session_date) {
        return SessionDateGroup::Yesterday;
    }

    let week_start = today
        .checked_sub_days(Days::new(today.weekday().num_days_from_monday().into()))
        .unwrap_or(today);
    if session_date >= week_start {
        return SessionDateGroup::ThisWeek;
    }

    if session_date.year() == today.year() && session_date.month() == today.month() {
        return SessionDateGroup::ThisMonth;
    }

    SessionDateGroup::Older
}

/// Bucket `items` by their date group, in [`SessionDateGroup::ALL`] order.
/// Items within a group are sorted by `timestamp` descending (most recent
/// first), and empty groups are omitted.
pub fn group_by_date<T>(
    items: Vec<T>,
    timestamp: impl Fn(&T) -> i64,
) -> Vec<(SessionDateGroup, Vec<T>)> {
    let today = Local::now().date_naive();
    let mut buckets: [Vec<T>; SessionDateGroup::ALL.len()] = Default::default();
    for item in items {
        buckets[session_date_group(timestamp(&item), today).index()].push(item);
    }

    let mut grouped = Vec::new();
    for (index, group) in SessionDateGroup::ALL.into_iter().enumerate() {
        let mut group_items = std::mem::take(&mut buckets[index]);
        if group_items.is_empty() {
            continue;
        }
        group_items.sort_by_key(|item| std::cmp::Reverse(timestamp(item)));
        grouped.push((group, group_items));
    }
    grouped
}

/// Bucket item *positions* `0..len` by date group, in
/// [`SessionDateGroup::ALL`] order, positions within a group sorted most
/// recent first. Like [`group_by_date`], but moves no payloads: callers keep
/// the shared item collection and resolve each position on demand, so only
/// the rows actually rendered (the virtualized visible ones) clone data.
pub fn group_indices_by_date(
    len: usize,
    timestamp: impl Fn(usize) -> i64,
) -> Vec<(SessionDateGroup, Vec<usize>)> {
    let today = Local::now().date_naive();
    let mut buckets: [Vec<usize>; SessionDateGroup::ALL.len()] = Default::default();
    for index in 0..len {
        buckets[session_date_group(timestamp(index), today).index()].push(index);
    }

    let mut grouped = Vec::new();
    for (bucket_index, group) in SessionDateGroup::ALL.into_iter().enumerate() {
        let mut positions = std::mem::take(&mut buckets[bucket_index]);
        if positions.is_empty() {
            continue;
        }
        positions.sort_by_key(|&index| std::cmp::Reverse(timestamp(index)));
        grouped.push((group, positions));
    }
    grouped
}
