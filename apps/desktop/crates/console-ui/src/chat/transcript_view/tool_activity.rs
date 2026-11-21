use super::*;

/// One run block for a turn: the tool calls, thinking, and progress text
/// assembled by the server (or streamed into the tail turn).
#[derive(Clone)]
pub struct RunActivityBlock {
    pub events: Vec<ActivityEvent>,
    pub started_at: Option<i64>,
    pub elapsed: u64,
    pub working: bool,
    /// Divider label shown before this block for wake turns; `None` for user turns.
    pub divider_label: Option<String>,
    /// Stable id for `ToolCallsState` disclosure.
    pub run_id: String,
}

pub fn run_block_for_turn(
    turn: &Turn,
    is_last_turn: bool,
    is_streaming: bool,
) -> Option<RunActivityBlock> {
    let run = turn.run.as_ref()?;
    let has_tool_call = run
        .events
        .iter()
        .any(|event| matches!(event, ActivityEvent::ToolCall(_)));
    if !has_tool_call {
        return None;
    }
    let working = is_last_turn && (is_streaming || run.working);
    let started_at = run.started_at.or(turn.created_at);
    let elapsed = if run.elapsed_ms > 0 {
        (run.elapsed_ms / 1000) as u64
    } else {
        0
    };
    Some(RunActivityBlock {
        events: run.events.clone(),
        started_at,
        elapsed,
        working,
        divider_label: turn.wake_label.clone(),
        run_id: run.run_id.clone(),
    })
}

impl TranscriptView {
    pub fn run_block_for_turn(&self, turn_index: usize) -> Option<RunActivityBlock> {
        let turn = self.turns.get(turn_index)?;
        run_block_for_turn(turn, turn_index + 1 == self.turns.len(), self.is_streaming)
    }
}
