use super::*;

impl TranscriptView {
    /// Return the logical row currently at the top of the viewport and its
    /// offset within that row. This remains stable when Markdown rows change
    /// height, unlike a total pixel offset from the top of the document.
    ///
    /// The row index is clamped to the last message: while a run streams the
    /// list carries a trailing "Working…" row that is not a message, and an
    /// index pointing at it would point past the end once that row is gone.
    pub fn scroll_anchor(&self) -> Option<TranscriptScrollAnchor> {
        if self.messages.is_empty() {
            return None;
        }
        let top = self.list_state.logical_scroll_top();
        let row_index = anchor_row(self.messages.len(), top.item_ix);
        Some(TranscriptScrollAnchor {
            row_index,
            offset_in_row: f32::from(top.offset_in_item),
            at_tail: self.list_state.is_following_tail(),
            message_id: message_id_at(&self.messages, row_index),
        })
    }

    /// The row holding the message with `id`, if it is loaded.
    pub fn row_for_message_id(&self, id: &str) -> Option<usize> {
        row_for_message_id(&self.messages, id)
    }

    /// Restore a logical row anchor after a session's message list is installed.
    /// GPUI can then resolve the anchor again as visible rows are measured.
    ///
    /// The message id names the anchor row, so the position survives a
    /// reloaded page whose rows sit at different indices. A message that is
    /// no longer loaded — it fell outside the fetched window — cannot be
    /// restored to, and the tail is the honest fallback: clamping to a row
    /// index would land on an unrelated message.
    pub fn restore_scroll_anchor(
        &mut self,
        message_id: Option<&str>,
        row_index: usize,
        offset_in_row: f32,
        at_tail: bool,
        cx: &mut Context<Self>,
    ) {
        let (row, at_tail) = match self.resolve_anchor_row(message_id, row_index) {
            Some(row) => (row, at_tail),
            None => (0, true),
        };
        self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
        let generation = self.scroll_restore_generation;
        self.apply_scroll_anchor(row, offset_in_row, at_tail);

        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            cx.background_executor()
                .timer(Duration::from_millis(16))
                .await;
            cx.update(|cx| {
                if let Some(entity) = entity.upgrade() {
                    entity.update(cx, |this, _| {
                        if this.scroll_restore_generation == generation {
                            this.apply_scroll_anchor(row, offset_in_row, at_tail);
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Resolve an anchor to a row of the currently loaded messages.
    ///
    /// A message id that is not loaded cannot be resolved: the fetch window
    /// moved on. The row index is only a fallback for messages that have no
    /// id at all — an optimistic user message, before the server has
    /// assigned one.
    pub(super) fn resolve_anchor_row(&self, message_id: Option<&str>, row_index: usize) -> Option<usize> {
        resolve_anchor_row(&self.messages, message_id, row_index)
    }

    pub(super) fn apply_scroll_anchor(&self, row_index: usize, offset_in_row: f32, at_tail: bool) {
        if at_tail {
            self.list_state.set_follow_mode(FollowMode::Tail);
            self.list_state.scroll_to_end();
            return;
        }
        self.list_state.set_follow_mode(FollowMode::Normal);
        let row_index = row_index.min(self.row_count().saturating_sub(1));
        self.list_state.scroll_to(ListOffset {
            item_ix: row_index,
            offset_in_item: px(offset_in_row),
        });
    }

    /// Smoothly animate scrolling toward the given target (a prompt row or the tail).
    pub fn animate_scroll_to_target(&mut self, target: ScrollTarget, cx: &mut Context<Self>) {
        if cx.reduce_motion() {
            match target {
                ScrollTarget::Bottom => {
                    self.list_state.set_follow_mode(FollowMode::Tail);
                    self.list_state.scroll_to_end();
                }
                ScrollTarget::Prompt(row) => {
                    self.restore_scroll_anchor(None, row, 0.0, false, cx);
                }
            }
            cx.notify();
            return;
        }

        if target == ScrollTarget::Bottom {
            self.animate_scroll_to_bottom(cx);
            return;
        }

        let start_offset = self.list_state.logical_scroll_top();
        let target_item_ix = match target {
            ScrollTarget::Bottom => self.messages.len(),
            ScrollTarget::Prompt(row) => row,
        };

        if start_offset.item_ix == target_item_ix && start_offset.offset_in_item == px(0.0) {
            match target {
                ScrollTarget::Bottom => {
                    self.list_state.set_follow_mode(FollowMode::Tail);
                    self.list_state.scroll_to_end();
                }
                ScrollTarget::Prompt(row) => {
                    self.restore_scroll_anchor(None, row, 0.0, false, cx);
                }
            }
            cx.notify();
            return;
        }

        self.list_state.set_follow_mode(FollowMode::Normal);
        let duration = crate::primitives::DEFAULT_SCROLL_DURATION;
        let start_time = std::time::Instant::now();
        let entity = cx.entity().downgrade();

        self.scroll_animation_task = Some(cx.spawn(async move |_, cx| {
            let duration_secs = duration.as_secs_f32();
            loop {
                cx.background_executor()
                    .timer(crate::primitives::ANIMATION_FRAME_INTERVAL)
                    .await;
                let elapsed = start_time.elapsed().as_secs_f32();
                let progress = (elapsed / duration_secs).min(1.0);
                let current_offset = crate::primitives::interpolate_list_offset(
                    start_offset,
                    target_item_ix,
                    px(0.0),
                    progress,
                );

                let is_done = progress >= 1.0;
                let should_continue = cx.update(|cx| {
                    if let Some(entity) = entity.upgrade() {
                        entity.update(cx, |this, cx| {
                            this.list_state.scroll_to(current_offset);
                            cx.notify();
                            if is_done {
                                this.scroll_animation_task = None;
                                match target {
                                    ScrollTarget::Bottom => {
                                        this.list_state.set_follow_mode(FollowMode::Tail);
                                        this.list_state.scroll_to_end();
                                    }
                                    ScrollTarget::Prompt(row) => {
                                        this.restore_scroll_anchor(None, row, 0.0, false, cx);
                                    }
                                }
                                cx.notify();
                            }
                        });
                        !is_done
                    } else {
                        false
                    }
                });

                if !should_continue {
                    break;
                }
            }
        }));
    }

    /// Glide to the tail in pixel space. Interpolating item indices instead
    /// jumps between rows of different heights, so this drives the list by
    /// pixel deltas and re-reads the (still being measured) end every frame.
    fn animate_scroll_to_bottom(&mut self, cx: &mut Context<Self>) {
        let list = self.list_state.clone();
        list.set_follow_mode(FollowMode::Normal);

        let current = -f32::from(list.scroll_px_offset_for_scrollbar().y);
        let end = f32::from(list.max_offset_for_scrollbar().y);
        if end - current < 1.0 {
            list.set_follow_mode(FollowMode::Tail);
            cx.notify();
            return;
        }

        // Long distances start one screen short of the end so the glide stays
        // readable instead of racing across the whole history.
        let viewport = f32::from(list.viewport_bounds().size.height);
        let max_travel = (viewport * 1.5).max(400.0);
        let start = if end - current > max_travel {
            list.scroll_by(px(end - max_travel - current));
            end - max_travel
        } else {
            current
        };

        let duration_secs = crate::primitives::DEFAULT_SCROLL_DURATION.as_secs_f32();
        let start_time = std::time::Instant::now();
        let entity = cx.entity().downgrade();

        self.scroll_animation_task = Some(cx.spawn(async move |_, cx| {
            let mut position = start;
            loop {
                cx.background_executor()
                    .timer(crate::primitives::ANIMATION_FRAME_INTERVAL)
                    .await;
                let progress = (start_time.elapsed().as_secs_f32() / duration_secs).min(1.0);
                let eased = crate::primitives::ease_out_cubic(progress);
                let is_done = progress >= 1.0;

                let keep_going = cx.update(|cx| {
                    let Some(entity) = entity.upgrade() else {
                        return false;
                    };
                    entity.update(cx, |this, cx| {
                        if is_done {
                            this.scroll_animation_task = None;
                            this.list_state.set_follow_mode(FollowMode::Tail);
                        } else {
                            let end = f32::from(this.list_state.max_offset_for_scrollbar().y);
                            let wanted = start + (end - start) * eased;
                            this.list_state.scroll_by(px(wanted - position));
                            position = wanted;
                        }
                        cx.notify();
                    });
                    !is_done
                });
                if !keep_going {
                    break;
                }
            }
        }));
    }

    /// Jump to the newest message and resume following the run.
    pub(crate) fn scroll_to_bottom(&mut self, cx: &mut Context<Self>) {
        self.animate_scroll_to_target(ScrollTarget::Bottom, cx);
    }
}
