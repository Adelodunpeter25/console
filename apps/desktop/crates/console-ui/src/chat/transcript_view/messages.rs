use super::*;

impl TranscriptView {
    pub fn set_pagination(&mut self, has_more: bool, next_cursor: Option<i64>) {
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;
    }

    pub fn prepend_turns(
        &mut self,
        older: Vec<Turn>,
        has_more: bool,
        next_cursor: Option<i64>,
        cx: &mut Context<Self>,
    ) {
        self.just_sent = None;
        let older: Vec<Turn> = older
            .into_iter()
            .filter(|old_turn| {
                old_turn.id.is_empty()
                    || !self.turns.iter().any(|existing| existing.id == old_turn.id)
            })
            .collect();

        if older.is_empty() {
            self.has_more = has_more;
            self.next_cursor = next_cursor;
            self.loading_older = false;
            cx.notify();
            return;
        }
        let anchor = self.scroll_anchor();
        let old_len = self.turns.len();
        let mut new_turns = older;
        new_turns.append(&mut self.turns);
        self.turns = new_turns;
        self.content_revisions = vec![0; self.turns.len()];
        self.presentation_cache.borrow_mut().clear();
        self.markdown_cache.borrow_mut().clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.expanded_user_messages.borrow_mut().clear();
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;

        let prepended = self.turns.len() - old_len;
        if let Some(anchor) = anchor {
            if anchor.at_tail {
                self.list_state.set_follow_mode(FollowMode::Tail);
                self.refresh_list();
                self.list_state.scroll_to_end();
            } else {
                let new_row = anchor.row_index + prepended;
                self.list_state.set_follow_mode(FollowMode::Normal);
                self.refresh_list();
                self.list_state.scroll_to(ListOffset {
                    item_ix: new_row,
                    offset_in_item: px(anchor.offset_in_row),
                });
            }
        } else {
            self.refresh_list();
        }
        cx.notify();
    }

    pub fn set_loading_older(&mut self, loading: bool) {
        self.loading_older = loading;
    }

    pub fn turn_count(&self) -> usize {
        self.turns.len()
    }

    pub fn has_trailing_user_message(&self, content: &str, source: Option<&str>) -> bool {
        matches!(
            self.turns.last().and_then(|turn| turn.user.as_ref()),
            Some(AgentMessage::User {
                content: last_content,
                source: last_source,
                ..
            }) if last_content == content && last_source.as_deref() == source
        )
    }

    pub fn attachments_for_message(&self, message_id: &str) -> Option<Vec<ImageAttachment>> {
        self.turns
            .iter()
            .filter_map(|turn| turn.user.as_ref())
            .find_map(|message| match message {
                AgentMessage::User {
                    id: Some(id),
                    attachments,
                    ..
                } if id == message_id => Some(attachments.clone().unwrap_or_default()),
                _ => None,
            })
    }

    pub fn annotations_for_message(
        &self,
        message_id: &str,
    ) -> Option<Vec<console_core::BrowserElementAnnotation>> {
        self.turns
            .iter()
            .filter_map(|turn| turn.user.as_ref())
            .find_map(|message| match message {
                AgentMessage::User {
                    id: Some(id),
                    annotations,
                    ..
                } if id == message_id => Some(annotations.clone().unwrap_or_default()),
                _ => None,
            })
    }

    pub(super) fn bump_last_content_revision(&mut self) {
        if let Some(revision) = self.content_revisions.last_mut() {
            *revision = revision.wrapping_add(1);
        }
    }

    pub fn set_turns(&mut self, turns: Vec<Turn>, cx: &mut Context<Self>) {
        self.just_sent = None;
        self.prompts = Rc::new(Vec::new());
        self.prompts_stale = true;
        self.live.clear();
        self.pending_results.clear();
        self.markdown_cache.borrow_mut().clear();
        self.presentation_cache.borrow_mut().clear();
        let anchor = self.scroll_anchor();
        self.turns = turns;
        self.content_revisions = vec![0; self.turns.len()];
        self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
        self.selection.clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.expanded_user_messages.borrow_mut().clear();
        self.is_streaming = false;
        self.streaming_started_at = None;
        let was_at_tail = anchor.as_ref().map(|anchor| anchor.at_tail).unwrap_or(true);
        if was_at_tail {
            self.list_state.set_follow_mode(FollowMode::Tail);
            self.list_state.reset(self.row_count());
            self.list_state.scroll_to_end();
        } else if let Some(anchor) = anchor {
            self.list_state.set_follow_mode(FollowMode::Normal);
            let row_index = anchor.row_index.min(self.row_count().saturating_sub(1));
            self.list_state.reset(self.row_count());
            self.list_state.scroll_to(ListOffset {
                item_ix: row_index,
                offset_in_item: px(anchor.offset_in_row),
            });
        } else {
            self.list_state.set_follow_mode(FollowMode::Tail);
            self.list_state.reset(self.row_count());
            self.list_state.scroll_to_end();
        }
        cx.notify();
    }

    pub fn push_message(&mut self, msg: AgentMessage, cx: &mut Context<Self>) {
        let created_at = match &msg {
            AgentMessage::User { created_at, .. } => *created_at,
            _ => None,
        };
        let is_user = matches!(msg, AgentMessage::User { .. });
        if is_user {
            self.prompts_stale = true;
        }
        self.live.clear();
        self.pending_results.clear();
        self.turns.push(Turn {
            id: String::new(),
            created_at,
            user: is_user.then_some(msg),
            wake_label: None,
            run: None,
            responses: Vec::new(),
        });
        self.content_revisions.push(0);
        if is_user {
            self.just_sent = Some((self.turns.len() - 1, Instant::now()));
        }
        self.list_state.set_follow_mode(FollowMode::Tail);
        self.refresh_list();
        cx.notify();
    }
}
