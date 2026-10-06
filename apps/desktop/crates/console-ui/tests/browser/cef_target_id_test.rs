use console_ui::browser::cef::client::parse_target_id;

#[test]
fn parses_target_id_from_get_target_info_result() {
    let result = br#"{"targetInfo":{"targetId":"294474B9957CEF672F9BFB2130A4B9D5","type":"page","title":"","url":"about:blank","attached":true}}"#;
    assert_eq!(
        parse_target_id(result).as_deref(),
        Some("294474B9957CEF672F9BFB2130A4B9D5")
    );
}

#[test]
fn rejects_malformed_or_empty_results() {
    assert_eq!(parse_target_id(b""), None);
    assert_eq!(parse_target_id(b"not json"), None);
    assert_eq!(parse_target_id(br#"{"other":1}"#), None);
    assert_eq!(parse_target_id(br#"{"targetInfo":{}}"#), None);
    assert_eq!(parse_target_id(br#"{"targetInfo":{"targetId":""}}"#), None);
    assert_eq!(parse_target_id(br#"{"targetInfo":{"targetId":5}}"#), None);
}
