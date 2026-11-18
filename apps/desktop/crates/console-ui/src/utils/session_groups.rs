//! Grouping helpers for the sidebar session list.

/// A project section key: `Some(id)` for a known project, `None` for
/// sessions without one (or whose project is unknown). Sessions resolve
/// display names against the project list; unknown ids fall into `None`.
pub type ProjectSectionKey = Option<String>;

/// Split item positions `0..len` into active and settled. Active keeps list
/// order; settled is sorted most recently settled first. Callers keep the
/// shared item collection and resolve each position on demand.
pub fn partition_settled(
    len: usize,
    is_settled: impl Fn(usize) -> bool,
    settled_at: impl Fn(usize) -> i64,
) -> (Vec<usize>, Vec<usize>) {
    let mut active = Vec::new();
    let mut settled = Vec::new();
    for index in 0..len {
        if is_settled(index) {
            settled.push(index);
        } else {
            active.push(index);
        }
    }
    settled.sort_by_key(|&index| std::cmp::Reverse(settled_at(index)));
    (active, settled)
}

/// Bucket item *positions* `0..len` by project section, ordered by each
/// section's most recent activity (newest section first), positions within a
/// section sorted most recent first. Sections with no positions are omitted.
/// Callers keep the shared item collection and resolve each position on demand.
pub fn group_indices_by_project(
    len: usize,
    project_key: impl Fn(usize) -> ProjectSectionKey,
    timestamp: impl Fn(usize) -> i64,
) -> Vec<(ProjectSectionKey, Vec<usize>)> {
    use std::collections::HashMap;
    let mut buckets: HashMap<ProjectSectionKey, Vec<usize>> = HashMap::new();
    for index in 0..len {
        buckets.entry(project_key(index)).or_default().push(index);
    }
    let mut grouped: Vec<(ProjectSectionKey, Vec<usize>, i64)> = buckets
        .into_iter()
        .map(|(key, mut positions)| {
            positions.sort_by_key(|&index| std::cmp::Reverse(timestamp(index)));
            let latest = positions
                .first()
                .map(|&index| timestamp(index))
                .unwrap_or(i64::MIN);
            (key, positions, latest)
        })
        .collect();
    // Most recently active section first; `None` (no project) always last.
    grouped.sort_by(|(key_a, _, latest_a), (key_b, _, latest_b)| {
        match (key_a.is_none(), key_b.is_none()) {
            (true, false) => std::cmp::Ordering::Greater,
            (false, true) => std::cmp::Ordering::Less,
            _ => latest_b.cmp(latest_a),
        }
    });
    grouped
        .into_iter()
        .map(|(key, positions, _)| (key, positions))
        .collect()
}
