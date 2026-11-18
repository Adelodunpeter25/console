use super::*;

/// One run block under a user message: the tool calls, thinking, and
/// progress text of a single turn segment. A `RunBoundary` marker splits a
/// user message's activity into multiple blocks — the first is the user's
/// own turn, later ones are server wake turns (finished background work).
#[derive(Clone)]
pub struct RunActivityBlock {
    pub events: Vec<ActivityEvent>,
    pub started_at: Option<i64>,
    pub elapsed: u64,
    pub working: bool,
    /// Divider label shown before this block; `None` for the first segment.
    pub divider_label: Option<String>,
    /// Message index of the wake boundary that opened this block, or `None`
    /// for the initial user segment.
    pub boundary_index: Option<usize>,
    /// Stable id for `ToolCallsState` disclosure (`run-{user}-wake-{n}`).
    pub run_id: String,
}

/// One segment of a user message's activity, collected before the
/// tool-call filter is applied.
struct Segment {
    events: Vec<ActivityEvent>,
    results: HashMap<String, ToolResult>,
    ended_at: Option<i64>,
    /// `(message_index, label, at)` of the `RunBoundary` that opened this
    /// segment, or `None` for the first segment.
    boundary: Option<(usize, String, Option<i64>)>,
}

/// Pure segmentation behind [`TranscriptView::tool_activity_for_user`],
/// split out so tests can exercise it without a gpui `Context`.
pub fn compute_run_blocks(
    messages: &[AgentMessage],
    user_index: usize,
    is_streaming: bool,
    streaming_started_at: Option<i64>,
) -> Option<Vec<RunActivityBlock>> {
    compute_run_blocks_with_boundaries(
        messages,
        user_index,
        is_streaming,
        streaming_started_at,
        &[],
    )
}

