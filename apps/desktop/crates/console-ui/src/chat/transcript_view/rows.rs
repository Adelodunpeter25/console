use super::*;

impl TranscriptView {
    /// Re-anchor the list after the message set changed: keep following the
    /// tail when the user is already following it (streaming, new prompts),
    /// otherwise leave the scroll position alone.
    pub(super) fn refresh_list(&self) {
        let row_count = self.row_count();
        if self.list_state.item_count() != row_count {
            if self.list_state.is_following_tail() {
                self.list_state.reset(row_count);
                self.list_state.scroll_to_end();
            } else {
                let top = self.list_state.logical_scroll_top();
                let item_ix = top.item_ix.min(row_count.saturating_sub(1));
                self.list_state.reset(row_count);
                self.list_state.scroll_to(ListOffset {
                    item_ix,
                    offset_in_item: top.offset_in_item,
                });
            }
        } else if self.list_state.is_following_tail() {
            self.list_state.scroll_to_end();
        }
    }

    /// Invalidate only the row whose content changed. GPUI can retain the
    /// measured heights and virtualization state for every other message.
    pub(super) fn remeasure_message(&self, message_index: usize) {
        if message_index < self.list_state.item_count() {
            self.list_state
                .remeasure_items(message_index..message_index + 1);
        }
    }

    pub(super) fn invalidate_activity_cache(&self) {
        self.tool_activity_cache_version
            .set(self.tool_activity_cache_version.get().wrapping_add(1));
        self.tool_activity_cache.borrow_mut().clear();
    }

    /// Drop only the newest user's cached run timeline. Stream deltas,
    /// tool-call upserts/results, and streaming-flag changes can only alter
    /// the run after the last user message, so earlier users' cached
    /// timelines stay valid. Deliberately does NOT bump the cache version:
    /// surviving entries must keep matching it to remain hits.
    pub(super) fn invalidate_latest_activity_cache(&self) {
        let latest_user = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }));
        if let Some(user_index) = latest_user {
            self.tool_activity_cache.borrow_mut().remove(&user_index);
        }
    }

    /// List rows: the messages plus, while a run is streaming, the trailing
    /// "Working…" indicator — a row like in Waku, so it sits directly below
    /// whatever has streamed in instead of floating at the bottom edge.
    pub(super) fn row_count(&self) -> usize {
        // Cheap check: does the last user run contain any tool call?
        // Avoid building full ActivityEvent vector just to test existence.
        let has_active_tool_activity = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }))
            .is_some_and(|user_index| {
                self.messages
                    .iter()
                    .skip(user_index + 1)
                    .any(|message| match message {
                        AgentMessage::Assistant { content, .. } => content
                            .iter()
                            .any(|part| matches!(part, AssistantContentPart::ToolCall { .. })),
                        AgentMessage::User { .. } => false,
                        AgentMessage::ToolResult { .. } => false,
                    })
            });
        self.messages.len() + usize::from(self.is_streaming && !has_active_tool_activity)
    }

    pub(crate) fn markdown_view_for(
        &self,
        message_index: usize,
        part_index: usize,
    ) -> Rc<RefCell<MarkdownView>> {
        self.markdown_cache
            .borrow_mut()
            .entry((message_index, part_index))
            .or_insert_with(|| Rc::new(RefCell::new(MarkdownView::new())))
            .clone()
    }

    pub(crate) fn assistant_presentation(
        &self,
        message_index: usize,
        content: &[AssistantContentPart],
    ) -> AssistantPresentation {
        let revision = self
            .content_revisions
            .get(message_index)
            .copied()
            .unwrap_or_default();
        let mut cache = self.presentation_cache.borrow_mut();
        if let Some((cached_revision, presentation)) = cache.get(&message_index)
            && *cached_revision == revision
        {
            return presentation.clone();
        }

        let content_parts = Rc::new(content.to_vec());
        let copy_content = content_parts
            .iter()
            .filter_map(|part| match part {
                AssistantContentPart::Text { text, .. }
                | AssistantContentPart::Thinking { text } => Some(text.as_str()),
                AssistantContentPart::ToolCall { .. } | AssistantContentPart::Image { .. } => None,
            })
            .collect::<Vec<_>>()
            .join("\n\n");
        let presentation = AssistantPresentation {
            content_parts,
            copy_content: SharedString::from(copy_content),
        };
        cache.insert(message_index, (revision, presentation.clone()));
        presentation
    }
}
