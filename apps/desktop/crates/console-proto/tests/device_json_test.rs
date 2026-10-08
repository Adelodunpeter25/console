use console_proto::{
    DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics, DeviceStreamMeta,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/device")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_list_fixture() {
    let devs: Vec<DeviceDescriptor> =
        serde_json::from_str(fixture("list.json").trim()).expect("decode list fixture");
    assert_eq!(devs.len(), 2);
    assert_eq!(devs[0].id, "Pixel_7");
    assert_eq!(devs[0].platform, "android");
    assert_eq!(devs[0].state, "shutdown");
    // Absent optionals read as None, never "".
    assert!(devs[0].os_version.is_none());
    assert!(devs[0].model.is_none());
    assert!(devs[0].is_available);
    assert_eq!(devs[1].state, "booted");
    assert_eq!(devs[1].os_version.as_deref(), Some("26.3"));
}

#[test]
fn decodes_golden_diagnostics_fixture() {
    // diskFreeBytes is uint64, so protojson sends it as a string; a u64 field
    // parses that back losslessly.
    let msg: DeviceDiagnostics =
        serde_json::from_str(fixture("diagnostics.json").trim()).expect("decode diagnostics");
    assert!(msg.xcode_installed);
    assert!(msg.simctl_available);
    assert!(msg.android_sdk_found);
    assert!(msg.adb_available);
    assert!(msg.emulator_available);
    assert_eq!(msg.disk_free_bytes, 111_201_239_040);
    assert!(msg.has_enough_disk_space);
    assert!(msg.xcode_version.is_none());
    assert!(msg.errors.is_empty());
}

#[test]
fn decodes_golden_stream_meta_fixture() {
    let msg: DeviceStreamMeta =
        serde_json::from_str(fixture("stream_meta.json").trim()).expect("decode meta");
    assert_eq!(msg.r#type, "meta");
    assert_eq!(msg.width, 1170);
    assert_eq!(msg.height, 2532);
    assert_eq!(msg.name, "iPad (A16)");
    assert_eq!(msg.codec, "avc1.42E01E");
}

#[test]
fn decodes_golden_action_fixture() {
    let msg: DeviceActionRequest =
        serde_json::from_str(fixture("action.json").trim()).expect("decode action");
    assert_eq!(msg.action, "swipe");
    assert!((msg.x.unwrap() - 100.5).abs() < f64::EPSILON);
    assert!((msg.y.unwrap() - 200.25).abs() < f64::EPSILON);
    assert_eq!(msg.duration_ms, Some(250));
    // Unused coordinates stay absent so the server's pixel() defaults apply.
    assert!(msg.text.is_none());
    assert!(msg.key.is_none());
    assert!(msg.appearance.is_none());
}

#[test]
fn ignores_unknown_fields() {
    let devs: Vec<DeviceDescriptor> = serde_json::from_str(
        r#"[{"id":"a","name":"b","platform":"ios","state":"booted","isAvailable":true,"futureField":"x"}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(devs[0].id, "a");
    assert!(devs[0].is_available);
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&DeviceActionRequest {
        action: "tap".into(),
        x: Some(1.5),
        y: Some(2.0),
        end_x: None,
        end_y: None,
        duration_ms: None,
        text: None,
        key: None,
        appearance: None,
    })
    .expect("encode action");
    assert!(raw.contains(r#""action":"tap""#), "unexpected encoding: {raw}");
    assert!(raw.contains(r#""x":1.5"#), "unexpected encoding: {raw}");
}