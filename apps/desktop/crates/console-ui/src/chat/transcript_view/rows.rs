use super::*;

impl TranscriptView {
    /// Re-anchor the list after the turn set changed: keep following the
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
    /// measured heights and virtualization state for every other turn.
    pub(super) fn remeasure_turn(&self, turn_index: usize) {
        if turn_index < self.list_state.item_count() {
            self.list_state
                .remeasure_items(turn_index..turn_index + 1);
        }
    }

    pub(super) fn remeasure_active_run_owner(&self) {
        if let Some(last) = self.turns.len().checked_sub(1) {
            self.remeasure_turn(last);
        }
    }

    /// List rows: one row per turn plus, while a run is streaming without
    /// tool activity in the tail turn, the trailing "Working…" indicator.
    pub(super) fn row_count(&self) -> usize {
        let has_active_tool_activity = self
            .turns
            .last()
            .and_then(|turn| turn.run.as_ref())
            .is_some_and(|run| {
                run.events
                    .iter()
                    .any(|event| matches!(event, ActivityEvent::ToolCall(_)))
            });
        self.turns.len() + usize::from(self.is_streaming && !has_active_tool_activity)
    }

    pub(crate) fn markdown_view_for(
        &self,
        turn_index: usize,
        slot: usize,
    ) -> Rc<RefCell<MarkdownView>> {
        self.markdown_cache
            .borrow_mut()
            .entry((turn_index, slot))
            .or_insert_with(|| Rc::new(RefCell::new(MarkdownView::new())))
            .clone()
    }

    pub(crate) fn assistant_presentation(
        &self,
        turn_index: usize,
        response_index: usize,
        content: &[AssistantContentPart],
    ) -> AssistantPresentation {
        let key = turn_index * 1024 + response_index;
        let revision = self
            .content_revisions
            .get(turn_index)
            .copied()
            .unwrap_or_default();
        let mut cache = self.presentation_cache.borrow_mut();
        if let Some((cached_revision, presentation)) = cache.get(&key)
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
        cache.insert(key, (revision, presentation.clone()));
        presentation
    }
}
