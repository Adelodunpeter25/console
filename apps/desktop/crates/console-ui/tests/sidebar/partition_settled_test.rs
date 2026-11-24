use console_ui::utils::{group_indices_by_project, partition_settled};

#[test]
fn partition_keeps_active_in_order_and_sorts_settled_newest_first() {
    // settled_at per position; position 3 is active again.
    let settled_at = |index: usize| [9_000, 1_000, 5_000, 0, 7_000][index];
    let is_settled = |index: usize| index != 3;

    let (active, settled) = partition_settled(5, is_settled, settled_at);

    assert_eq!(active, vec![3]);
    // Most recently settled first: 0 (9000), 4 (7000), 2 (5000), 1 (1000).
    assert_eq!(settled, vec![0, 4, 2, 1]);
}

#[test]
fn partition_with_nothing_settled_puts_everything_in_active() {
    let (active, settled) = partition_settled(3, |_| false, |_| 42);

    assert_eq!(active, vec![0, 1, 2]);
    assert!(settled.is_empty());
}

#[test]
fn partition_with_all_settled_leaves_active_empty() {
    let (active, settled) = partition_settled(2, |_| true, |index| index as i64);

    assert!(active.is_empty());
    assert_eq!(settled, vec![1, 0]);
}

#[test]
fn group_indices_by_project_sorts_chats_by_activity_and_projects_by_created_at() {
    // Project A has a newer chat (9000) than Project B (5000), and None has 9999.
    // Project B was added more recently (created_at 200 > 100), so B comes first,
    // A second, and None always last.
    let keys = [
        Some("proj-a".to_string()),
        Some("proj-b".to_string()),
        Some("proj-a".to_string()),
        None,
    ];
    let timestamps = [1_000, 5_000, 9_000, 9_999];
    let created_at = |id: &str| match id {
        "proj-a" => 100,
        "proj-b" => 200,
        _ => i64::MIN,
    };

    let grouped = group_indices_by_project(
        keys.len(),
        |i| keys[i].clone(),
        |i| timestamps[i],
        created_at,
    );

    assert_eq!(
        grouped,
        vec![
            (Some("proj-b".to_string()), vec![1]),
            (Some("proj-a".to_string()), vec![2, 0]),
            (None, vec![3]),
        ]
    );
}
