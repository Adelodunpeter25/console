use super::*;

impl TranscriptView {
    /// Replace the session's recorded file changes and re-measure the rows
    /// whose chip rows appeared, changed or went away.
    pub fn set_session_changes(
        &mut self,
        changes: Rc<Vec<SessionFileChange>>,
        cx: &mut Context<Self>,
    ) {
        let before: Vec<usize> = self.rows_with_turn_changes();
        self.session_changes = changes;
        self.turn_changes = crate::chat::turn_changes::group_turn_changes(&self.session_changes);
        let after: Vec<usize> = self.rows_with_turn_changes();
        if before.is_empty() && after.is_empty() {
            // No chip row to re-measure, but the pinned summary pill still
            // needs a repaint when its totals change.
            cx.notify();
            return;
        }
        let mut affected: Vec<usize> = before;
        affected.extend(after);
        affected.sort_unstable();
        affected.dedup();
        for index in affected {
            self.list_state.splice(index..index + 1, 1);
        }
        cx.notify();
    }

    /// Forget the recorded file changes. Called when the transcript is blanked
    /// for a different (or brand-new) chat, so the pinned summary pill never
    /// shows the previous chat's totals. Not part of `set_messages`: a chat
    /// load starts its changes request before its messages arrive, and
    /// clearing there could erase the fresh result.
    pub fn clear_session_changes(&mut self, cx: &mut Context<Self>) {
        if !self.session_changes.is_empty() {
            self.set_session_changes(Rc::new(Vec::new()), cx);
        }
    }

    /// Whole-session change totals for the pinned summary pill above the
    /// composer. `None` when the session has not touched any file.
    pub fn changes_summary(&self) -> Option<ChangesSummary> {
        let summary = ChangesSummary::from_changes(&self.session_changes);
        (!summary.is_empty()).then_some(summary)
    }

    pub(super) fn rows_with_turn_changes(&self) -> Vec<usize> {
        crate::chat::turn_changes::rows_with_turn_changes(
            &self.messages,
            &self.turn_changes,
            crate::chat::row::is_hidden_tool_transport,
        )
    }

    /// File changes of the turn a settled assistant message closes. Only the
    /// turn's last visible assistant message carries them: a turn is one user
    /// message and everything the agent did until the next one.
    pub(crate) fn turn_changes_for_assistant(&self, index: usize) -> Option<Rc<TurnChanges>> {
        crate::chat::turn_changes::turn_changes_for_assistant(
            &self.messages,
            &self.turn_changes,
            index,
            crate::chat::row::is_hidden_tool_transport,
        )
    }
}