/// Like [`compute_run_blocks`], also splitting at reconstructed
/// `wake_boundaries` (used after history reloads where `RunBoundary`
/// markers are not present in `messages`).
pub fn compute_run_blocks_with_boundaries(
    messages: &[AgentMessage],
    user_index: usize,
    is_streaming: bool,
    streaming_started_at: Option<i64>,
    wake_boundaries: &[(usize, String, Option<i64>)],
) -> Option<Vec<RunActivityBlock>> {
    let mut segments: Vec<Segment> = vec![Segment {
        events: Vec::new(),
        results: HashMap::new(),
        ended_at: None,
        boundary: None,
    }];

    for (index, message) in messages.iter().enumerate().skip(user_index + 1) {
        if matches!(message, AgentMessage::User { .. }) {
            break;
        }
        if let AgentMessage::RunBoundary { label, at } = message {
            segments.push(Segment {
                events: Vec::new(),
                results: HashMap::new(),
                ended_at: None,
                boundary: Some((index, label.clone(), *at)),
            });
            continue;
        }
        if let Some((_, label, at)) = wake_boundaries
            .iter()
            .find(|(boundary_index, _, _)| *boundary_index == index)
        {
            segments.push(Segment {
                events: Vec::new(),
                results: HashMap::new(),
                ended_at: None,
                boundary: Some((index, label.clone(), *at)),
            });
        }
        let segment = segments.last_mut().expect("segments never empty");
        match message {
            AgentMessage::Assistant {
                content,
                created_at,
                ..
            } => {
                segment.ended_at = segment
                    .ended_at
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
                            segment.events.push(ActivityEvent::Thinking {
                                id: format!("run-thinking-{user_index}-{}", segment.events.len()),
                                text: text.clone(),
                            });
                        }
                        AssistantContentPart::Text { text, .. } if !text.trim().is_empty() => {
                            segment.events.push(ActivityEvent::Text {
                                id: format!("run-text-{user_index}-{}", segment.events.len()),
                                text: text.clone(),
                            });
                        }
                        AssistantContentPart::ToolCall { call } => {
                            segment
                                .events
                                .push(ActivityEvent::ToolCall(Box::new(ToolCallEntry {
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
                segment.ended_at = segment
                    .ended_at
                    .max(created_at.and_then(crate::utils::time::normalize_unix_timestamp));
                for result in message_results {
                    segment
                        .results
                        .insert(result.tool_call_id.clone(), result.clone());
                }
            }
            AgentMessage::User { .. } | AgentMessage::RunBoundary { .. } => {}
        }
    }

    let user_started_at = match messages.get(user_index) {
        Some(AgentMessage::User { created_at, .. }) => {
            created_at.and_then(crate::utils::time::normalize_unix_timestamp)
        }
        _ => None,
    };
    let latest_user = messages
        .iter()
        .rposition(|message| matches!(message, AgentMessage::User { .. }));
    // A trailing RunBoundary means the wake turn has produced no output yet;
    // nothing inside a block may claim "working" — the transcript's trailing
    // WorkingIndicator row covers that state.
    let last_is_boundary = matches!(messages.last(), Some(AgentMessage::RunBoundary { .. }));
    let any_working = is_streaming && latest_user == Some(user_index) && !last_is_boundary;

    // Only segments containing tool calls emit a block; `working` belongs to
    // the last emitted block.
    let emit: Vec<bool> = segments
        .iter()
        .map(|segment| {
            segment
                .events
                .iter()
                .any(|event| matches!(event, ActivityEvent::ToolCall(_)))
        })
        .collect();
    let segment_count = segments.len();

    let mut blocks = Vec::new();
    for (segment_index, mut segment) in segments.into_iter().enumerate() {
        if !emit[segment_index] {
            continue;
        }
        segment.events = segment
            .events
            .into_iter()
            .map(|event| match event {
                ActivityEvent::ToolCall(mut entry) => {
                    entry.result = segment.results.get(&entry.call.id).cloned();
                    ActivityEvent::ToolCall(entry)
                }
                other => other,
            })
            .collect();

        let (divider_label, boundary_index, boundary_at) = match &segment.boundary {
            Some((idx, label, at)) => (Some(label.clone()), Some(*idx), *at),
            None => (None, None, None),
        };
        let working = any_working && segment_index + 1 == segment_count;
        let started_at = if segment_index == 0 {
            if working {
                streaming_started_at.or(user_started_at)
            } else {
                user_started_at
            }
        } else if working {
            boundary_at.or(streaming_started_at).or(user_started_at)
        } else {
            boundary_at.or(user_started_at)
        };
        let elapsed = crate::chat::toolcalls::elapsed_seconds(started_at, segment.ended_at);
        blocks.push(RunActivityBlock {
            events: segment.events,
            started_at,
            elapsed,
            working,
            divider_label,
            boundary_index,
            run_id: if segment_index == 0 {
                format!("run-{user_index}")
            } else {
                format!("run-{user_index}-wake-{segment_index}")
            },
        });
    }
    if blocks.is_empty() {
        return None;
    }
    Some(blocks)
}

impl TranscriptView {
    /// Collect the run blocks belonging to the assistant activity after a
    /// user message, in chronological order. A new user message starts the
    /// next run; a `RunBoundary` marker (server wake turn) starts a new
    /// block within the same user message. Thinking/text between tool calls
    /// stay in the timeline, and segments without tool calls produce no
    /// block.
    pub fn tool_activity_for_user(&self, user_index: usize) -> Option<Vec<RunActivityBlock>> {
        let version = self.tool_activity_cache_version.get();
        if let Some((cached_version, cached)) = self.tool_activity_cache.borrow().get(&user_index)
            && *cached_version == version
        {
            return cached.clone();
        }
        let computed = compute_run_blocks_with_boundaries(
            &self.messages,
            user_index,
            self.is_streaming,
            self.streaming_started_at,
            &self.wake_boundaries,
        );
        self.tool_activity_cache
            .borrow_mut()
            .insert(user_index, (version, computed.clone()));
        computed
    }
}
