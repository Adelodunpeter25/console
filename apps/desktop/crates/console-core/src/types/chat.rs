//! Run-activity timeline types shared by the live stream handler and the
//! transcript renderer. Mirrors the desktop app's `types/chat.ts` so thinking
//! and text between tool calls stay in chronological order in one canonical
//! shape.

use super::agent::{AgentMessage, ToolCall, ToolResult};

/// A tool call and the result, when the backend has returned one.
#[derive(Clone, Debug)]
pub struct ToolCallEntry {
    pub call: ToolCall,
    pub result: Option<ToolResult>,
}

/// An ordered step in a run's activity timeline: reasoning, progress text, or
/// a tool call with its result.
#[derive(Clone, Debug)]
pub enum ActivityEvent {
    Text { id: String, text: String },
    Thinking { id: String, text: String },
    ToolCall(Box<ToolCallEntry>),
}

/// Live state of a turn's run. `started_at` is unix seconds.
#[derive(Clone, Debug)]
pub struct TurnRun {
    pub run_id: String,
    pub started_at: Option<i64>,
    pub elapsed_ms: i64,
    pub working: bool,
    pub events: Vec<ActivityEvent>,
}

/// One server-assembled turn: the prompt (or a wake divider), the run
/// activity between, and the final assistant replies.
#[derive(Clone, Debug)]
pub struct Turn {
    pub id: String,
    pub created_at: Option<i64>,
    pub user: Option<AgentMessage>,
    pub wake_label: Option<String>,
    pub run: Option<TurnRun>,
    pub responses: Vec<AgentMessage>,
}

fn to_seconds(ts: i64) -> i64 {
    if ts > 10_000_000_000 { ts / 1000 } else { ts }
}

impl Turn {
    pub fn from_proto(t: console_proto::TranscriptTurn) -> Self {
        use console_proto::run_activity_event::Event;
        let run = t.run.map(|run| TurnRun {
            run_id: run.run_id,
            started_at: Some(to_seconds(run.started_at)),
            elapsed_ms: run.elapsed_ms,
            working: run.status == console_proto::RunStatus::Working as i32,
            events: run
                .events
                .into_iter()
                .filter_map(|event| match event.event? {
                    Event::Thinking(text) => Some(ActivityEvent::Thinking { id: event.id, text }),
                    Event::Text(text) => Some(ActivityEvent::Text { id: event.id, text }),
                    Event::ToolCall(rc) => {
                        let call = ToolCall::from_proto(rc.call.as_ref()?);
                        let result = rc.result.as_ref().map(|r| {
                            let mut result = ToolResult::from_proto(r);
                            result.omitted_size = rc.result_size;
                            if rc.result_size > 0 && !rc.result_message_id.is_empty() {
                                result.result_message_id = Some(rc.result_message_id.clone());
                            }
                            result
                        });
                        Some(ActivityEvent::ToolCall(Box::new(ToolCallEntry { call, result })))
                    }
                })
                .collect(),
        });
        Turn {
            id: t.id,
            created_at: Some(to_seconds(t.created_at)),
            user: t.user_message.and_then(AgentMessage::from_proto),
            wake_label: t.wake_label,
            run,
            responses: t
                .responses
                .into_iter()
                .filter_map(AgentMessage::from_proto)
                .collect(),
        }
    }
}
