use console_core::SessionHeaderExt;
use console_ui::{ComposerMention, DraftSummary};
use gpui::Context;


use crate::state::app::ConsoleDesktopApp;
use crate::state::types::{InMemoryDraft, InMemoryDraftMention};

impl ConsoleDesktopApp {
    pub fn draft_summaries(&self) -> Vec<DraftSummary> {
        let mut summaries = Vec::new();
        for session_id in &self.sidebar_draft_ids {
            let Some(session) = self.sessions.iter().find(|s| &s.id == session_id) else {
                continue;
            };
            let updated_at = self
                .drafts
                .get(session_id)
                .map(|d| d.updated_at)
                .unwrap_or(0);
            summaries.push(DraftSummary {
                session_id: Some(session.id.clone()),
                title: session.display_title().to_string(),
                preview: String::new(),
                project_name: None,
                updated_at,
            });
        }
        summaries
    }

    pub fn get_draft_with_mentions(
        &self,
        session_id: Option<&str>,
    ) -> Option<(String, Vec<ComposerMention>)> {
        self.get_draft_with_mentions_for_key(session_id.unwrap_or("new_chat"))
    }

    pub(crate) fn get_draft_with_mentions_for_key(
        &self,
        key: &str,
    ) -> Option<(String, Vec<ComposerMention>)> {
        self.drafts.get(key).map(|draft| {
            (
                draft.prompt.clone(),
                draft
                    .mentions
                    .iter()
                    .map(|mention| ComposerMention {
                        range: mention.start..mention.end,
                        path: mention.path.clone(),
                        label: mention.label.clone().unwrap_or_else(|| {
                            std::path::Path::new(&mention.path)
                                .file_name()
                                .and_then(|n| n.to_str())
                                .unwrap_or(&mention.path)
                                .to_string()
                        }),
                    })
                    .collect(),
            )
        })
    }

    pub fn get_draft_context_files(&self, session_id: Option<&str>) -> Vec<String> {
        self.get_draft_context_files_for_key(session_id.unwrap_or("new_chat"))
    }

    pub(crate) fn get_draft_context_files_for_key(&self, key: &str) -> Vec<String> {
        self.drafts
            .get(key)
            .map(|draft| draft.context_files.clone())
            .unwrap_or_default()
    }

    /// Restores the draft (text, mentions, context files, attachments, annotations)
    /// into the composer of the specified pane.
    pub fn restore_draft_for_pane(
        &mut self,
        pane_id: &str,
        session_id: Option<&str>,
        cx: &mut Context<Self>,
    ) {
        // A desk pane that has not rendered yet has no composer of its own:
        // composer_for_pane would fall back to the shared pane-main input,
        // whose Edited handler resolves "pane-main" — absent from this
        // workspace — and saves the draft under "new_chat". The pane's real
        // composer is created on render and ensure_workspace_pane_state
        // restores the draft then.
        if pane_id != "pane-main" && !self.workspace_pane_states.contains_key(pane_id) {
            return;
        }
        let draft = self.get_draft_with_mentions(session_id);
        let draft_ctx_files = self.get_draft_context_files(session_id);
        let key = session_id.unwrap_or("new_chat");
        let draft_attachments = self
            .drafts
            .get(key)
            .map(|draft| draft.attachments.clone())
            .unwrap_or_default();
        self.set_attachments_for_pane(pane_id, draft_attachments);
        let draft_annotations = self
            .drafts
            .get(key)
            .map(|draft| draft.annotations.clone())
            .unwrap_or_default();
        self.set_annotations_for_pane(pane_id, draft_annotations);
        self.history_draft_attachments.remove(pane_id);
        self.history_draft_annotations.remove(pane_id);
        self.autocomplete_states.remove(pane_id);
        self.question_input_for_pane(pane_id)
            .update(cx, |input, cx| input.clear(cx));
        self.composer_for_pane(pane_id).update(cx, |input, cx| {
            input.set_prompt_history(Vec::new(), cx);
            if let Some((draft_text, mentions)) = draft {
                input.set_content_with_mentions(draft_text, mentions, cx);
            } else {
                input.clear(cx);
            }
            input.set_context_files(draft_ctx_files);
            cx.notify();
        });
    }

