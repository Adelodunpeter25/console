use console_ui::utils::format_compact_count;

#[test]
fn small_numbers_are_unchanged() {
    assert_eq!(format_compact_count(0), "0");
    assert_eq!(format_compact_count(129), "129");
    assert_eq!(format_compact_count(999), "999");
}

#[test]
fn thousands_round_to_one_decimal() {
    assert_eq!(format_compact_count(1_000), "1k");
    assert_eq!(format_compact_count(1_150), "1.2k");
    assert_eq!(format_compact_count(1_639), "1.6k");
    assert_eq!(format_compact_count(4_279), "4.3k");
    assert_eq!(format_compact_count(27_000), "27k");
}

#[test]
fn rounding_up_never_reads_as_a_thousand_k() {
    assert_eq!(format_compact_count(999_949), "999.9k");
    assert_eq!(format_compact_count(999_950), "1M");
}

#[test]
fn millions_use_an_m_suffix() {
    assert_eq!(format_compact_count(1_000_000), "1M");
    assert_eq!(format_compact_count(1_250_000), "1.3M");
}
