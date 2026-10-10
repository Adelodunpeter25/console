use console_core::ForkSessionDto;

#[test]
fn fork_dto_sends_camel_case_message_id() {
    let dto = ForkSessionDto {
        message_id: "m1".into(),
    };
    let json = serde_json::to_value(&dto).expect("serialize fork dto");
    assert_eq!(json, serde_json::json!({"messageId": "m1"}));
}
