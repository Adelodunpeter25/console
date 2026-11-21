use super::*;

impl TranscriptView {
    /// Insert a live wake-turn boundary at the transcript tail. Called when
    /// `backgroundWorkContinue` arrives — it is broadcast before the wake
    /// turn's first model/tool event, so the marker lands ahead of all of it.
    pub fn push_run_boundary(&mut self, label: String, at: i64, cx: &mut Context<Self>) {
        self.messages.push(AgentMessage::RunBoundary {
            label,
            at: Some(at),
        });
        self.content_revisions.push(0);
        self.invalidate_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    pub(crate) fn wake_boundary_at(&self, index: usize) -> Option<&str> {
        self.wake_boundaries
            .iter()
            .find(|(boundary_index, _, _)| *boundary_index == index)
            .map(|(_, label, _)| label.as_str())
    }
}
