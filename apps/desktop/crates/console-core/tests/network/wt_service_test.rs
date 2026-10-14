//! Offline unit tests for the WebTransport service's wire contracts: the
//! run-prompt mapping, the origin derivation, and the terminal frame decode.
//!
//! These need no server — they pin the shapes the Go server writes, so a
//! silent mis-decode (the pbjson impl ignores unknown fields, so decoding a
//! wrapped payload into its inner type succeeds with empty data) fails here
//! instead of showing up as an empty terminal.

use console_core::ConsoleClient;
use console_core::services::WtTerminalEvent;
use console_core::services::transport::{
    AUTO_DIAL_TIMEOUT, Transport, TransportMode, tls_from_setting,
};
use console_core::services::wt::{chat_run_start, terminal_event, upgrade_to_https};
use console_core::types::{ImageAttachment, RunPromptDto};
use console_proto::{
    TerminalExit, TerminalOutput, TerminalServerMessage, WtFrame, terminal_server_message,
};

fn frame(frame_type: &str, payload: Vec<u8>) -> WtFrame {
    WtFrame {
        seq: 0,
        r#type: frame_type.into(),
        payload,
    }
}

fn server_message(message: TerminalServerMessage) -> Vec<u8> {
    serde_json::to_vec(&message).expect("encode TerminalServerMessage")
}

fn prompt(prompt: &str) -> RunPromptDto {
    RunPromptDto {
        prompt: prompt.into(),
        context_files: None,
        model_id: None,
        provider: None,
        approval_mode: None,
        thinking_level: None,
        attachments: None,
    }
}

#[test]
fn text_output_unwraps_the_server_message() {
    // The regression this file exists for: decoding the payload into the
    // inner TerminalOutput yields an empty string, not an error.
    let payload = server_message(TerminalServerMessage {
        event: Some(terminal_server_message::Event::Output(TerminalOutput {
            data: "wt-ok\r\n".into(),
        })),
    });
    match terminal_event(false, &frame("output", payload)).expect("decode output") {
        WtTerminalEvent::Output(output) => assert_eq!(output.data, "wt-ok\r\n"),
        other => panic!("expected output, got {other:?}"),
    }
}

#[test]
fn spawned_carries_the_terminal_id() {
    let payload = server_message(TerminalServerMessage {
        event: Some(terminal_server_message::Event::Spawned(
            console_proto::TerminalSpawned {
                id: "term-1".into(),
                pid: 42,
                ..Default::default()
            },
        )),
    });
    match terminal_event(false, &frame("spawned", payload)).expect("decode spawned") {
        WtTerminalEvent::Spawned(spawned) => {
            assert_eq!(spawned.id, "term-1");
            assert_eq!(spawned.pid, 42);
        }
        other => panic!("expected spawned, got {other:?}"),
    }
}

#[test]
fn exit_keeps_its_code() {
    let payload = server_message(TerminalServerMessage {
        event: Some(terminal_server_message::Event::Exit(TerminalExit {
            code: Some(0),
        })),
    });
    match terminal_event(false, &frame("exit", payload)).expect("decode exit") {
        WtTerminalEvent::Exit(exit) => assert_eq!(exit.code, Some(0)),
        other => panic!("expected exit, got {other:?}"),
    }
}

#[test]
fn server_error_message_survives_as_text() {
    let payload = server_message(TerminalServerMessage {
        event: Some(terminal_server_message::Event::Error(
            console_proto::TerminalError {
                message: "Unknown terminal frame type".into(),
            },
        )),
    });
    match terminal_event(false, &frame("error", payload)).expect("decode error") {
        WtTerminalEvent::Error(message) => assert_eq!(message, "Unknown terminal frame type"),
        other => panic!("expected error, got {other:?}"),
    }
}

#[test]
fn binary_output_strips_the_tag_byte() {
    let payload = vec![0x01, b'a', b'b'];
    match terminal_event(true, &frame("output", payload)).expect("decode binary output") {
        WtTerminalEvent::OutputBytes(bytes) => assert_eq!(bytes, b"ab"),
        other => panic!("expected binary output, got {other:?}"),
    }
}

