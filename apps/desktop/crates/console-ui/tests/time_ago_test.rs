use console_ui::utils::format_time_ago;

fn seconds_ago(seconds: i64) -> i64 {
    chrono::Utc::now().timestamp() - seconds
}

#[test]
fn just_now_has_no_suffix() {
    assert_eq!(format_time_ago(seconds_ago(0)), "just now");
    assert_eq!(format_time_ago(seconds_ago(59)), "just now");
}

#[test]
fn compact_units_include_ago() {
    assert_eq!(format_time_ago(seconds_ago(60)), "1m ago");
    assert_eq!(format_time_ago(seconds_ago(5 * 60)), "5m ago");
    assert_eq!(format_time_ago(seconds_ago(3 * 3600)), "3h ago");
    assert_eq!(format_time_ago(seconds_ago(2 * 86400)), "2d ago");
}

#[test]
fn old_timestamps_render_as_a_bare_date() {
    let old = format_time_ago(seconds_ago(8 * 86400));
    assert!(!old.contains("ago"), "date should not carry 'ago': {old:?}");
    assert_eq!(old.len(), 6, "expected a 'Mon DD' date: {old:?}");
}

#[test]
fn milliseconds_are_normalized() {
    assert_eq!(format_time_ago(seconds_ago(5 * 60) * 1000), "5m ago");
}
