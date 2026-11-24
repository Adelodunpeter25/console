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

/// Bucket item *positions* `0..len` by project section, keeping sections in
/// `project_order` (`None` always last) while sorting positions within each
/// section most recent first. Sections with no positions are omitted.
/// Callers keep the shared item collection and resolve each position on demand.
pub fn group_indices_by_project(
    len: usize,
    project_key: impl Fn(usize) -> ProjectSectionKey,
    timestamp: impl Fn(usize) -> i64,
    project_order: impl Fn(&str) -> usize,
) -> Vec<(ProjectSectionKey, Vec<usize>)> {
    use std::collections::HashMap;
    let mut buckets: HashMap<ProjectSectionKey, Vec<usize>> = HashMap::new();
    for index in 0..len {
        buckets.entry(project_key(index)).or_default().push(index);
    }
    let mut grouped: Vec<(ProjectSectionKey, Vec<usize>)> = buckets
        .into_iter()
        .map(|(key, mut positions)| {
            positions.sort_by_key(|&index| std::cmp::Reverse(timestamp(index)));
            (key, positions)
        })
        .collect();
    // Keep project sections in stable order; `None` (no project) always last.
    grouped.sort_by(|(key_a, _), (key_b, _)| {
        match (key_a.as_deref(), key_b.as_deref()) {
            (None, Some(_)) => std::cmp::Ordering::Greater,
            (Some(_), None) => std::cmp::Ordering::Less,
            (Some(a), Some(b)) => project_order(a).cmp(&project_order(b)).then_with(|| a.cmp(b)),
            (None, None) => std::cmp::Ordering::Equal,
        }
    });
    grouped
}
