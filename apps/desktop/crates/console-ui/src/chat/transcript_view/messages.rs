use super::*;

impl TranscriptView {
    pub fn set_pagination(&mut self, has_more: bool, next_cursor: Option<i64>) {
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;
    }

    pub fn prepend_messages(
        &mut self,
        older: Vec<AgentMessage>,
        has_more: bool,
        next_cursor: Option<i64>,
        cx: &mut Context<Self>,
    ) {
        // Filter out any older messages that are already present in self.messages
        let older: Vec<AgentMessage> = older
            .into_iter()
            .filter(|old_msg| {
                !self
                    .messages
                    .iter()
                    .any(|existing| messages_match(old_msg, existing))
            })
            .collect();

        if older.is_empty() {
            self.has_more = has_more;
            self.next_cursor = next_cursor;
            self.loading_older = false;
            cx.notify();
            return;
        }
        // Preserve scroll anchor at current top item before prepending
        let anchor = self.scroll_anchor();
        let old_len = self.messages.len();
        // Prepend older messages
        let mut new_messages = older;
        new_messages.append(&mut self.messages);
        self.messages = new_messages;
        // Rebuild revisions for new messages (simple: reset all)
        self.content_revisions = vec![0; self.messages.len()];
        self.presentation_cache.borrow_mut().clear();
        self.markdown_cache.borrow_mut().clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.expanded_user_messages.borrow_mut().clear();
        self.invalidate_activity_cache();
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;

        // Keep scroll position visually stable by preserving current offset immediately
        let prepended = self.messages.len() - old_len;
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

    pub fn message_count(&self) -> usize {
        self.messages.len()
    }

    /// The image attachments carried by the user message with this id.
    ///
    /// Composer history recall holds a message id rather than the image bytes
    /// itself, so this is where a recalled prompt's staged attachments are
    /// read back from. `None` when no loaded user message carries that id —
    /// the message may not be in the loaded window, or may have had no images.
    pub fn attachments_for_message(&self, message_id: &str) -> Option<Vec<ImageAttachment>> {
        self.messages.iter().find_map(|message| match message {
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
        self.messages.iter().find_map(|message| match message {
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

    pub fn set_messages(&mut self, messages: Vec<AgentMessage>, cx: &mut Context<Self>) {
        // A different chat's prompts must not flash when the popup next opens.
        self.prompts = Rc::new(Vec::new());
        self.prompts_stale = true;
        // Keep cached views whose message/part identity still exists. When a
        // session is replaced, the next `set_text` compares the new source and
        // safely resets any retained parser state for a reused index.
        let valid_keys = messages
            .iter()
            .enumerate()
            .filter_map(|(message_index, message)| match message {
                AgentMessage::Assistant { content, .. } => Some(
                    content
                        .iter()
                        .enumerate()
                        .filter_map(move |(part_index, part)| {
                            matches!(
                                part,
                                AssistantContentPart::Text { .. }
                                    | AssistantContentPart::Thinking { .. }
                            )
                            .then_some((message_index, part_index))
                        }),
                ),
                _ => None,
            })
            .flatten()
            .collect::<std::collections::HashSet<_>>();
        self.markdown_cache
            .borrow_mut()
            .retain(|key, _| valid_keys.contains(key));
        self.presentation_cache.borrow_mut().clear();
        let anchor = self.scroll_anchor();
        self.messages = messages;
        self.content_revisions = vec![0; self.messages.len()];
        self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
        self.selection.clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.expanded_user_messages.borrow_mut().clear();
        self.invalidate_activity_cache();
        self.is_streaming = false;
        self.streaming_started_at = None;
        // Keep following tail if the user was already at the tail or this is a
        // freshly loaded session; otherwise restore their previous scroll anchor
        // so settling never teleports an inspecting user back down or up.
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

    /// Insert a page of older messages at the front of the transcript.
    ///
    /// The viewport is re-anchored to whatever row was at the top before the
    /// prepend plus the number of new messages, so the user does not see the
    /// scroll position jump. Empty input is a no-op.
    pub fn prepend_older_messages(&mut self, older: Vec<AgentMessage>, cx: &mut Context<Self>) {
        if older.is_empty() {
            return;
        }
        // Snapshot the anchor BEFORE the prepend so we can restore the same
        // visible row afterwards.
        let anchor = self.scroll_anchor();
        let prepend_count = older.len();

        // Invalidate markdown caches for the prepended rows so they get fresh
        // renders (their indices shift by `prepend_count`).
        self.markdown_cache.borrow_mut().clear();
        self.presentation_cache.borrow_mut().clear();

        // Bump content revisions for the existing rows so virtualization
        // knows their layout is now offset by `prepend_count` new rows above.
        let new_len = self.messages.len() + prepend_count;
        self.content_revisions = vec![0; new_len];

        // Splice the older messages in front.
        let mut combined = older;
        combined.append(&mut self.messages);
        self.messages = combined;

        // The activity cache is keyed by user index and would now point at
        // the wrong messages — invalidate it.
        self.invalidate_activity_cache();

        // Reset the list with the new row count, then restore the anchor.
        self.list_state.reset(self.row_count());
        let _ = anchor.map(|anchor| {
            if anchor.at_tail {
                self.list_state.set_follow_mode(FollowMode::Tail);
                self.list_state.scroll_to_end();
            } else {
                self.list_state.set_follow_mode(FollowMode::Normal);
                let new_row_index = anchor.row_index.saturating_add(prepend_count);
                self.list_state.scroll_to(ListOffset {
                    item_ix: new_row_index,
                    offset_in_item: px(anchor.offset_in_row),
                });
                // Re-apply on the next frame so variable-height Markdown
                // rows are measured before the offset resolves.
                self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
                let generation = self.scroll_restore_generation;
                let entity = cx.entity().downgrade();
                let row = new_row_index;
                let off = anchor.offset_in_row;
                cx.spawn(async move |_, cx| {
                    cx.background_executor()
                        .timer(Duration::from_millis(16))
                        .await;
                    cx.update(|cx| {
                        if let Some(entity) = entity.upgrade() {
                            entity.update(cx, |this, _| {
                                if this.scroll_restore_generation == generation {
                                    this.list_state.set_follow_mode(FollowMode::Normal);
                                    this.list_state.scroll_to(ListOffset {
                                        item_ix: row,
                                        offset_in_item: px(off),
                                    });
                                }
                            });
                        }
                    });
                })
                .detach();
            }
        });
        self.loading_older = false;
        cx.notify();
    }

    pub fn push_message(&mut self, msg: AgentMessage, cx: &mut Context<Self>) {
        if matches!(msg, AgentMessage::User { .. }) {
            self.prompts_stale = true;
        }
        // The previous latest user's cached run may still say "working"; once
        // a newer prompt exists (e.g. a steer) that run is no longer live.
        self.invalidate_latest_activity_cache();
        self.messages.push(msg);
        self.content_revisions.push(0);
        // A new prompt means the user wants to see its response: re-engage
        // the tail so streaming stays in view.
        self.list_state.set_follow_mode(FollowMode::Tail);
        self.refresh_list();
        cx.notify();
    }
}

fn messages_match(a: &AgentMessage, b: &AgentMessage) -> bool {
    match (a, b) {
        (
            AgentMessage::User {
                content: c1,
                created_at: t1,
                ..
            },
            AgentMessage::User {
                content: c2,
                created_at: t2,
                ..
            },
        ) => c1 == c2 && (t1.is_none() || t2.is_none() || t1 == t2),
        (
            AgentMessage::Assistant { id: Some(id1), .. },
            AgentMessage::Assistant { id: Some(id2), .. },
        ) => id1 == id2,
        (
            AgentMessage::ToolResult { results: r1, .. },
            AgentMessage::ToolResult { results: r2, .. },
        ) => !r1.is_empty() && !r2.is_empty() && r1[0].tool_call_id == r2[0].tool_call_id,
        _ => false,
    }
}
