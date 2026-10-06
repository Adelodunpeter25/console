use console_proto::{
    TerminalClientMessage, TerminalServerMessage,
    terminal_client_message::Event as ClientEvent,
    terminal_server_message::Event as ServerEvent,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/terminal")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_server_frames() {
    let spawned: TerminalServerMessage =
        serde_json::from_str(fixture("spawned.json").trim()).expect("spawned");
    match spawned.event {
        Some(ServerEvent::Spawned(s)) => {
            assert_eq!(s.id, "t1");
            assert_eq!(s.pid, 123);
            assert_eq!(s.cols, 80);
        }
        _ => panic!("wrong variant"),
    }
    let output: TerminalServerMessage =
        serde_json::from_str(fixture("output.json").trim()).expect("output");
    match output.event {
        Some(ServerEvent::Output(o)) => assert_eq!(o.data, "hi\n"),
        _ => panic!("wrong variant"),
    }
    let exit: TerminalServerMessage =
        serde_json::from_str(fixture("exit.json").trim()).expect("exit");
    match exit.event {
        Some(ServerEvent::Exit(e)) => assert_eq!(e.code, Some(0)),
        _ => panic!("wrong variant"),
    }
    let error: TerminalServerMessage =
        serde_json::from_str(fixture("error.json").trim()).expect("error");
    match error.event {
        Some(ServerEvent::Error(e)) => assert_eq!(e.message, "boom"),
        _ => panic!("wrong variant"),
    }
}

#[test]
fn decodes_golden_client_frames() {
    let input: TerminalClientMessage =
        serde_json::from_str(fixture("input.json").trim()).expect("input");
    match input.event {
        Some(ClientEvent::Input(i)) => assert_eq!(i.data, "ls\n"),
        _ => panic!("wrong variant"),
    }
    let resize: TerminalClientMessage =
        serde_json::from_str(fixture("resize.json").trim()).expect("resize");
    match resize.event {
        Some(ClientEvent::Resize(r)) => {
            assert_eq!(r.cols, 100);
            assert_eq!(r.rows, 30);
        }
        _ => panic!("wrong variant"),
    }
    let kill: TerminalClientMessage =
        serde_json::from_str(fixture("kill.json").trim()).expect("kill");
    assert!(matches!(kill.event, Some(ClientEvent::Kill(_))));
}

#[test]
fn ignores_unknown_fields() {
    let msg: TerminalServerMessage = serde_json::from_str(
        r#"{"spawned":{"id":"t","pid":1,"cwd":"/","shell":"sh","cols":80,"rows":24,"label":"x"}}"#,
    )
    .expect("dropped label must be ignored");
    match msg.event {
        Some(ServerEvent::Spawned(s)) => assert_eq!(s.id, "t"),
        _ => panic!("wrong variant"),
    }
}

#[test]
fn serializes_oneof_shape() {
    let raw = serde_json::to_string(&TerminalClientMessage {
        event: Some(ClientEvent::Resize(console_proto::TerminalResize {
            cols: 100,
            rows: 30,
        })),
    })
    .expect("encode resize");
    assert!(raw.contains(r#""resize":{"cols":100,"rows":30}"#), "unexpected: {raw}");
}
