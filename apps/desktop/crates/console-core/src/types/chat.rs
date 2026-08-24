//! Run-activity timeline types shared by the live stream handler and the
//! transcript renderer. Mirrors the desktop app's `types/chat.ts` so thinking
//! and text between tool calls stay in chronological order in one canonical
//! shape.

use super::agent::{ToolCall, ToolResult};

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
    ToolCall(ToolCallEntry),
}
