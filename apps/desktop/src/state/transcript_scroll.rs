//! Per-session transcript scroll persistence.
//!
//! The transcript itself owns GPUI's virtualized list state, while this module
//! owns the app-level mapping from session ids to logical scroll anchors. A row
//! anchor survives variable-height Markdown remeasurement better than a raw
//! pixel offset, and a message id survives a reload whose rows have moved.

use gpui::Context;

use crate::persistence::store::{PersistedTranscriptScroll, PersistedTranscriptScrollState};

use super::ConsoleDesktopApp;

/// Delay before a snapshot is written to disk. Scroll positions change in
/// bursts — a fling, a page of older messages — and the state file is
/// rewritten whole, so the writes are coalesced.
const TRANSCRIPT_SCROLL_SAVE_DEBOUNCE: std::time::Duration = std::time::Duration::from_millis(500);

/// How many chats keep a remembered scroll position.
///
/// Pruning to the chats that still exist normally keeps the map well under
/// this; the cap is the backstop for a server with a very long history.
const MAX_TRANSCRIPT_SCROLL_POSITIONS: usize = 200;

/// Where a chat's transcript was left.
///
/// The message id names the anchor row, so the position survives a reload
/// whose rows sit at different indices. `row_index` is the fallback for
/// messages that have no id at all — an optimistic user message, before the
/// server has assigned one.
#[derive(Clone, Debug)]
pub(crate) struct TranscriptScrollPosition {
    pub message_id: Option<String>,
    pub row_index: usize,
    pub offset_in_row: f32,
    pub at_tail: bool,
}

impl ConsoleDesktopApp {
    /// Snapshot the chat a pane is currently showing.
    pub(crate) fn save_pane_scroll_position(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(session_id) = self.active_session_for_pane(pane_id) else {
            return;
        };
        self.save_transcript_scroll_position_for_pane(pane_id, &session_id, cx);
    }

    /// Record where a pane's transcript is scrolled to.
    pub(crate) fn save_transcript_scroll_position_for_pane(
        &mut self,
        pane_id: &str,
        session_id: &str,
        cx: &mut Context<Self>,
    ) {
        // A pane that does not exist has no transcript of its own. The
        // render-side lookup falls back to the main transcript, which would
        // record the main pane's position under this pane's session.
        let Some(transcript) = self.transcript_for_pane_checked(pane_id) else {
            return;
        };
        let Some(anchor) = transcript.read(cx).scroll_anchor() else {
            return;
        };
        self.transcript_scroll_positions.insert(
            session_id.to_string(),
            TranscriptScrollPosition {
                message_id: anchor.message_id,
                row_index: anchor.row_index,
                offset_in_row: anchor.offset_in_row,
                at_tail: anchor.at_tail,
            },
        );
        self.schedule_transcript_scroll_persist(cx);
    }

    /// Snapshot every pane's visible chat.
    ///
    /// The remembered positions are only as fresh as the last snapshot, so a
    /// chat that is still on screen has moved since it was recorded. This is
    /// what makes the positions written on quit the ones the user is looking
    /// at.
    pub(crate) fn snapshot_visible_pane_scrolls(&mut self, cx: &mut Context<Self>) {
        let pane_ids: Vec<String> = self
            .workspace_root
            .leaves()
            .into_iter()
            .map(|leaf| leaf.id.clone())
            .collect();
        for pane_id in pane_ids {
            self.save_pane_scroll_position(&pane_id, cx);
        }
    }

    /// Forget a chat's remembered position.
    pub(crate) fn forget_transcript_scroll_position(
        &mut self,
        session_id: &str,
        cx: &mut Context<Self>,
    ) {
        if self
            .transcript_scroll_positions
            .remove(session_id)
            .is_some()
        {
            self.schedule_transcript_scroll_persist(cx);
        }
    }

    /// Drop the positions of chats that no longer exist, and keep the map
    /// within its cap.
    ///
    /// The cap evicts the least recently updated chats first: those are the
    /// ones a returning user is least likely to want back, and their
    /// positions are the cheapest to lose.
    pub(crate) fn prune_transcript_scroll_positions(&mut self, cx: &mut Context<Self>) {
        let live: std::collections::HashSet<String> = self
            .sessions
            .iter()
            .map(|session| session.id.clone())
            .collect();
        let before = self.transcript_scroll_positions.len();
        self.transcript_scroll_positions
            .retain(|session_id, _| live.contains(session_id));
        if self.transcript_scroll_positions.len() > MAX_TRANSCRIPT_SCROLL_POSITIONS {
            let mut by_recency: Vec<(i64, String)> = self
                .transcript_scroll_positions
                .keys()
                .map(|session_id| {
                    let recency = self
                        .sessions
                        .iter()
                        .find(|session| &session.id == session_id)
                        .map(|session| session.updated_at)
                        .unwrap_or(i64::MIN);
                    (recency, session_id.clone())
                })
                .collect();
            by_recency.sort();
            let excess = self.transcript_scroll_positions.len() - MAX_TRANSCRIPT_SCROLL_POSITIONS;
            for (_, session_id) in by_recency.into_iter().take(excess) {
                self.transcript_scroll_positions.remove(&session_id);
            }
        }
        if self.transcript_scroll_positions.len() != before {
            self.schedule_transcript_scroll_persist(cx);
        }
    }

    /// Queue a debounced write of the scroll positions to disk.
    fn schedule_transcript_scroll_persist(&mut self, cx: &mut Context<Self>) {
        if !self.is_main_window {
            return;
        }
        if self.transcript_scroll_save_pending.get() {
            return;
        }
        self.transcript_scroll_save_pending.set(true);
        cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(TRANSCRIPT_SCROLL_SAVE_DEBOUNCE)
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _| {
                        this.transcript_scroll_save_pending.set(false);
                        this.persist_transcript_scroll();
                    });
                }
            });
        })
        .detach();
    }

    /// Write the scroll positions to disk.
    pub(crate) fn persist_transcript_scroll(&self) {
        if !self.is_main_window {
            // Secondary "New Window" windows never persist; only the main
            // window's state survives restarts (same rule as `persist_layout`).
            return;
        }
        let positions = self
            .transcript_scroll_positions
            .iter()
            .map(|(session_id, position)| {
                (
                    session_id.clone(),
                    PersistedTranscriptScroll {
                        message_id: position.message_id.clone(),
                        offset_in_row: position.offset_in_row,
                        at_tail: position.at_tail,
                    },
                )
            })
            .collect();
        crate::persistence::store::save_transcript_scroll(PersistedTranscriptScrollState {
            positions,
        });
    }

    /// Seed the remembered positions from the state file.
    ///
    /// Only the main window restores them: a secondary window opens on the
    /// server it was launched against, and its positions are not the ones the
    /// main window will write back.
    pub(crate) fn seed_transcript_scroll_positions(
        &mut self,
        state: Option<&PersistedTranscriptScrollState>,
    ) {
        if !self.is_main_window {
            return;
        }
        let Some(state) = state else {
            return;
        };
        self.transcript_scroll_positions = state
            .positions
            .iter()
            .map(|(session_id, position)| {
                (
                    session_id.clone(),
                    TranscriptScrollPosition {
                        message_id: position.message_id.clone(),
                        // The row index is not persisted: it counts rows of a
                        // window that has since changed. A chat whose anchor
                        // row has an id is restored by id; one without falls
                        // back to the tail.
                        row_index: 0,
                        offset_in_row: position.offset_in_row,
                        at_tail: position.at_tail,
                    },
                )
            })
            .collect();
    }
}
