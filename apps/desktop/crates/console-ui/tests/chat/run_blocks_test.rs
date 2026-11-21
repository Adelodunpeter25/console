//! Tests for wake-turn run-block splitting: a `RunBoundary` marker (server
//! wake turn for finished background work) must produce a second run block
//! under the same user message, with its own timer and divider label, instead
//! of merging into the previous turn's activity.

use console_core::types::SessionFileChange;
use console_core::types::agent::{AgentMessage, AssistantContentPart, ToolCall};
use console_core::{ActivityEvent, ToolResult};
use console_ui::chat::row::is_hidden_tool_transport;
use console_ui::chat::turn_changes::{group_turn_changes, turn_changes_for_assistant};
use console_ui::chat::{compute_run_blocks, compute_run_blocks_with_boundaries};

fn user(created_at: Option<i64>) -> AgentMessage {
    AgentMessage::User {
        id: Some("u1".to_string()),
        content: "run the job".to_string(),
        attachments: None,
        context_files: None,
        annotations: None,
        created_at,
        approval_mode: None,
        source: None,
    }
}

fn assistant_with_tool(created_at: i64, call_id: &str) -> AgentMessage {
    AgentMessage::Assistant {
        id: None,
        content: vec![AssistantContentPart::ToolCall {
            call: ToolCall {
                id: call_id.to_string(),
                name: "bash".to_string(),
                arguments: serde_json::Value::Null,
                thought_signature: None,
                label: None,
                kind_flags: None,
                icon: None,
            },
        }],
        stop_reason: None,
        created_at: Some(created_at),
        tok_per_sec: None,
    }
}

fn tool_result(call_id: &str, created_at: i64) -> AgentMessage {
    AgentMessage::ToolResult {
        results: vec![ToolResult {
            tool_call_id: call_id.to_string(),
            tool_name: Some("bash".to_string()),
            content: serde_json::Value::Null,
            is_error: Some(false),
            visual: None,
            html: None,
            omitted_size: 0,
            result_message_id: None,
        }],
        created_at: Some(created_at),
    }
}

fn boundary(label: &str, at: i64) -> AgentMessage {
    AgentMessage::RunBoundary {
        label: label.to_string(),
        at: Some(at),
    }
}

#[test]
fn wake_turn_produces_second_block() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        tool_result("c1", 120),
        boundary("Bash tool completed, continuing agent", 200),
        assistant_with_tool(210, "c2"),
        tool_result("c2", 220),
    ];
    let blocks = compute_run_blocks(&messages, 0, false, None).expect("blocks");
    assert_eq!(blocks.len(), 2);
    assert_eq!(blocks[0].run_id, "run-0");
    assert_eq!(blocks[0].divider_label, None);
    assert_eq!(blocks[0].boundary_index, None);
    assert_eq!(blocks[0].started_at, Some(100));
    assert_eq!(blocks[1].run_id, "run-0-wake-1");
    assert_eq!(
        blocks[1].divider_label.as_deref(),
        Some("Bash tool completed, continuing agent")
    );
    assert_eq!(blocks[1].boundary_index, Some(3));
    // The wake segment's timer seeds from the boundary, not the prompt.
    assert_eq!(blocks[1].started_at, Some(200));
}

#[test]
fn working_flag_only_on_last_block_while_streaming() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        tool_result("c1", 120),
        boundary("Bash tool completed, continuing agent", 200),
        assistant_with_tool(210, "c2"),
    ];
    let blocks = compute_run_blocks(&messages, 0, true, Some(200)).expect("blocks");
    assert_eq!(blocks.len(), 2);
    assert!(!blocks[0].working);
    assert!(blocks[1].working);
    assert_eq!(blocks[1].started_at, Some(200));
}

#[test]
fn trailing_boundary_means_nothing_working() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        boundary("Bash tool completed, continuing agent", 200),
    ];
    let blocks = compute_run_blocks(&messages, 0, true, Some(200)).expect("blocks");
    assert_eq!(blocks.len(), 1);
    assert!(!blocks[0].working);
}

#[test]
fn segment_without_tool_calls_emits_no_block() {
    let messages = vec![
        user(Some(100)),
        boundary("Bash tool completed, continuing agent", 200),
        // Wake turn produced only text — no run block for it.
        AgentMessage::Assistant {
            id: None,
            content: vec![AssistantContentPart::Text {
                text: "All done.".to_string(),
                thought_signature: None,
            }],
            stop_reason: None,
            created_at: Some(210),
            tok_per_sec: None,
        },
    ];
    assert!(compute_run_blocks(&messages, 0, false, None).is_none());
}

