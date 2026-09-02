use console_ui::DraftSummary;
use std::collections::HashSet;

use crate::persistence;
use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    #[allow(dead_code)]
    pub fn draft_session_ids(&self) -> HashSet<String> {
        self.drafts
            .iter()
            .filter(|(k, v)| k.as_str() != "new_chat" && !v.prompt.trim().is_empty())
            .map(|(k, _)| k.clone())
            .collect()
    }

    pub fn draft_summaries(&self) -> Vec<DraftSummary> {
        let mut summaries = Vec::new();
        for (key, draft) in &self.drafts {
            let prompt_trimmed = draft.prompt.trim();
            if prompt_trimmed.is_empty() {
                continue;
            }
            // Session drafts only appear in the sidebar once confirmed (tab was closed
            // with unsent text). new_chat is always shown when it has text.
            if key != "new_chat" && !self.sidebar_draft_ids.contains(key) {
                continue;
            }
            let first_line = prompt_trimmed.lines().next().unwrap_or("").trim().to_string();
            let preview = if first_line.chars().count() > 42 {
                let truncated: String = first_line.chars().take(40).collect();
                format!("{truncated}…")
            } else {
                first_line
            };
            let project_name = if key == "new_chat" {
                self.active_pane_id
                    .as_deref()
                    .and_then(|pane_id| self.selected_project_for_pane(pane_id))
                    .map(|p| p.name.clone())
            } else if let Some(session) = self.sessions.iter().find(|s| &s.id == key) {
                self.projects
                    .iter()
                    .find(|project| project.matches_session(session))
                    .map(|project| project.name.clone())
            } else {
                None
            };

            if key == "new_chat" {
                summaries.push(DraftSummary {
                    session_id: None,
                    title: "New Chat".to_string(),
                    preview,
                    project_name,
                    updated_at: draft.updated_at,
                });
            } else if let Some(session) = self.sessions.iter().find(|s| &s.id == key) {
                summaries.push(DraftSummary {
                    session_id: Some(session.id.clone()),
                    title: session.display_title().to_string(),
                    preview,
                    project_name,
                    updated_at: draft.updated_at,
                });
            }
        }
        summaries.sort_by(|a, b| b.updated_at.cmp(&a.updated_at));
        summaries
    }

    pub fn get_draft_for_session(&self, session_id: Option<&str>) -> Option<&str> {
        let key = session_id.unwrap_or("new_chat");
        self.drafts.get(key).map(|d| d.prompt.as_str())
    }

    pub fn save_draft_for_session(&mut self, session_id: Option<&str>, text: &str) {
        let key = session_id.unwrap_or("new_chat").to_string();
        if text.trim().is_empty() {
            if self.drafts.remove(&key).is_some() {
                persistence::store::save_drafts(self.drafts.clone());
            }
        } else {
            let changed = match self.drafts.get(&key) {
                Some(existing) => existing.prompt != text,
                None => true,
            };
            if changed {
                self.drafts.insert(
                    key,
                    persistence::store::PersistedDraft {
                        prompt: text.to_string(),
                        updated_at: chrono::Utc::now().timestamp(),
                    },
                );
                persistence::store::save_drafts(self.drafts.clone());
            }
        }
    }

    pub fn clear_draft_for_session(&mut self, session_id: Option<&str>) {
        let key = session_id.unwrap_or("new_chat");
        if self.drafts.remove(key).is_some() {
            persistence::store::save_drafts(self.drafts.clone());
        }
    }

    /// Called when a tab closes — commits the current draft state to the sidebar.
    /// If `text` is non-empty, the session appears in the draft sidebar.
    /// If empty, it is removed from the sidebar.
    pub fn commit_draft_to_sidebar(&mut self, session_id: &str, text: &str) {
        self.save_draft_for_session(Some(session_id), text);
        if text.trim().is_empty() {
            self.sidebar_draft_ids.remove(session_id);
        } else {
            self.sidebar_draft_ids.insert(session_id.to_string());
        }
    }

    /// Called when submitting — removes the session from the sidebar draft list immediately.
    pub fn revoke_sidebar_draft(&mut self, session_id: &str) {
        self.sidebar_draft_ids.remove(session_id);
    }

    /// Discard a draft via the sidebar context menu.
    /// `key` is the draft map key: a session id or `"new_chat"`.
    pub fn discard_draft(&mut self, key: &str) {
        let is_new_chat = key == "new_chat";
        let session_opt = if is_new_chat { None } else { Some(key) };
        self.clear_draft_for_session(session_opt);
        if !is_new_chat {
            self.sidebar_draft_ids.remove(key);
        }
    }
}
