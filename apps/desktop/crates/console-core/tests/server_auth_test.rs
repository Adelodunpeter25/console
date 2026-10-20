//! The server-pairing payloads are shared protobuf types, so the desktop
//! client decodes them with the same protojson-named serde impls the rest of
//! the client uses. These cases pin that contract against the golden
//! fixtures in proto/testdata/auth.

use console_core::services::{PairingData, PairingGrantData, ServerDevice};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/auth")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_the_golden_pair_device_fixture() {
    let msg: PairingData =
        serde_json::from_str(fixture("pair_device.json").trim()).expect("decode pair device");
    assert_eq!(msg.token, "ct1_device_token");
    assert_eq!(msg.expires_at, 1762592000000);
    let device = msg.device.as_ref().expect("device reported");
    assert_eq!(device.id, "device-1");
    assert_eq!(device.name, "My desktop");
    assert_eq!(device.platform, "desktop");
}

#[test]
fn decodes_the_golden_pairing_grant_fixture() {
    let msg: PairingGrantData =
        serde_json::from_str(fixture("pairing_grant.json").trim()).expect("decode grant");
    assert_eq!(msg.grant, "cp1_grant");
    assert_eq!(msg.expires_at, 1760000030000);
}

// A device that has never been seen carries no lastSeenAt: the timestamp has
// explicit presence, so "never" must not collapse into "at the epoch".
#[test]
fn decodes_the_golden_server_devices_fixture() {
    let msg: console_core::services::ServerDevicesResponse =
        serde_json::from_str(fixture("server_devices.json").trim()).expect("decode devices");
    assert_eq!(msg.devices.len(), 2);
    let desktop: &ServerDevice = &msg.devices[0];
    assert_eq!(desktop.id, "device-1");
    assert_eq!(desktop.created_at, Some(1760000000000));
    assert_eq!(desktop.last_seen_at, None);
    assert_eq!(desktop.revoked_at, None);
    let phone: &ServerDevice = &msg.devices[1];
    assert_eq!(phone.last_seen_at, Some(1760000099000));
    assert_eq!(phone.revoked_at, Some(1760000100000));
}
