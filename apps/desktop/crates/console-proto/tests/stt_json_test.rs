use console_proto::{SetSttConfigResponse, SttStatus, SttTranscript};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/stt")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_configured_fixture() {
    let msg: SttStatus =
        serde_json::from_str(fixture("status_configured.json").trim()).expect("decode status");
    // `configured` has explicit presence (optional bool): Some(true) when set.
    assert_eq!(msg.configured, Some(true));
    assert_eq!(msg.source.as_deref(), Some("stored"));
    assert_eq!(
        msg.account_id.as_deref(),
        Some("e32ab932dec1b35e0df9c0ec56ab6c8f")
    );
    assert_eq!(msg.model, "@cf/openai/whisper-large-v3-turbo");
    assert_eq!(msg.updated_at.as_deref(), Some("2026-11-09T10:30:00Z"));
    assert_eq!(msg.max_audio_bytes, 4_194_304);
}

#[test]
fn decodes_unconfigured_fixture() {
    // protojson drops unset optionals — an unconfigured server omits source,
    // accountId and updatedAt, and the UI must read that as "not configured".
    let msg: SttStatus =
        serde_json::from_str(fixture("status_unconfigured.json").trim()).expect("decode status");
    assert_eq!(msg.configured, Some(false));
    assert!(msg.source.is_none());
    assert!(msg.account_id.is_none());
    assert!(msg.updated_at.is_none());
    assert_eq!(msg.model, "@cf/openai/whisper-large-v3-turbo");
    assert_eq!(msg.max_audio_bytes, 4_194_304);
}

#[test]
fn absent_configured_reads_as_not_configured() {
    // A response missing `configured` entirely must not be treated as
    // configured; callers compare against Some(true).
    let msg: SttStatus = serde_json::from_str(
        r#"{"model":"@cf/openai/whisper-large-v3-turbo","maxAudioBytes":4194304}"#,
    )
    .expect("decode status without configured");
    assert_eq!(msg.configured, None);
    assert_ne!(msg.configured, Some(true));
}

#[test]
fn decodes_set_config_response() {
    let body = format!(
        r#"{{"status":{},"removed":false}}"#,
        fixture("status_configured.json").trim()
    );
    let msg: SetSttConfigResponse =
        serde_json::from_str(&body).expect("decode set-config response");
    assert!(!msg.removed);
    let status = msg.status.expect("status present");
    assert_eq!(status.configured, Some(true));
    assert_eq!(
        status.account_id.as_deref(),
        Some("e32ab932dec1b35e0df9c0ec56ab6c8f")
    );
}

#[test]
fn decodes_transcript() {
    let msg: SttTranscript = serde_json::from_str(
        r#"{"text":"hello from audio","model":"@cf/openai/whisper-large-v3-turbo"}"#,
    )
    .expect("decode transcript");
    assert_eq!(msg.text, "hello from audio");
    assert_eq!(msg.model, "@cf/openai/whisper-large-v3-turbo");
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&SttStatus {
        configured: Some(true),
        source: Some("stored".into()),
        account_id: Some("acct".into()),
        model: "@cf/openai/whisper-large-v3-turbo".into(),
        updated_at: Some("2026-11-09T10:30:00Z".into()),
        max_audio_bytes: 4_194_304,
    })
    .expect("encode status");
    assert!(raw.contains("configured"), "unexpected encoding: {raw}");
    assert!(raw.contains("accountId"), "unexpected encoding: {raw}");
    assert!(raw.contains("maxAudioBytes"), "unexpected encoding: {raw}");
    assert!(raw.contains("updatedAt"), "unexpected encoding: {raw}");
}
