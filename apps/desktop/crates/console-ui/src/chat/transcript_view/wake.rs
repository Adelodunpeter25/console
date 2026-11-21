use super::*;

impl TranscriptView {
    /// Insert a live wake turn at the transcript tail. Called when
    /// `backgroundWorkContinue` arrives — it is broadcast before the wake
    /// turn's first model/tool event, so the divider lands ahead of all of it.
    pub fn push_run_boundary(&mut self, label: String, at: i64, cx: &mut Context<Self>) {
        self.live.clear();
        self.pending_results.clear();
        self.turns.push(Turn {
            id: String::new(),
            created_at: Some(at),
            user: None,
            wake_label: Some(label),
            run: None,
            responses: Vec::new(),
        });
        self.content_revisions.push(0);
        self.refresh_list();
        cx.notify();
    }
}
