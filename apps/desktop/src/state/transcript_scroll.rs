//! Per-session transcript scroll persistence.
//!
//! The transcript itself owns GPUI's virtualized list state, while this module
//! owns the app-level mapping from session ids to logical scroll anchors. A row
//! anchor survives variable-height Markdown remeasurement better than a raw
//! pixel offset.

use gpui::Context;

use super::ConsoleDesktopApp;

#[derive(Clone, Copy, Debug)]
pub(crate) struct TranscriptScrollPosition {
    pub row_index: usize,
    pub offset_in_row: f32,
    pub at_tail: bool,
}

impl ConsoleDesktopApp {
    /// Snapshot the active chat's logical list position before replacing its
    /// transcript with another session.
    pub(crate) fn save_transcript_scroll_position(&mut self, cx: &mut Context<Self>) {
        let Some(session_id) = self.selected_session_id.clone() else {
            return;
        };
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        self.save_transcript_scroll_position_for_pane(&pane_id, &session_id, cx);
    }

    pub(crate) fn save_transcript_scroll_position_for_pane(
        &mut self,
        pane_id: &str,
        session_id: &str,
        cx: &mut Context<Self>,
    ) {
        let Some((row_index, offset_in_row, at_tail)) =
            self.transcript_for_pane(pane_id).read(cx).scroll_anchor()
        else {
            return;
        };
        self.transcript_scroll_positions.insert(
            session_id.to_string(),
            TranscriptScrollPosition {
                row_index,
                offset_in_row,
                at_tail,
            },
        );
    }
}
