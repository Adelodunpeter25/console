//! Live interop against a real Go WT server (opt-in).
//!
//! Proves the Rust client speaks the protocol: handshake + auth, request
//! framing both directions, and server-byte decode (spawned/exit/error
//! oneofs, watch snapshots). Needs no provider credentials — every case
//! stays on control/error paths.
//!
//! Run a WT-enabled server first (`make dev-server-wt`), mint a token, then:
//! ```sh
//! WT_TEST_BASE_URL=https://127.0.0.1:14443 \
//! WT_TEST_TOKEN=ct1_... \
//! WT_TEST_CERT_HASH=$(openssl x509 -in ~/.console-dev/wt-dev.crt -noout -fingerprint -sha256 | cut -d= -f2) \
//! cargo test -p console-core --test wt_interop_test
//! ```
//! Absent env skips every case so plain `cargo test` stays green.

use console_core::net::{WtClient, WtSession, WtStreamKind, WtTls};
use std::time::Duration;

struct LiveConfig {
    base_url: String,
    token: String,
    cert_hash: [u8; 32],
}

fn live_config() -> Option<LiveConfig> {
    let base_url = std::env::var("WT_TEST_BASE_URL").ok()?;
    let token = std::env::var("WT_TEST_TOKEN").ok()?;
    let fingerprint = std::env::var("WT_TEST_CERT_HASH").ok()?;
    let clean: String = fingerprint.chars().filter(|c| *c != ':').collect();
    if clean.len() != 64 {
        return None;
    }
    let mut cert_hash = [0u8; 32];
    for (i, byte) in cert_hash.iter_mut().enumerate() {
        *byte = u8::from_str_radix(&clean[2 * i..2 * i + 2], 16).ok()?;
    }
    Some(LiveConfig {
        base_url,
        token,
        cert_hash,
    })
}

fn skipped(test: &str) -> Option<WtSession> {
    eprintln!("skipped {test}: set WT_TEST_BASE_URL/WT_TEST_TOKEN/WT_TEST_CERT_HASH for live WT interop");
    None
}

async fn live_session(test: &str) -> Option<WtSession> {
    let Some(cfg) = live_config() else {
        eprintln!("skipped {test}: set WT_TEST_BASE_URL/WT_TEST_TOKEN/WT_TEST_CERT_HASH for live WT interop");
        return None;
    };
    Some(
        WtClient::connect(
            &cfg.base_url,
            &cfg.token,
            WtTls::PinnedHashes(vec![cfg.cert_hash]),
        )
        .await
        .expect("live WT connect failed"),
    )
}

async fn send_request(
    stream: &mut console_core::net::WtStream,
    frame_type: &str,
    payload: Vec<u8>,
) -> anyhow::Result<()> {
    stream
        .send_frame(&console_proto::WtFrame {
            seq: 0,
            r#type: frame_type.into(),
            payload,
        })
        .await
}

async fn next_frame_timeout(
    stream: &mut console_core::net::WtStream,
    secs: u64,
) -> anyhow::Result<Option<console_proto::WtFrame>> {
    tokio::time::timeout(Duration::from_secs(secs), stream.next_frame())
        .await
        .map_err(|_| anyhow::anyhow!("Timed out waiting for a WT frame."))?
}

#[tokio::test]
async fn handshake_rejects_bad_token() {
    let Some(cfg) = live_config() else {
        skipped("handshake_rejects_bad_token");
        return;
    };
    let result = WtClient::connect(
        &cfg.base_url,
        "ct1_bogus.bogus",
        WtTls::PinnedHashes(vec![cfg.cert_hash]),
    )
    .await;
    assert!(
        result.is_err(),
        "bad token must fail the handshake, not open a session"
    );
}

#[tokio::test]
async fn chat_start_requires_prompt() {
    let Some(session) = live_session("chat_start_requires_prompt").await else {
        skipped("chat_start_requires_prompt");
        return;
    };
    let mut stream = session
        .open_stream(WtStreamKind::ChatRun, "wt-interop-nope")
        .await
        .expect("open chat stream");
    let req = console_proto::WtChatRunRequest {
        session_id: "wt-interop-nope".into(),
        request: Some(
            console_proto::wt_chat_run_request::Request::Start(
                console_proto::WtChatRunStart {
                    text: "  ".into(),
                    ..Default::default()
                },
            ),
        ),
    };
    let payload =
        serde_json::to_vec(&req).expect("protojson chat start request");
    send_request(&mut stream, "start", payload)
        .await
        .expect("send start");
    let frame = next_frame_timeout(&mut stream, 15)
        .await
        .expect("read frame")
        .expect("one error frame, not EOF");
    assert_eq!(frame.r#type, "error", "blank prompt answers error");
    assert!(
        String::from_utf8_lossy(&frame.payload).contains("prompt"),
        "error names the missing prompt: {}",
        String::from_utf8_lossy(&frame.payload)
    );
}

#[tokio::test]
async fn terminal_spawn_and_kill() {
    let Some(session) = live_session("terminal_spawn_and_kill").await else {
        skipped("terminal_spawn_and_kill");
        return;
    };
    let mut stream = session
        .open_stream(WtStreamKind::Terminal, "")
        .await
        .expect("open terminal stream");
    let req = console_proto::WtTerminalRequest {
        cwd: std::env::temp_dir().to_string_lossy().into_owned(),
        shell: Some("/bin/sh".into()),
        ..Default::default()
    };
    let payload =
        serde_json::to_vec(&req).expect("protojson spawn request");
    send_request(&mut stream, "spawn", payload)
        .await
        .expect("send spawn");
    let frame = next_frame_timeout(&mut stream, 15)
        .await
        .expect("read spawned")
        .expect("spawned frame, not EOF");
    assert_eq!(frame.r#type, "spawned");
    let msg: console_proto::TerminalServerMessage =
        serde_json::from_slice(&frame.payload).expect("decode spawned");
    let spawned_id = match msg.event {
        Some(console_proto::terminal_server_message::Event::Spawned(s)) => s.id,
        other => panic!("first frame must be spawned, got {other:?}"),
    };
    assert!(!spawned_id.is_empty(), "spawned carries a terminal id");

    send_request(&mut stream, "kill", Vec::new())
        .await
        .expect("send kill");
    let end = next_frame_timeout(&mut stream, 15)
        .await
        .expect("read close");
    assert!(
        end.is_none(),
        "killed terminal closes the stream cleanly"
    );
}

#[tokio::test]
async fn ports_watch_snapshot() {
    let Some(session) = live_session("ports_watch_snapshot").await else {
        skipped("ports_watch_snapshot");
        return;
    };
    let mut stream = session
        .open_stream(WtStreamKind::Watch, "")
        .await
        .expect("open watch stream");
    let req = console_proto::WtWatchRequest {
        target: console_proto::WtWatchTarget::Ports as i32,
        ..Default::default()
    };
    let payload =
        serde_json::to_vec(&req).expect("protojson watch request");
    send_request(&mut stream, "watch", payload)
        .await
        .expect("send watch");
    let frame = next_frame_timeout(&mut stream, 15)
        .await
        .expect("read snapshot")
        .expect("snapshot frame, not EOF");
    assert_eq!(frame.r#type, "ports");
    let snapshot: serde_json::Value =
        serde_json::from_slice(&frame.payload).expect("snapshot is a JSON array");
    assert!(
        snapshot.is_array(),
        "ports snapshot decodes as an array: {snapshot}"
    );
}
