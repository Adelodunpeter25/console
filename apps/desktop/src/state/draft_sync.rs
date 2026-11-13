//! Server-synced chat drafts: composer text and mentions follow the chat
//! across devices. File comments are saved on their own, per project.
//!
//! Pull on open, push after a typing pause. The server keeps one draft per
//! chat and the newest `updated_at` wins, so every push carries the whole
//! draft (a removed comment or cleared text has to propagate) and a clear is
//! pushed as an empty draft rather than skipped. The local drafts file stays
//! the offline cache and the source for attachments, which are not synced yet.

use std::time::Duration;

use console_proto::{ChatDraft, DraftMention};
use console_ui::ComposerMention;
use gpui::Context;

use super::app::ConsoleDesktopApp;
use crate::persistence;

/// Trailing delay before a draft goes to the server; typing bursts coalesce.
const DRAFT_PUSH_DEBOUNCE: Duration = Duration::from_millis(800);

impl ConsoleDesktopApp {
    pub(crate) fn schedule_draft_push(&mut self, session_id: String, cx: &mut Context<Self>) {
        if !self.drafts_push_pending.insert(session_id.clone()) {
            return;
        }
        cx.spawn(async move |entity, cx| {
            cx.background_executor().timer(DRAFT_PUSH_DEBOUNCE).await;
            let prepared = cx.update(|cx| {
                let app = entity.upgrade()?;
                app.update(cx, |this, _| {
                    this.drafts_push_pending.remove(&session_id);
                    let draft = this.build_server_draft(&session_id)?;
                    Some((this.client.clone(), draft))
                })
            });
            let Some((client, draft)) = prepared else {
                return;
            };
            // Offline or server error: the local drafts file still has it, and
            // the next edit or open retries.
            let Ok(response) = client.drafts.put(&draft).await else {
                return;
            };
            if response.applied {
                return;
            }
            // Another device wrote later: take its draft instead of fighting it.
            if let Some(remote) = response.draft {
                cx.update(|cx| {
                    let Some(handle) = cx.active_window() else {
                        return;
                    };
                    let _ = handle.update(cx, |_, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.adopt_remote_draft(remote, cx));
                        }
                    });
                });
            }
        })
        .detach();
    }

    /// The pane whose chat is `session_id`, preferring the focused one.
    fn pane_showing_session(&self, session_id: &str) -> Option<String> {
        if let Some(active) = self.active_pane_id.as_deref()
            && self.active_session_for_pane(active).as_deref() == Some(session_id)
        {
            return Some(active.to_string());
        }
        self.workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| self.active_session_for_pane(&leaf.id).as_deref() == Some(session_id))
            .map(|leaf| leaf.id.clone())
    }

    /// The draft as the server stores it. `None` when the chat has no local draft state to push.
    fn build_server_draft(&self, session_id: &str) -> Option<ChatDraft> {
        let local = self.drafts.get(session_id);
        Some(ChatDraft {
            session_id: session_id.to_string(),
            text: local.map(|draft| draft.prompt.clone()).unwrap_or_default(),
            mentions: local
                .map(|draft| {
                    draft
                        .mentions
                        .iter()
                        .map(|mention| DraftMention {
                            start: mention.start as u32,
                            end: mention.end as u32,
                            path: mention.path.clone(),
                            label: mention.label.clone().unwrap_or_default(),
                        })
                        .collect()
                })
                .unwrap_or_default(),
            comments: Vec::new(),
            updated_at: chrono::Utc::now().timestamp_millis(),
        })
    }

    /// Fetch the chat's synced draft as it opens and adopt it when newer.
    pub(crate) fn pull_draft(&mut self, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let Ok(Some(remote)) = client.drafts.get(&session_id).await else {
                return;
            };
            cx.update(|cx| {
                let Some(handle) = cx.active_window() else {
                    return;
                };
                let _ = handle.update(cx, |_, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.adopt_remote_draft(remote, cx));
                    }
                });
            });
        })
        .detach();
    }

    /// Apply a server draft if it is newer than what this device holds.
    /// Never touches a draft the user has edited since it was requested:
    /// those edits stamp a newer local time.
    fn adopt_remote_draft(
        &mut self,
        remote: ChatDraft,
        cx: &mut Context<Self>,
    ) {
        let session_id = remote.session_id.clone();
        // A push is queued, so this device has edits the server has not seen
        // yet; adopting now would overwrite them.
        if self.drafts_push_pending.contains(&session_id) {
            return;
        }
        let local_ms = self
            .drafts
            .get(&session_id)
            .map(|draft| draft.updated_at.saturating_mul(1000))
            .unwrap_or(0);
        if remote.updated_at <= local_ms {
            return;
        }

        let mentions: Vec<ComposerMention> = remote
            .mentions
            .iter()
            .map(|mention| ComposerMention {
                range: mention.start as usize..mention.end as usize,
                path: mention.path.clone(),
                label: mention.label.clone(),
            })
            .collect();

        let attachments = self
            .drafts
            .get(&session_id)
            .map(|draft| draft.attachments.clone())
            .unwrap_or_default();
        let annotations = self
            .drafts
            .get(&session_id)
            .map(|draft| draft.annotations.clone())
            .unwrap_or_default();
        if remote.text.trim().is_empty() && attachments.is_empty() && annotations.is_empty() {
            self.drafts.remove(&session_id);
            self.sidebar_draft_ids.remove(&session_id);
        } else {
            self.drafts.insert(
                session_id.clone(),
                persistence::store::PersistedDraft {
                    prompt: remote.text.clone(),
                    updated_at: remote.updated_at / 1000,
                    mentions: remote
                        .mentions
                        .iter()
                        .map(|mention| persistence::store::PersistedDraftMention {
                            start: mention.start as usize,
                            end: mention.end as usize,
                            path: mention.path.clone(),
                            label: Some(mention.label.clone()),
                        })
                        .collect(),
                    context_files: console_shared::composer::mentions::context_file_paths(
                        &mentions,
                    ),
                    attachments,
                    annotations,
                    floating: None,
                },
            );
        }
        self.schedule_drafts_save(cx);

        let Some(pane_id) = self.pane_showing_session(&session_id) else {
            cx.notify();
            return;
        };
        let composer = self.composer_for_pane(&pane_id);
        let context_files = console_shared::composer::mentions::context_file_paths(&mentions);
        // The draft usually just echoes this device's own last push; rewriting
        // the field then would only move the caret.
        if composer.read(cx).content() != remote.text.as_str() {
            composer.update(cx, |input, cx| {
                if remote.text.is_empty() {
                    input.clear(cx);
                } else {
                    input.set_content_with_mentions(remote.text.clone(), mentions, cx);
                }
                input.set_context_files(context_files);
                cx.notify();
            });
        }
        cx.notify();
    }
}
