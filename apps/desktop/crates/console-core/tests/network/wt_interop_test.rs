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
use console_core::services::WtService;
use console_core::types::{AgentSessionEvent, RunPromptDto, TerminalSpawnParams};
use console_core::utils::HttpTransport;
use futures_util::StreamExt;
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
    eprintln!(
        "skipped {test}: set WT_TEST_BASE_URL/WT_TEST_TOKEN/WT_TEST_CERT_HASH for live WT interop"
    );
    None
}

async fn live_session(test: &str) -> Option<WtSession> {
    let Some(cfg) = live_config() else {
        skipped(test);
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

/// Connect the service layer the way the app does: mint its own token over
/// the API origin (`WT_TEST_API_URL`) and dial the WT origin. Needs all four
/// env vars; the API origin is what proves token minting end to end.
async fn live_service(test: &str) -> Option<WtService> {
    let Some(cfg) = live_config() else {
        skipped(test);
        return None;
    };
    let api_url = std::env::var("WT_TEST_API_URL").ok()?;
    let transport = HttpTransport::new(Some(api_url));
    Some(
        WtService::connect(
            &transport,
            Some(&cfg.base_url),
            WtTls::PinnedHashes(vec![cfg.cert_hash]),
        )
        .await
        .expect("live WtService::connect failed"),
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
        request: Some(console_proto::wt_chat_run_request::Request::Start(
            console_proto::WtChatRunStart {
                text: "  ".into(),
                ..Default::default()
            },
        )),
    };
    let payload = serde_json::to_vec(&req).expect("protojson chat start request");
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
    let payload = serde_json::to_vec(&req).expect("protojson spawn request");
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
    assert!(end.is_none(), "killed terminal closes the stream cleanly");
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
    let payload = serde_json::to_vec(&req).expect("protojson watch request");
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

// ── Service layer: mint + dial + typed decode, as the app does it ────────

/// First stream item, bounded so a server that never answers fails instead
/// of hanging the suite.
async fn first_item<S, T>(mut stream: S) -> Result<T, String>
where
    S: futures_util::Stream<Item = anyhow::Result<T>> + Unpin,
{
    match tokio::time::timeout(Duration::from_secs(15), stream.next()).await {
        Ok(Some(Ok(item))) => Ok(item),
        Ok(Some(Err(error))) => Err(format!("{error:#}")),
        Ok(None) => Err("stream ended without an item".into()),
        Err(_) => Err("timed out waiting for a stream item".into()),
    }
}

#[tokio::test]
async fn service_start_run_rejects_unknown_session() {
    let Some(service) = live_service("service_start_run_rejects_unknown_session").await else {
        skipped("service_start_run_rejects_unknown_session");
        return;
    };
    let prompt = RunPromptDto {
        prompt: "hello".into(),
        context_files: None,
        model_id: None,
        provider: None,
        approval_mode: None,
        thinking_level: None,
        attachments: None,
    };
    let events = service
        .start_run("wt-interop-missing-session", &prompt)
        .await
        .expect("start_run opens the stream");
    let error = first_item(events)
        .await
        .err()
        .expect("unknown session answers an error frame");
    assert!(
        error.contains("not found"),
        "error says the session is missing: {error}"
    );
}

#[tokio::test]
async fn service_attach_run_without_active_run_errors() {
    let Some(service) = live_service("service_attach_run_without_active_run_errors").await else {
        skipped("service_attach_run_without_active_run_errors");
        return;
    };
    let events = service
        .attach_run("wt-interop-nope", None)
        .await
        .expect("attach_run opens the stream");
    let error = first_item(events)
        .await
        .err()
        .expect("attaching to a dead run answers an error frame");
    assert!(
        error.contains("No active run"),
        "error explains there is no run: {error}"
    );
}

#[tokio::test]
async fn service_start_run_rejects_blank_prompt() {
    let Some(service) = live_service("service_start_run_rejects_blank_prompt").await else {
        skipped("service_start_run_rejects_blank_prompt");
        return;
    };
    let prompt = RunPromptDto {
        prompt: "   ".into(),
        context_files: None,
        model_id: None,
        provider: None,
        approval_mode: None,
        thinking_level: None,
        attachments: None,
    };
    let events = service
        .start_run("wt-interop-nope", &prompt)
        .await
        .expect("start_run opens the stream");
    let error = first_item::<_, AgentSessionEvent>(events)
        .await
        .err()
        .expect("blank prompt answers an error frame");
    assert!(
        error.contains("prompt"),
        "error names the missing prompt: {error}"
    );
}

/// Next terminal event, bounded so a server that stops answering fails the
/// case instead of hanging the suite.
async fn next_terminal_event(
    terminal: &mut console_core::services::WtTerminalHandle,
) -> Result<Option<console_core::services::WtTerminalEvent>, String> {
    match tokio::time::timeout(Duration::from_secs(15), terminal.next_event()).await {
        Ok(Ok(event)) => Ok(event),
        Ok(Err(error)) => Err(format!("{error:#}")),
        Err(_) => Err("timed out waiting for a terminal event".into()),
    }
}

#[tokio::test]
async fn service_terminal_round_trip() {
    let Some(service) = live_service("service_terminal_round_trip").await else {
        skipped("service_terminal_round_trip");
        return;
    };
    let params = TerminalSpawnParams {
        cwd: std::env::temp_dir().to_string_lossy().into_owned(),
        shell: Some("/bin/sh".into()),
        ..Default::default()
    };
    let mut terminal = service
        .spawn_terminal(&params, false)
        .await
        .expect("spawn_terminal opens the stream");

    let spawned = next_terminal_event(&mut terminal)
        .await
        .expect("read spawned within 15s")
        .expect("spawned event");
    let id = match spawned {
        console_core::services::WtTerminalEvent::Spawned(spawned) => spawned.id,
        other => panic!("first event must be spawned, got {other:?}"),
    };
    assert!(!id.is_empty(), "spawned carries a terminal id");

    terminal
        .send_input(b"echo wt-ok\n")
        .await
        .expect("send input");
    terminal.resize(100, 30).await.expect("resize");

    // The echoed command proves input, resize and output all crossed the
    // stream; the end of stream proves kill reaped the PTY.
    let mut saw_output = false;
    for _ in 0..20 {
        match next_terminal_event(&mut terminal)
            .await
            .expect("read terminal event within 15s")
        {
            Some(console_core::services::WtTerminalEvent::Output(output)) => {
                if output.data.contains("wt-ok") {
                    saw_output = true;
                    break;
                }
            }
            Some(console_core::services::WtTerminalEvent::Error(error)) => {
                panic!("terminal error: {error}")
            }
            Some(_) => continue,
            None => break,
        }
    }
    assert!(saw_output, "terminal echoes the input it received");

    terminal.kill().await.expect("kill");
    let mut saw_end = false;
    for _ in 0..10 {
        match next_terminal_event(&mut terminal)
            .await
            .expect("read terminal event within 15s")
        {
            Some(console_core::services::WtTerminalEvent::Exit(exit)) => {
                assert!(exit.code.is_some(), "exit always carries a code");
                saw_end = true;
                break;
            }
            Some(console_core::services::WtTerminalEvent::Error(error)) => {
                panic!("terminal error: {error}")
            }
            Some(_) => continue,
            None => {
                saw_end = true;
                break;
            }
        }
    }
    assert!(saw_end, "killed terminal ends the stream");
}

#[tokio::test]
async fn service_watch_ports_snapshot_decodes() {
    let Some(service) = live_service("service_watch_ports_snapshot_decodes").await else {
        skipped("service_watch_ports_snapshot_decodes");
        return;
    };
    let mut ports = service
        .watch_ports(None)
        .await
        .expect("watch_ports opens the stream");
    let snapshot = first_item(&mut ports)
        .await
        .expect("ports watch sends a snapshot first");
    // Nothing may be listening, so only the decode itself is asserted.
    let _ = snapshot;
}

#[tokio::test]
async fn service_watch_fs_requires_path() {
    let Some(service) = live_service("service_watch_fs_requires_path").await else {
        skipped("service_watch_fs_requires_path");
        return;
    };
    let mut events = service
        .watch_fs("")
        .await
        .expect("watch_fs opens the stream");
    let error = first_item(&mut events)
        .await
        .err()
        .expect("blank path answers an error frame");
    assert!(
        error.contains("path"),
        "error names the missing path: {error}"
    );
}