#[test]
fn next_user_message_still_terminates() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        boundary("Bash tool completed, continuing agent", 200),
        assistant_with_tool(210, "c2"),
        AgentMessage::User {
            id: Some("u2".to_string()),
            content: "next".to_string(),
            attachments: None,
            context_files: None,
            annotations: None,
            created_at: Some(300),
            approval_mode: None,
            source: None,
        },
        assistant_with_tool(310, "c3"),
    ];
    let blocks = compute_run_blocks(&messages, 0, false, None).expect("blocks");
    assert_eq!(blocks.len(), 2);
    // The second user message's activity is a separate call.
    let next = compute_run_blocks(&messages, 4, false, None).expect("next blocks");
    assert_eq!(next.len(), 1);
    assert_eq!(next[0].run_id, "run-4");
}

#[test]
fn tool_results_stay_in_their_own_segment() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        boundary("Bash tool completed, continuing agent", 200),
        assistant_with_tool(210, "c2"),
        tool_result("c1", 220), // late result lands in the wake segment
        tool_result("c2", 230),
    ];
    let blocks = compute_run_blocks(&messages, 0, false, None).expect("blocks");
    assert_eq!(blocks.len(), 2);
    let result_of = |block: &console_ui::chat::RunActivityBlock, call_id: &str| {
        block
            .events
            .iter()
            .find_map(|e| match e {
                ActivityEvent::ToolCall(entry) if entry.call.id == call_id => {
                    Some(entry.result.is_some())
                }
                _ => None,
            })
            .unwrap_or(false)
    };
    // c1's result arrived after the boundary, so it joined the wake segment's
    // result map and cannot attach to the first segment's call.
    assert!(!result_of(&blocks[0], "c1"));
    assert!(result_of(&blocks[1], "c2"));
}

fn assistant_text(created_at: i64, text: &str) -> AgentMessage {
    AgentMessage::Assistant {
        id: None,
        content: vec![AssistantContentPart::Text {
            text: text.to_string(),
            thought_signature: None,
        }],
        stop_reason: None,
        created_at: Some(created_at),
        tok_per_sec: None,
    }
}

#[test]
fn reconstructed_wake_boundaries_split_reloaded_history() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        tool_result("c1", 120),
        assistant_text(130, "Waiting for background check."),
        assistant_with_tool(210, "c2"),
        tool_result("c2", 220),
        assistant_text(230, "All done."),
    ];
    let boundaries = vec![(
        4,
        "Background work completed, continuing agent".to_string(),
        Some(210),
    )];
    let blocks =
        compute_run_blocks_with_boundaries(&messages, 0, false, None, &boundaries).expect("blocks");
    assert_eq!(blocks.len(), 2);
    assert_eq!(blocks[0].boundary_index, None);
    assert_eq!(blocks[0].divider_label, None);
    assert_eq!(blocks[1].boundary_index, Some(4));
    assert_eq!(
        blocks[1].divider_label.as_deref(),
        Some("Background work completed, continuing agent")
    );
    assert_eq!(blocks[1].started_at, Some(210));
}

#[test]
fn streaming_wake_text_does_not_reopen_previous_block() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        tool_result("c1", 120),
        assistant_text(130, "Waiting for background check."),
        boundary("Background work completed, continuing agent", 200),
        assistant_text(210, "Checking results..."),
    ];
    let blocks = compute_run_blocks(&messages, 0, true, Some(200)).expect("blocks");
    assert_eq!(blocks.len(), 1);
    assert!(!blocks[0].working);
}

#[test]
fn turn_changes_attach_only_to_final_assistant_across_wake_boundary() {
    let messages = vec![
        user(Some(100)),
        assistant_with_tool(110, "c1"),
        tool_result("c1", 120),
        assistant_text(130, "Applied fixes, waiting for background check."),
        boundary("Background work completed, continuing agent", 200),
        assistant_with_tool(210, "c2"),
        tool_result("c2", 220),
        assistant_text(230, "Checks passed."),
    ];
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

    assert!(turn_changes_for_assistant(&messages, &grouped, 3, is_hidden_tool_transport).is_none());
    let final_turn = turn_changes_for_assistant(&messages, &grouped, 7, is_hidden_tool_transport)
        .expect("final assistant message should carry turn changes");
    assert_eq!(final_turn.files.len(), 1);
    assert_eq!(final_turn.files[0].path, "src/lib.rs");
}
