//! Per-session pagination cursors for the transcript.
//!
//! The desktop loads the tail page (most recent `limit` messages) when a
//! chat is opened. Older messages are fetched on demand by passing
//! `next_cursor` as the `before` query parameter to the session API. This
//! module owns the per-session cursor and `has_more` flag, and clears them
//! when the chat is switched away.

use gpui::Context;

use super::ConsoleDesktopApp;

#[derive(Clone, Copy, Debug)]
pub(crate) struct SessionPaginationState {
    pub has_more: bool,
    pub next_cursor: Option<i64>,
}

impl ConsoleDesktopApp {
    /// Forget pagination state for a session whose chat was switched out.
    /// Called from environments.rs alongside the scroll-position clearing.
    pub(crate) fn clear_transcript_pagination(&mut self) {
        self.transcript_pagination.clear();
    }

    /// Look up the pagination state for the session a pane currently shows.
    /// Falls back to `None` if no state exists (e.g. nothing has been loaded
    /// yet, or the session was cleared on branch switch).
    pub(crate) fn pagination_for_session(&self, session_id: &str) -> Option<SessionPaginationState> {
        self.transcript_pagination.get(session_id).copied()
    }

    /// Store the pagination state returned by the most recent `getSession`
    /// response for a session. Overwrites any prior cursor so a re-load
    /// re-anchors the affordance correctly.
    pub(crate) fn set_pagination_for_session(
        &mut self,
        session_id: &str,
        state: SessionPaginationState,
        cx: &mut Context<Self>,
    ) {
        self.transcript_pagination.insert(session_id.to_string(), state);
        // Re-pull the active pane's transcript so the "Load older" button
        // shows or hides in response to the new state.
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        if self.active_session_for_pane(&pane_id).as_deref() == Some(session_id) {
            let transcript = self.transcript_for_pane(&pane_id);
            transcript.update(cx, |t, _| {
                t.set_pagination(state.has_more, state.next_cursor);
            });
        }
    }

    /// Load the next older page for the session shown in `pane_id`.
    /// Prepends messages and keeps scroll position stable.
    pub fn load_older_messages_for_pane(&mut self, pane_id: String, cx: &mut Context<Self>) {
        let Some(session_id) = self.active_session_for_pane(&pane_id).map(|s| s.to_string()) else {
            return;
        };
        let Some(pagination) = self.pagination_for_session(&session_id) else {
            return;
        };
        if !pagination.has_more || pagination.next_cursor.is_none() {
            return;
        }
        let before = pagination.next_cursor.unwrap();
        // Mark loading
        self.transcript_for_pane(&pane_id).update(cx, |t, _| t.set_loading_older(true));
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            match client.sessions.get_paginated(&session_id, Some(50), Some(before)).await {
                Ok(detail) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                // Only prepend if still showing same session
                                if this.active_session_for_pane(&pane_id).as_deref() != Some(session_id.as_str()) {
                                    return;
                                }
                                this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                                    t.prepend_messages(detail.messages, detail.has_more, detail.next_cursor, cx);
                                });
                                this.set_pagination_for_session(
                                    &session_id,
                                    SessionPaginationState {
                                        has_more: detail.has_more,
                                        next_cursor: detail.next_cursor,
                                    },
                                    cx,
                                );
                                cx.notify();
                            });
                        }
                    });
                }
                Err(_) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.transcript_for_pane(&pane_id).update(cx, |t, _| t.set_loading_older(false));
                                cx.notify();
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }
}