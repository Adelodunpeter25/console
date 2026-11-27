use console_ui::primitives::split_arcs;

const EPS: f32 = 1e-4;

fn close(a: f32, b: f32) -> bool {
    (a - b).abs() < EPS
}

#[test]
fn no_weight_draws_nothing() {
    assert_eq!(
        split_arcs(&[0.0, 0.0], 0.06, 0.06),
        vec![(0.0, 0.0), (0.0, 0.0)]
    );
}

#[test]
fn single_side_fills_the_whole_ring() {
    assert_eq!(
        split_arcs(&[10.0, 0.0], 0.06, 0.06),
        vec![(0.0, 1.0), (0.0, 0.0)]
    );
    assert_eq!(
        split_arcs(&[0.0, 7.0], 0.06, 0.06),
        vec![(0.0, 0.0), (0.0, 1.0)]
    );
}

#[test]
fn two_sides_share_the_ring_with_a_gap_between_them() {
    let arcs = split_arcs(&[50.0, 50.0], 0.06, 0.06);
    let (a_start, a_len) = arcs[0];
    let (b_start, b_len) = arcs[1];

    assert!(close(a_len, b_len));
    // Both arcs plus two gaps make the full circle.
    assert!(close(a_len + b_len + 0.12, 1.0));
    // The first arc starts half a gap past 12 o'clock, and a full gap separates it from the second.
    assert!(close(a_start, 0.03));
    assert!(close(b_start, a_start + a_len + 0.06));
}

#[test]
fn a_tiny_share_keeps_a_visible_arc() {
    let arcs = split_arcs(&[4279.0, 1.0], 0.06, 0.06);

    assert!(arcs[1].1 >= 0.05);
    assert!(close(arcs[0].1 + arcs[1].1 + 0.12, 1.0));
}

#[test]
fn arcs_follow_the_weight_ratio() {
    let arcs = split_arcs(&[3.0, 1.0], 0.0, 0.0);

    assert!(close(arcs[0].1, 0.75));
    assert!(close(arcs[1].1, 0.25));
    assert!(close(arcs[1].0, 0.75));
}
