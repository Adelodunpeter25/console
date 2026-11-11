use super::*;

impl TranscriptView {
    pub fn begin_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(chrono::Utc::now().timestamp());
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    /// Re-engage the trailing "Working for Ns" row after `set_messages` reset
    /// it. Used when switching back to a chat whose run is still active: the
    /// started_at is the run's real start time (from `running_sessions`) so the
    /// elapsed counter continues seamlessly instead of restarting from zero.
    pub fn resume_streaming(&mut self, started_at: i64, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(started_at);
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    pub fn finish_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = false;
        self.streaming_started_at = None;
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    pub fn append_assistant_text(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(existing) = content.iter_mut().find_map(|part| match part {
                AssistantContentPart::Text { text, .. } => Some(text),
                _ => None,
            }) {
                existing.push_str(text);
            } else {
                content.push(AssistantContentPart::Text {
                    text: text.to_string(),
                    thought_signature: None,
                });
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::Text {
                    text: text.to_string(),
                    thought_signature: None,
                }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
                tok_per_sec: None,
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
    }

    pub fn append_assistant_thinking(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(existing) = content.iter_mut().find_map(|part| match part {
                AssistantContentPart::Thinking { text } => Some(text),
                _ => None,
            }) {
                existing.push_str(text);
            } else {
                let text_pos = content
                    .iter()
                    .position(|p| matches!(p, AssistantContentPart::Text { .. }));
                let part = AssistantContentPart::Thinking {
                    text: text.to_string(),
                };
                if let Some(pos) = text_pos {
                    content.insert(pos, part);
                } else {
                    content.push(part);
                }
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::Thinking {
                    text: text.to_string(),
                }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
                tok_per_sec: None,
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
    }

    /// Flush the latest coalesced stream state into the list and publish one
    /// transcript repaint. Text deltas update the message buffer immediately,
    /// but this is called only once per stream cadence window.
    pub fn flush_stream_render(&mut self, cx: &mut Context<Self>) {
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        self.refresh_list();
        cx.notify();
    }

    pub fn finalize_assistant_message(
        &mut self,
        message: AssistantMessage,
        cx: &mut Context<Self>,
    ) {
        if let Some(AgentMessage::Assistant {
            id,
            content,
            stop_reason,
            created_at,
            tok_per_sec,
        }) = self.messages.last_mut()
        {
            if message.id.is_some() {
                *id = message.id;
            }
            *content = message.content;
            *stop_reason = message.stop_reason;
            if message.created_at.is_some() {
                *created_at = message.created_at;
            }
            *tok_per_sec = message.tok_per_sec;
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: message.id,
                content: message.content,
                stop_reason: message.stop_reason,
                created_at: message.created_at,
                tok_per_sec: message.tok_per_sec,
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn upsert_assistant_tool_call(
        &mut self,
        call: console_core::ToolCall,
        cx: &mut Context<Self>,
    ) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(AssistantContentPart::ToolCall { call: existing }) = content.iter_mut().find(|part| {
                matches!(part, AssistantContentPart::ToolCall { call: existing } if existing.id == call.id)
            }) {
                *existing = call;
            } else {
                content.push(AssistantContentPart::ToolCall { call });
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::ToolCall { call }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
                tok_per_sec: None,
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn upsert_assistant_tool_calls(
        &mut self,
        calls: Vec<console_core::ToolCall>,
        cx: &mut Context<Self>,
    ) {
        if calls.is_empty() {
            return;
        }
        for call in calls {
            if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
                if let Some(AssistantContentPart::ToolCall { call: existing }) =
                    content.iter_mut().find(|part| {
                        matches!(part, AssistantContentPart::ToolCall { call: existing } if existing.id == call.id)
                    })
                {
                    *existing = call;
                } else {
                    content.push(AssistantContentPart::ToolCall { call });
                }
            } else {
                self.messages.push(AgentMessage::Assistant {
                    id: None,
                    content: vec![AssistantContentPart::ToolCall { call }],
                    stop_reason: None,
                    created_at: Some(chrono::Utc::now().timestamp()),
                    tok_per_sec: None,
                });
                self.content_revisions.push(0);
            }
        }
        // Coalesce to one revision bump / cache invalidation / remeasure
        self.bump_last_content_revision();
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn append_tool_results(&mut self, results: Vec<ToolResult>, cx: &mut Context<Self>) {
        let existing_ids = self
            .messages
            .iter()
            .filter_map(|message| match message {
                AgentMessage::ToolResult { results, .. } => Some(results),
                _ => None,
            })
            .flatten()
            .map(|result| result.tool_call_id.as_str())
            .collect::<std::collections::HashSet<_>>();
        let results = results
            .into_iter()
            .filter(|result| !existing_ids.contains(result.tool_call_id.as_str()))
            .collect::<Vec<_>>();
        if results.is_empty() {
            return;
        }
        self.messages.push(AgentMessage::ToolResult {
            results,
            created_at: Some(chrono::Utc::now().timestamp()),
        });
        self.content_revisions.push(0);
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }
}
