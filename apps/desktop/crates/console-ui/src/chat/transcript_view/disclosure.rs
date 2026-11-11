use super::*;

impl TranscriptView {
    /// Expand or collapse one turn's changed-file row and re-measure it.
    pub(crate) fn toggle_turn_changes(
        &mut self,
        row: usize,
        turn_id: String,
        cx: &mut Context<Self>,
    ) {
        {
            let mut expanded = self.expanded_turn_changes.borrow_mut();
            if !expanded.remove(&turn_id) {
                expanded.insert(turn_id);
            }
        }
        self.list_state.splice(row..row + 1, 1);
        cx.notify();
    }

    /// Whether a user message's folded text is expanded via Show more.
    /// Keyed by server id (stable across pagination), falling back to the
    /// row key for optimistic messages that lack one.
    pub fn user_message_expanded(&self, key: &str) -> bool {
        self.expanded_user_messages.borrow().contains(key)
    }

    /// Expand or collapse one user message's folded text and re-measure it.
    pub(crate) fn toggle_user_message_expanded(
        &mut self,
        key: String,
        row: usize,
        cx: &mut Context<Self>,
    ) {
        {
            let mut expanded = self.expanded_user_messages.borrow_mut();
            if !expanded.remove(&key) {
                expanded.insert(key);
            }
        }
        self.list_state.splice(row..row + 1, 1);
        cx.notify();
    }

    pub(crate) fn apply_thinking_toggle(
        &mut self,
        thinking_id: String,
        expanded: bool,
        cx: &mut Context<Self>,
    ) {
        let mut expanded_state = self.thinking_expanded.borrow_mut();
        if expanded {
            expanded_state.insert(thinking_id);
        } else {
            expanded_state.remove(&thinking_id);
        }
        drop(expanded_state);
        cx.notify();
    }

    pub(crate) fn apply_tool_calls_action(
        &mut self,
        action: ToolCallsAction,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let mut state = self.tool_calls_state.borrow_mut();
        match action {
            ToolCallsAction::ToggleRun { run_id, expanded } => {
                if expanded {
                    state.expanded_runs.insert(run_id.clone());
                    state.collapsed_runs.remove(&run_id);
                } else {
                    state.expanded_runs.remove(&run_id);
                    state.collapsed_runs.insert(run_id);
                }
            }
            ToolCallsAction::ToggleCall { call_id, expanded } => {
                if expanded {
                    state.expanded_calls.insert(call_id);
                } else {
                    state.expanded_calls.remove(&call_id);
                }
            }
            ToolCallsAction::ToggleThinking { id, expanded } => {
                if expanded {
                    state.thinking_expanded.insert(id);
                } else {
                    state.thinking_expanded.remove(&id);
                }
            }
            ToolCallsAction::ViewSubagentInPanel { call_id } => {
                let handler = self.on_view_subagent.clone();
                drop(state);
                if let Some(handler) = handler {
                    handler(call_id, window, cx);
                }
                return;
            }
        }
        drop(state);
        cx.notify();
    }
}