#[test]
fn binary_output_without_the_tag_is_an_error() {
    // Silently decoding this would surface an empty terminal instead.
    let error = terminal_event(true, &frame("output", b"raw".to_vec()))
        .expect_err("untagged binary output must fail");
    assert!(error.to_string().contains("tag byte"), "got: {error}");
}

#[test]
fn unknown_terminal_frame_type_is_rejected() {
    let error = terminal_event(false, &frame("nonsense", Vec::new()))
        .expect_err("unknown frame type must fail");
    assert!(error.to_string().contains("nonsense"), "got: {error}");
}

#[test]
fn chat_run_start_maps_every_prompt_field() {
    let start = chat_run_start(&RunPromptDto {
        prompt: "fix the bug".into(),
        context_files: Some(vec!["src/main.rs".into()]),
        model_id: Some("model-1".into()),
        provider: Some("anthropic".into()),
        approval_mode: Some("acceptEdits".into()),
        thinking_level: Some(console_core::types::ThinkingLevel::High),
        attachments: Some(vec![ImageAttachment {
            data: "base64data".into(),
            mime_type: "image/png".into(),
        }]),
    });
    assert_eq!(start.text, "fix the bug");
    assert_eq!(start.context_files, vec!["src/main.rs"]);
    assert_eq!(start.model_id.as_deref(), Some("model-1"));
    assert_eq!(start.provider.as_deref(), Some("anthropic"));
    assert_eq!(start.approval_mode.as_deref(), Some("acceptEdits"));
    assert_eq!(start.thinking_level.as_deref(), Some("high"));
    assert_eq!(start.attachments.len(), 1);
    assert_eq!(start.attachments[0].mime_type, "image/png");
    assert_eq!(start.attachments[0].data, "base64data");
    assert!(start.annotations.is_empty());
}

#[test]
fn chat_run_start_keeps_unset_prompt_fields_absent() {
    // protojson omits unset optionals, so the server reads them as unset
    // rather than as empty strings that would override its own defaults.
    let start = chat_run_start(&prompt("hello"));
    assert_eq!(start.text, "hello");
    assert!(start.context_files.is_empty());
    assert!(start.model_id.is_none());
    assert!(start.provider.is_none());
    assert!(start.approval_mode.is_none());
    assert!(start.thinking_level.is_none());
    assert!(start.attachments.is_empty());
}

#[test]
fn origin_upgrades_http_and_leaves_tls_alone() {
    assert_eq!(
        upgrade_to_https("http://localhost:3000"),
        "https://localhost:3000"
    );
    assert_eq!(
        upgrade_to_https("https://api.example.com"),
        "https://api.example.com"
    );
}

// ── Transport switch ────────────────────────────────────────────────────

#[test]
fn only_wt_opts_in_to_webtransport() {
    // `auto` is the default: QUIC first, legacy on failure. Only an explicit
    // choice changes that, and an explicit `legacy` is honored.
    assert_eq!(TransportMode::from_setting("wt"), TransportMode::Wt);
    assert_eq!(TransportMode::from_setting("WT"), TransportMode::Wt);
    assert_eq!(TransportMode::from_setting("legacy"), TransportMode::Legacy);
    assert_eq!(TransportMode::from_setting("LEGACY"), TransportMode::Legacy);
    for value in ["auto", "", "websocket", "wtp", "nonsense"] {
        assert_eq!(
            TransportMode::from_setting(value),
            TransportMode::Auto,
            "{value:?} must fall through to auto"
        );
    }
}

#[test]
fn legacy_transport_starts_without_a_session() {
    let transport = Transport::new(TransportMode::Legacy);
    assert!(!transport.is_wt());
}

#[tokio::test]
async fn wt_transport_connects_lazily() {
    // Nothing is dialed at construction: the app starts before its server is
    // reachable, and the first stream forces the handshake.
    let transport = Transport::with_settings(
        TransportMode::Wt,
        Some("https://127.0.0.1:14443".into()),
        console_core::net::WtTls::PinnedHashes(vec![[7u8; 32]]),
    );
    assert!(transport.is_wt());
    assert!(
        transport.session().await.is_none(),
        "no handshake until a stream asks for one"
    );
    // Clones share one session cell, so the app opens one connection.
    assert!(transport.clone().session().await.is_none());
}

