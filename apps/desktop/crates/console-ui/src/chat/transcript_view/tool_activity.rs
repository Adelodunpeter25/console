use super::*;

impl TranscriptView {
    /// Collect the tool calls, thinking, and progress text belonging to the
    /// assistant run after a user message, in chronological content order. The
    /// desktop client uses the same boundary: a new user message starts the
    /// next run, and thinking/text between tool calls stay in the timeline.
    pub(crate) fn tool_activity_for_user(
        &self,
        user_index: usize,
    ) -> Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)> {
        let version = self.tool_activity_cache_version.get();
        if let Some((cached_version, cached)) = self.tool_activity_cache.borrow().get(&user_index)
            && *cached_version == version
        {
            return cached.clone();
        }
        let computed = self.compute_tool_activity_for_user(user_index);
        self.tool_activity_cache
            .borrow_mut()
            .insert(user_index, (version, computed.clone()));
        computed
    }

    pub(super) fn compute_tool_activity_for_user(
        &self,
        user_index: usize,
    ) -> Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)> {
        let mut events: Vec<ActivityEvent> = Vec::new();
        let mut results = HashMap::<String, ToolResult>::new();
        let mut ended_at = None;

        for message in self.messages.iter().skip(user_index + 1) {
            if matches!(message, AgentMessage::User { .. }) {
                break;
            }
            match message {
                AgentMessage::Assistant {
                    content,
                    created_at,
                    ..
                } => {
                    ended_at = ended_at
                        .max(created_at.and_then(crate::utils::time::normalize_unix_timestamp));
                    // Mirror Electron: text/thinking only enters the run timeline if
                    // that same turn contains a tool call. A final answer with no
                    // tool calls is rendered as a standalone bubble, not in activity.
                    let has_tool_call_in_message = content
                        .iter()
                        .any(|part| matches!(part, AssistantContentPart::ToolCall { .. }));
                    if !has_tool_call_in_message {
                        continue;
                    }
                    for part in content {
                        match part {
                            AssistantContentPart::Thinking { text } if !text.trim().is_empty() => {
                                events.push(ActivityEvent::Thinking {
                                    id: format!("run-thinking-{user_index}-{}", events.len()),
                                    text: text.clone(),
                                });
                            }
                            AssistantContentPart::Text { text, .. } if !text.trim().is_empty() => {
                                events.push(ActivityEvent::Text {
                                    id: format!("run-text-{user_index}-{}", events.len()),
                                    text: text.clone(),
                                });
                            }
                            AssistantContentPart::ToolCall { call } => {
                                events.push(ActivityEvent::ToolCall(Box::new(ToolCallEntry {
                                    call: call.clone(),
                                    result: None,
                                })));
                            }
                            _ => {}
                        }
                    }
                }
                AgentMessage::ToolResult {
                    results: message_results,
                    created_at,
                } => {
                    ended_at = ended_at
                        .max(created_at.and_then(crate::utils::time::normalize_unix_timestamp));
                    for result in message_results {
                        results.insert(result.tool_call_id.clone(), result.clone());
                    }
                }
                AgentMessage::User { .. } => unreachable!(),
            }
        }

        let has_tool_calls = events
            .iter()
            .any(|event| matches!(event, ActivityEvent::ToolCall(_)));
        if !has_tool_calls {
            return None;
        }
        let events = events
            .into_iter()
            .map(|event| match event {
                ActivityEvent::ToolCall(mut entry) => {
                    entry.result = results.get(&entry.call.id).cloned();
                    ActivityEvent::ToolCall(entry)
                }
                other => other,
            })
            .collect::<Vec<_>>();

        let user_started_at = match self.messages.get(user_index) {
            Some(AgentMessage::User { created_at, .. }) => {
                created_at.and_then(crate::utils::time::normalize_unix_timestamp)
            }
            _ => None,
        };
        let latest_user = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }));
        let working = self.is_streaming && latest_user == Some(user_index);
        let started_at = if working {
            self.streaming_started_at.or(user_started_at)
        } else {
            user_started_at
        };
        let elapsed = crate::chat::toolcalls::elapsed_seconds(started_at, ended_at);
        Some((events, started_at, elapsed, working))
    }
}
