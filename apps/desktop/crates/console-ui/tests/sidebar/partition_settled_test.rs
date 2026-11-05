use console_ui::utils::partition_settled;

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
