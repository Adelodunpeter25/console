use console_core::services::{PairingData, ServerDeviceInfo};

#[test]
fn pairing_wire_types_use_server_camel_case() {
    let data: PairingData = serde_json::from_value(serde_json::json!({
        "device": {"id":"d1","name":"Desktop","platform":"desktop"},
        "token":"ct1_token",
        "expiresAt":123
    })).unwrap();
    assert_eq!(data.device.id, "d1");
    assert_eq!(data.expires_at, 123);
}

#[test]
fn device_wire_types_accept_optional_timestamps() {
    let device: ServerDeviceInfo = serde_json::from_value(serde_json::json!({
        "id":"d1","name":"Desktop","platform":"desktop"
    })).unwrap();
    assert_eq!(device.name, "Desktop");
    assert!(device.last_seen_at.is_none());
}