/// Auto mode against a server that cannot serve WebTransport must fall back
/// rather than error, and must not pay the dial timeout on every stream.
#[tokio::test]
async fn auto_falls_back_when_webtransport_is_unreachable() {
    // Port 1 is reserved and refuses instantly, so this exercises the
    // fallback decision without needing a server.
    let transport = Transport::with_settings(
        TransportMode::Auto,
        Some("https://127.0.0.1:1".into()),
        console_core::net::WtTls::Native,
    );
    let client = ConsoleClient::new(Some("http://127.0.0.1:1".into()));

    let started = std::time::Instant::now();
    let first = transport
        .connect(&client)
        .await
        .expect("auto downgrades instead of erroring");
    assert!(first.is_none(), "falls back to legacy");
    assert!(
        transport.session().await.is_none(),
        "a failed handshake is never cached as a session"
    );

    // The backoff means the next stream does not dial again.
    let again = transport
        .connect(&client)
        .await
        .expect("second attempt also downgrades");
    assert!(again.is_none());
    assert!(
        started.elapsed() < AUTO_DIAL_TIMEOUT * 2,
        "the retry backoff must keep the second attempt free"
    );
}

/// A plaintext server can never serve WebTransport (it mandates TLS), so auto
/// must not stall the first stream probing one. This is the default `make
/// dev-server` boot.
#[tokio::test]
async fn auto_skips_the_probe_against_a_plaintext_server() {
    let transport =
        Transport::with_settings(TransportMode::Auto, None, console_core::net::WtTls::Native);
    let client = ConsoleClient::new(Some("http://192.0.2.10:3000".into()));

    let started = std::time::Instant::now();
    let session = transport
        .connect(&client)
        .await
        .expect("plaintext origin downgrades without erroring");
    assert!(session.is_none());
    assert!(
        started.elapsed() < AUTO_DIAL_TIMEOUT,
        "no dial attempt against a plaintext origin"
    );
}

/// `wt` is the opposite contract: no silent downgrade, so a broken handshake
/// is visible instead of quietly running on SSE.
#[tokio::test]
async fn wt_mode_reports_failure_instead_of_downgrading() {
    let transport = Transport::with_settings(
        TransportMode::Wt,
        Some("https://127.0.0.1:1".into()),
        console_core::net::WtTls::Native,
    );
    let client = ConsoleClient::new(Some("http://127.0.0.1:1".into()));
    let error = transport
        .connect(&client)
        .await
        .err()
        .expect("wt mode must surface the failure");
    assert!(error.to_string().contains("WebTransport"), "got: {error}");
    assert!(
        transport.session().await.is_none(),
        "no session is cached for a failed handshake"
    );
}

#[test]
fn tls_defaults_to_platform_roots() {
    assert!(matches!(
        tls_from_setting("").expect("unset"),
        console_core::net::WtTls::Native
    ));
    assert!(matches!(
        tls_from_setting("native").expect("native"),
        console_core::net::WtTls::Native
    ));
}

#[test]
fn tls_pins_parse_hex_with_or_without_colons() {
    let plain = "571b8c9e079e80a7cf7e141f3ef1087b42a56e632797438c969cee576ecf93d9";
    let colons = "57:1B:8C:9E:07:9E:80:A7:CF:7E:14:1F:3E:F1:08:7B:42:A5:6E:63:27:97:43:8C:96:9C:EE:57:6E:CF:93:D9";
    let hashes = |setting: String| match tls_from_setting(&setting).expect("pin") {
        console_core::net::WtTls::PinnedHashes(hashes) => hashes,
        console_core::net::WtTls::Native => panic!("expected pins"),
    };
    let plain_hashes = hashes(format!("pin:{plain}"));
    assert_eq!(
        plain_hashes,
        hashes(format!("pin:{colons}")),
        "colons are presentation only"
    );
    assert_eq!(plain_hashes[0][0], 0x57);
}

#[test]
fn tls_pins_reject_bad_input() {
    for bad in [
        "pin:",
        "pin:deadbeef",
        "pin:zz1b8c9e079e80a7cf7e141f3ef1087b42a56e632797438c969cee576ecf93d9",
    ] {
        assert!(
            tls_from_setting(bad).is_err(),
            "{bad:?} should be rejected, not silently ignored"
        );
    }
}