    /// Persist the pane's current composer text, mentions, attachments, and
    /// annotations under its active session's draft.
    pub(crate) fn save_pane_draft(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let session_id = self.active_session_for_pane(pane_id);
        let (text, mentions) = {
            let input = self.composer_for_pane(pane_id).read(cx);
            (input.content().to_string(), input.mentions().to_vec())
        };
        let attachments = self.attachments_for_pane(pane_id);
        let annotations = self.annotations_for_pane(pane_id);
        self.save_draft_for_session_with_context(
            session_id.as_deref(),
            &text,
            &mentions,
            &[],
            &attachments,
            &annotations,
            cx,
        );
        if let Some(session_id) = session_id {
            self.schedule_draft_push(session_id, cx);
        }
    }

    pub fn save_draft_for_session_with_context(
        &mut self,
        session_id: Option<&str>,
        text: &str,
        mentions: &[ComposerMention],
        context_files: &[String],
        attachments: &[console_core::ImageAttachment],
        annotations: &[console_core::BrowserElementAnnotation],
        cx: &mut Context<Self>,
    ) {
        let key = session_id.unwrap_or("new_chat");
        self.save_draft_for_key(
            key,
            text,
            mentions,
            context_files,
            attachments,
            annotations,
            cx,
        );
    }

    pub(crate) fn save_draft_for_key(
        &mut self,
        key: &str,
        text: &str,
        mentions: &[ComposerMention],
        context_files: &[String],
        attachments: &[console_core::ImageAttachment],
        annotations: &[console_core::BrowserElementAnnotation],
        cx: &mut Context<Self>,
    ) {
        let key = key.to_string();
        let _ = cx;
        if text.trim().is_empty()
            && context_files.is_empty()
            && attachments.is_empty()
            && annotations.is_empty()
        {
            self.drafts.remove(&key);
        } else {
            let in_memory_mentions = mentions
                .iter()
                .map(|mention| InMemoryDraftMention {
                    start: mention.range.start,
                    end: mention.range.end,
                    path: mention.path.clone(),
                    label: Some(mention.label.clone()),
                })
                .collect::<Vec<_>>();
            let existing_floating = self.drafts.get(&key).and_then(|d| d.floating.clone());
            let changed = match self.drafts.get(&key) {
                Some(existing) => {
                    existing.prompt != text
                        || existing.mentions != in_memory_mentions
                        || existing.context_files != context_files
                        || existing.attachments != attachments
                        || existing.annotations != annotations
                }
                None => true,
            };
            if changed {
                self.drafts.insert(
                    key,
                    InMemoryDraft {
                        prompt: text.to_string(),
                        updated_at: chrono::Utc::now().timestamp_millis(),
                        mentions: in_memory_mentions,
                        context_files: context_files.to_vec(),
                        attachments: attachments.to_vec(),
                        annotations: annotations.to_vec(),
                        floating: existing_floating,
                    },
                );
            }
        }
    }

    pub fn clear_draft_for_session(&mut self, session_id: Option<&str>, cx: &mut Context<Self>) {
        let key = session_id.unwrap_or("new_chat");
        self.drafts.remove(key);
        if let Some(session_id) = session_id {
            self.sidebar_draft_ids.remove(session_id);
            self.schedule_draft_push(session_id.to_string(), cx);
        }
    }

    /// Called when a tab closes — commits the current draft state to the sidebar.
    /// If `text` is non-empty, the session appears in the draft sidebar.
    /// If empty, it is removed from the sidebar.
    pub fn commit_draft_to_sidebar_with_context(
        &mut self,
        session_id: &str,
        text: &str,
        mentions: &[ComposerMention],
        context_files: &[String],
        attachments: &[console_core::ImageAttachment],
        annotations: &[console_core::BrowserElementAnnotation],
        cx: &mut Context<Self>,
    ) {
        self.save_draft_for_session_with_context(
            Some(session_id),
            text,
            mentions,
            context_files,
            attachments,
            annotations,
            cx,
        );
        if text.trim().is_empty() && attachments.is_empty() && annotations.is_empty() {
            self.sidebar_draft_ids.remove(session_id);
        } else {
            self.sidebar_draft_ids.insert(session_id.to_string());
        }
    }

    /// Called when submitting — removes the session from the sidebar draft list immediately.
    pub fn revoke_sidebar_draft(&mut self, session_id: &str) {
        self.sidebar_draft_ids.remove(session_id);
    }
}
