use super::*;

impl TranscriptView {
    pub fn set_prompts(
        &mut self,
        prompts: Vec<console_core::SessionPrompt>,
        cx: &mut Context<Self>,
    ) {
        // Open at the newest prompts, which are the ones most likely wanted.
        if let Some(last) = prompts.len().checked_sub(1) {
            self.prompts_scroll
                .scroll_to_item_strict(last, ScrollStrategy::Bottom);
        }
        self.prompts = Rc::new(prompts);
        cx.notify();
    }

    pub fn is_prompts_open(&self) -> bool {
        self.prompts_open
    }

    pub fn close_prompts(&mut self, cx: &mut Context<Self>) {
        if self.prompts_open {
            self.prompts_open = false;
            cx.notify();
        }
    }

    /// Scroll so the user prompt `from_end` prompts back from the newest (0 =
    /// newest) is at the top of the viewport. Counting from the end needs no
    /// message id, which prompts sent this session do not have yet. Returns
    /// false when that prompt is not loaded.
    pub fn jump_to_prompt(&mut self, from_end: usize, cx: &mut Context<Self>) -> bool {
        let user_rows: Vec<usize> = self
            .messages
            .iter()
            .enumerate()
            .filter(|(_, message)| matches!(message, AgentMessage::User { .. }))
            .map(|(row, _)| row)
            .collect();
        let Some(row) = user_rows
            .len()
            .checked_sub(from_end + 1)
            .map(|at| user_rows[at])
        else {
            return false;
        };
        self.animate_scroll_to_target(ScrollTarget::Prompt(row), cx);
        true
    }

    /// When the newest user prompt was sent, in unix seconds. A run attached
    /// after a restart has no live start time; this is the closest persisted one.
    pub fn latest_user_prompt_at(&self) -> Option<i64> {
        self.messages
            .iter()
            .rev()
            .find_map(|message| match message {
                AgentMessage::User { created_at, .. } => {
                    created_at.and_then(crate::utils::time::normalize_unix_timestamp)
                }
                _ => None,
            })
    }
}
