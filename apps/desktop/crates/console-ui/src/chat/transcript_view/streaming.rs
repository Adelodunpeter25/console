use super::*;

impl TranscriptView {
    pub fn begin_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(chrono::Utc::now().timestamp());
        self.refresh_list();
        cx.notify();
    }

    /// Re-engage the trailing "Working for Ns" row after `set_turns` reset
    /// it. Used when switching back to a chat whose run is still active.
    pub fn resume_streaming(&mut self, started_at: i64, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(started_at);
        self.refresh_list();
        cx.notify();
    }

    pub fn finish_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = false;
        self.streaming_started_at = None;
        self.live.clear();
        if let Some(last) = self.turns.last_mut()
            && let Some(run) = last.run.as_mut()
        {
            run.working = false;
        }
        self.refresh_list();
        self.remeasure_active_run_owner();
        cx.notify();
    }

    fn ensure_tail_run(&mut self) -> &mut TurnRun {
        if self.turns.is_empty() {
            self.turns.push(Turn {
                id: String::new(),
                created_at: Some(chrono::Utc::now().timestamp()),
                user: None,
                wake_label: None,
                run: None,
                responses: Vec::new(),
            });
            self.content_revisions.push(0);
        }
        let started_at = self.streaming_started_at;
        let counter = &mut self.live_counter;
        let turn = self.turns.last_mut().expect("turns non-empty");
        turn.run.get_or_insert_with(|| {
            let run_id = if turn.id.is_empty() {
                *counter += 1;
                format!("live-{}", *counter)
            } else {
                turn.id.clone()
            };
            TurnRun {
                run_id,
                started_at: started_at.or(turn.created_at),
                elapsed_ms: 0,
                working: true,
                events: Vec::new(),
            }
        })
    }

    pub fn append_assistant_text(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(existing) = self.live.iter_mut().find_map(|part| match part {
            AssistantContentPart::Text { text, .. } => Some(text),
            _ => None,
        }) {
            existing.push_str(text);
        } else {
            self.live.push(AssistantContentPart::Text {
                text: text.to_string(),
                thought_signature: None,
            });
        }
        self.bump_last_content_revision();
    }

    pub fn append_assistant_thinking(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(existing) = self.live.iter_mut().find_map(|part| match part {
            AssistantContentPart::Thinking { text } => Some(text),
            _ => None,
        }) {
            existing.push_str(text);
        } else {
            let text_pos = self
                .live
                .iter()
                .position(|p| matches!(p, AssistantContentPart::Text { .. }));
            let part = AssistantContentPart::Thinking {
                text: text.to_string(),
            };
            if let Some(pos) = text_pos {
                self.live.insert(pos, part);
            } else {
                self.live.push(part);
            }
        }
        self.bump_last_content_revision();
    }

    /// Flush the latest coalesced stream state into the list and publish one
    /// transcript repaint.
    pub fn flush_stream_render(&mut self, cx: &mut Context<Self>) {
        self.remeasure_active_run_owner();
        self.refresh_list();
        cx.notify();
    }

    pub fn finalize_assistant_message(
        &mut self,
        message: AssistantMessage,
        cx: &mut Context<Self>,
    ) {
        self.live.clear();
        let has_tool_call = message
            .content
            .iter()
            .any(|part| matches!(part, AssistantContentPart::ToolCall { .. }));
        if has_tool_call {
            let mut pending_results = std::mem::take(&mut self.pending_results);
            let run = self.ensure_tail_run();
            for part in message.content {
                match part {
                    AssistantContentPart::Thinking { text } if !text.trim().is_empty() => {
                        let id = format!("{}-thinking-{}", run.run_id, run.events.len());
                        run.events.push(ActivityEvent::Thinking { id, text });
                    }
                    AssistantContentPart::Text { text, .. } if !text.trim().is_empty() => {
                        let id = format!("{}-text-{}", run.run_id, run.events.len());
                        run.events.push(ActivityEvent::Text { id, text });
                    }
                    AssistantContentPart::ToolCall { call } => {
                        if let Some(ActivityEvent::ToolCall(entry)) =
                            run.events.iter_mut().find(|event| {
                                matches!(event, ActivityEvent::ToolCall(e) if e.call.id == call.id)
                            })
                        {
                            entry.call = call;
                        } else {
                            let result = pending_results.remove(&call.id);
                            run.events
                                .push(ActivityEvent::ToolCall(Box::new(ToolCallEntry {
                                    call,
                                    result,
                                })));
                        }
                    }
                    _ => {}
                }
            }
            self.pending_results = pending_results;
        } else {
            if self.turns.is_empty() {
                self.turns.push(Turn {
                    id: String::new(),
                    created_at: Some(chrono::Utc::now().timestamp()),
                    user: None,
                    wake_label: None,
                    run: None,
                    responses: Vec::new(),
                });
                self.content_revisions.push(0);
            }
            let turn = self.turns.last_mut().expect("turns non-empty");
            let msg = AgentMessage::Assistant {
                id: message.id,
                content: message.content,
                stop_reason: message.stop_reason,
                created_at: message
                    .created_at
                    .or_else(|| Some(chrono::Utc::now().timestamp())),
                tok_per_sec: message.tok_per_sec,
            };
            turn.responses.push(msg);
        }
        self.bump_last_content_revision();
        self.refresh_list();
        self.remeasure_active_run_owner();
        cx.notify();
    }

    pub fn upsert_assistant_tool_call(
        &mut self,
        call: console_core::ToolCall,
        cx: &mut Context<Self>,
    ) {
        if let Some(AssistantContentPart::ToolCall { call: existing }) =
            self.live.iter_mut().find(|part| {
                matches!(part, AssistantContentPart::ToolCall { call: existing } if existing.id == call.id)
            })
        {
            *existing = call;
        } else {
            self.live.push(AssistantContentPart::ToolCall { call });
        }
        self.bump_last_content_revision();
        self.refresh_list();
        self.remeasure_active_run_owner();
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
        let mut pending_results = std::mem::take(&mut self.pending_results);
        let run = self.ensure_tail_run();
        for call in calls {
            if let Some(ActivityEvent::ToolCall(entry)) = run
                .events
                .iter_mut()
                .find(|event| matches!(event, ActivityEvent::ToolCall(e) if e.call.id == call.id))
            {
                entry.call = call;
            } else {
                let result = pending_results.remove(&call.id);
                run.events
                    .push(ActivityEvent::ToolCall(Box::new(ToolCallEntry {
                        call,
                        result,
                    })));
            }
        }
        self.pending_results = pending_results;
        self.bump_last_content_revision();
        self.refresh_list();
        self.remeasure_active_run_owner();
        cx.notify();
    }

    pub fn append_tool_results(&mut self, results: Vec<ToolResult>, cx: &mut Context<Self>) {
        if results.is_empty() {
            return;
        }
        let mut changed = false;
        for result in results {
            let mut attached = false;
            if let Some(turn) = self.turns.last_mut()
                && let Some(run) = turn.run.as_mut()
                && let Some(ActivityEvent::ToolCall(entry)) = run.events.iter_mut().find(|event| {
                    matches!(event, ActivityEvent::ToolCall(e) if e.call.id == result.tool_call_id)
                })
            {
                if entry.result.is_none() {
                    entry.result = Some(result.clone());
                    changed = true;
                }
                attached = true;
            }
            if !attached {
                self.pending_results
                    .entry(result.tool_call_id.clone())
                    .or_insert(result);
            }
        }
        if changed {
            self.bump_last_content_revision();
            self.refresh_list();
            self.remeasure_active_run_owner();
            cx.notify();
        }
    }
}
