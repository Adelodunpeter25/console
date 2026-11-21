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

    pub fn clear_session_changes(&mut self, cx: &mut Context<Self>) {
        if !self.session_changes.is_empty() {
            self.set_session_changes(Rc::new(Vec::new()), cx);
        }
    }

    pub fn changes_summary(&self) -> Option<ChangesSummary> {
        let summary = ChangesSummary::from_changes(&self.session_changes);
        (!summary.is_empty()).then_some(summary)
    }

    pub(super) fn rows_with_turn_changes(&self) -> Vec<usize> {
        crate::chat::turn_changes::rows_with_turn_changes(&self.turns, &self.turn_changes)
    }

    pub(crate) fn turn_changes_for_turn(&self, turn_index: usize) -> Option<Rc<TurnChanges>> {
        crate::chat::turn_changes::turn_changes_for_turn(
            &self.turns,
            &self.turn_changes,
            turn_index,
        )
    }
}
