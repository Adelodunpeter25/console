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

    pub fn draft_summaries(&self, open_sessions: &std::collections::HashSet<String>) -> Vec<DraftSummary> {
        let mut summaries = Vec::new();
        for (key, draft) in &self.drafts {
            let prompt_trimmed = draft.prompt.trim();
            if prompt_trimmed.is_empty() {
                continue;
            }
            // Don't show a draft entry for sessions the user is currently viewing.
            if key != "new_chat" && open_sessions.contains(key) {
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
}
