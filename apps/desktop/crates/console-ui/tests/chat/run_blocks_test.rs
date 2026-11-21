//! Tests for server-assembled turn run blocks and per-turn file changes.

use console_core::types::SessionFileChange;
use console_core::types::agent::{AgentMessage, AssistantContentPart, ToolCall};
use console_core::{ActivityEvent, ToolCallEntry, ToolResult, Turn, TurnRun};
use console_ui::chat::transcript_view::run_block_for_turn;
use console_ui::chat::turn_changes::{group_turn_changes, turn_changes_for_turn};

fn user_turn(id: &str, user_id: &str, created_at: i64, run: Option<TurnRun>) -> Turn {
    Turn {
        id: id.to_string(),
        created_at: Some(created_at),
        user: Some(AgentMessage::User {
            id: Some(user_id.to_string()),
            content: "run the job".to_string(),
            attachments: None,
            context_files: None,
            annotations: None,
            created_at: Some(created_at),
            approval_mode: None,
            source: None,
        }),
        wake_label: None,
        run,
        responses: Vec::new(),
    }
}

fn wake_turn(id: &str, label: &str, created_at: i64, run: Option<TurnRun>) -> Turn {
    Turn {
        id: id.to_string(),
        created_at: Some(created_at),
        user: None,
        wake_label: Some(label.to_string()),
        run,
        responses: Vec::new(),
    }
}

fn tool_event(call_id: &str, with_result: bool) -> ActivityEvent {
    ActivityEvent::ToolCall(Box::new(ToolCallEntry {
        call: ToolCall {
            id: call_id.to_string(),
            name: "bash".to_string(),
            arguments: serde_json::Value::Null,
            thought_signature: None,
            label: None,
            kind_flags: None,
            icon: None,
        },
        result: with_result.then(|| ToolResult {
            tool_call_id: call_id.to_string(),
            tool_name: Some("bash".to_string()),
            content: serde_json::Value::Null,
            is_error: Some(false),
            visual: None,
            html: None,
            omitted_size: 0,
            result_message_id: None,
        }),
    }))
}

fn assistant_text(text: &str) -> AgentMessage {
    AgentMessage::Assistant {
        id: None,
        content: vec![AssistantContentPart::Text {
            text: text.to_string(),
            thought_signature: None,
        }],
        stop_reason: None,
        created_at: Some(230),
        tok_per_sec: None,
    }
}

#[test]
fn wake_turn_carries_its_own_run_block_and_divider() {
    let t1 = user_turn(
        "t1",
        "u1",
        100,
        Some(TurnRun {
            run_id: "t1".to_string(),
            started_at: Some(100),
            elapsed_ms: 20_000,
            working: false,
            events: vec![tool_event("c1", true)],
        }),
    );
    let t2 = wake_turn(
        "t2",
        "Bash tool completed, continuing agent",
        200,
        Some(TurnRun {
            run_id: "t2".to_string(),
            started_at: Some(200),
            elapsed_ms: 15_000,
            working: false,
            events: vec![tool_event("c2", true)],
        }),
    );
    let b1 = run_block_for_turn(&t1, false, false).expect("first block");
    let b2 = run_block_for_turn(&t2, true, false).expect("wake block");
    assert_eq!(b1.run_id, "t1");
    assert_eq!(b1.divider_label, None);
    assert_eq!(b1.elapsed, 20);
    assert_eq!(b2.run_id, "t2");
    assert_eq!(
        b2.divider_label.as_deref(),
        Some("Bash tool completed, continuing agent")
    );
    assert_eq!(b2.started_at, Some(200));
    assert_eq!(b2.elapsed, 15);
}

#[test]
fn working_flag_only_on_last_turn_while_streaming() {
    let t1 = user_turn(
        "t1",
        "u1",
        100,
        Some(TurnRun {
            run_id: "t1".to_string(),
            started_at: Some(100),
            elapsed_ms: 10_000,
            working: false,
            events: vec![tool_event("c1", true)],
        }),
    );
    let t2 = wake_turn(
        "t2",
        "Bash tool completed, continuing agent",
        200,
        Some(TurnRun {
            run_id: "t2".to_string(),
            started_at: Some(200),
            elapsed_ms: 0,
            working: true,
            events: vec![tool_event("c2", false)],
        }),
    );
    let b1 = run_block_for_turn(&t1, false, true).expect("first block");
    let b2 = run_block_for_turn(&t2, true, true).expect("second block");
    assert!(!b1.working);
    assert!(b2.working);
}

#[test]
fn run_without_tool_calls_emits_no_block() {
    let t = wake_turn(
        "t2",
        "Bash tool completed, continuing agent",
        200,
        Some(TurnRun {
            run_id: "t2".to_string(),
            started_at: Some(200),
            elapsed_ms: 0,
            working: false,
            events: vec![ActivityEvent::Text {
                id: "e1".to_string(),
                text: "All done.".to_string(),
            }],
        }),
    );
    assert!(run_block_for_turn(&t, true, false).is_none());
}

#[test]
fn turn_changes_attach_only_to_final_turn_across_wake_boundary() {
    let mut t1 = user_turn("t1", "u1", 100, None);
    t1.responses.push(assistant_text("Applied fixes, waiting for background check."));
    let mut t2 = wake_turn("t2", "Background work completed, continuing agent", 200, None);
    t2.responses.push(assistant_text("Checks passed."));
    let turns = vec![t1, t2];

    let changes = vec![SessionFileChange {
        path: "src/lib.rs".to_string(),
        turn_index: 0,
        status: "modified".to_string(),
        additions: 3,
        deletions: 1,
        diff_text: None,
        reviewed: false,
        updated_at: 125,
        user_message_id: "u1".to_string(),
    }];
    let grouped = group_turn_changes(&changes);

    assert!(turn_changes_for_turn(&turns, &grouped, 0).is_none());
    let final_turn = turn_changes_for_turn(&turns, &grouped, 1)
        .expect("final turn should carry turn changes");
    assert_eq!(final_turn.files.len(), 1);
    assert_eq!(final_turn.files[0].path, "src/lib.rs");
}
