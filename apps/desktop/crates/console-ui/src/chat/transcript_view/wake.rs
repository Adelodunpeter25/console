use super::*;
use crate::utils::time::normalize_unix_timestamp;
use console_core::WakeRecord;

/// Slack subtracted from a wake record's receipt time when matching the
/// first persisted wake-turn message. The frame arrives before the turn's
/// messages are written, but clock granularity and write latency make an
/// exact `>=` fragile; a few seconds of slack is safe because the previous
/// turn's messages all predate the wake by the background job's runtime.
const WAKE_MATCH_SLACK_SECS: i64 = 5;

impl TranscriptView {
    /// Store this session's recorded wake turns. Call before `set_messages`
    /// so history loads can rebuild run boundaries; live wakes append via
    /// `push_run_boundary` instead.
    pub fn set_wake_records(&mut self, records: Vec<WakeRecord>, _cx: &mut Context<Self>) {
        self.wake_records = records;
    }

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

    /// Rebuild `wake_boundaries` (message index → label/at) from
    /// `wake_records` after the message list changed. A record matches the
    /// first Assistant or ToolResult message whose normalized `created_at`
    /// is at or after the record's time (minus slack); records are consumed
    /// in `at` order so an earlier wake cannot steal a later wake's first
    /// message.
    pub(super) fn rebuild_run_boundaries(&mut self) {
        self.wake_boundaries.clear();
        if self.wake_records.is_empty() {
            return;
        }
        let mut records = self.wake_records.clone();
        records.sort_by_key(|r| r.at);
        let mut search_from = 0usize;
        for record in records {
            let threshold = record.at - WAKE_MATCH_SLACK_SECS;
            let mut matched = None;
            for (index, message) in self.messages.iter().enumerate().skip(search_from) {
                let created = match message {
                    AgentMessage::Assistant { created_at, .. }
                    | AgentMessage::ToolResult { created_at, .. } => {
                        created_at.and_then(normalize_unix_timestamp)
                    }
                    AgentMessage::User { .. } | AgentMessage::RunBoundary { .. } => continue,
                };
                if let Some(created) = created
                    && created >= threshold
                {
                    matched = Some(index);
                    break;
                }
            }
            if let Some(index) = matched {
                let at =
                    message_created_at(&self.messages[index]).and_then(normalize_unix_timestamp);
                self.wake_boundaries
                    .push((index, record.message.clone(), at));
                search_from = index + 1;
            }
        }
        self.wake_boundaries.sort_by_key(|b| b.0);
    }

    pub(crate) fn wake_boundary_at(&self, index: usize) -> Option<&str> {
        self.wake_boundaries
            .iter()
            .find(|(boundary_index, _, _)| *boundary_index == index)
            .map(|(_, label, _)| label.as_str())
    }
}

fn message_created_at(message: &AgentMessage) -> Option<i64> {
    match message {
        AgentMessage::Assistant { created_at, .. }
        | AgentMessage::ToolResult { created_at, .. } => *created_at,
        AgentMessage::User { created_at, .. } => *created_at,
        AgentMessage::RunBoundary { at, .. } => *at,
    }
}
