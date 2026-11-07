//! Chats, through console-core's `SessionService`.

use console_core::SessionService;

use crate::native::http;
use crate::native::types::SessionSummary;

pub struct Sessions {
    service: SessionService,
}

impl Sessions {
    pub fn new(base_url: &str) -> Self {
        Self {
            service: SessionService::new(http::transport(base_url)),
        }
    }

    /// The chats of one project, or of all projects when `project_id` is
    /// empty, newest first. Rejects when the backend fails, so the UI can
    /// tell "none" apart from "down".
    pub async fn list(&self, project_id: &str) -> Result<Vec<SessionSummary>, String> {
        Ok(self
            .service
            .list(
                None,
                if project_id.is_empty() {
                    None
                } else {
                    Some(project_id)
                },
            )
            .await
            .map_err(|err| err.to_string())?
            .into_iter()
            .map(|s| SessionSummary {
                id: s.id,
                title: s.title,
                status: s.status,
                updated_at: s.updated_at,
                project_id: s.project_id,
            })
            .collect())
    }

    /// Soft-deletes one chat (trash; restorable for 7 days).
    pub async fn delete(&self, id: &str) -> Result<(), String> {
        self.service.delete(id).await.map_err(|err| err.to_string())
    }

    /// The soft-deleted chats (trash), optionally one project's.
    pub async fn list_deleted(&self, project_id: Option<&str>) -> Result<Vec<SessionSummary>, String> {
        Ok(self
            .service
            .list_deleted(project_id)
            .await
            .map_err(|err| err.to_string())?
            .into_iter()
            .map(|s| SessionSummary {
                id: s.id,
                title: s.title,
                status: s.status,
                updated_at: s.updated_at,
                project_id: s.project_id,
            })
            .collect())
    }

    /// Brings a soft-deleted chat back.
    pub async fn restore(&self, id: &str) -> Result<(), String> {
        self.service.restore(id).await.map_err(|err| err.to_string())
    }

    /// Marks a finished chat settled. The server refuses while a run is in
    /// flight or a prompt is queued — the error string surfaces as a message.
    pub async fn settle(&self, id: &str) -> Result<(), String> {
        self.service.settle(id).await.map_err(|err| err.to_string())
    }

    /// Brings a settled chat back to active.
    pub async fn unsettle(&self, id: &str) -> Result<(), String> {
        self.service.unsettle(id).await.map_err(|err| err.to_string())
    }

    /// Forks a chat at one of its messages: the forked header comes back as
    /// JSON (the transcript's fork button opens it as a new tab).
    pub async fn fork(&self, id: &str, message_id: &str) -> Result<String, String> {
        let header = self
            .service
            .fork(id, message_id)
            .await
            .map_err(|err| err.to_string())?;
        serde_json::to_string(&header).map_err(|err| err.to_string())
    }

    /// One chat's file changes (the review tab's list). `turn_index` of
    /// `None` aggregates every turn.
    pub async fn changes(
        &self,
        id: &str,
        turn_index: Option<u64>,
    ) -> Result<String, String> {
        let changes = self
            .service
            .get_changes(id, turn_index)
            .await
            .map_err(|err| err.to_string())?;
        serde_json::to_string(&changes).map_err(|err| err.to_string())
    }

    /// The cached unified diff text of one file change (the review tab's
    /// per-file diff, served straight from the backend's DB).
    pub async fn change_diff(&self, id: &str, path: &str, turn_index: u64) -> Result<String, String> {
        self.service
            .get_change_diff(id, path, turn_index)
            .await
            .map_err(|err| err.to_string())
    }

    /// Marks one file change reviewed (or unreviews it).
    pub async fn set_change_reviewed(
        &self,
        id: &str,
        path: &str,
        turn_index: u32,
        reviewed: bool,
    ) -> Result<(), String> {
        self.service
            .set_change_reviewed(id, path, turn_index, reviewed)
            .await
            .map_err(|err| err.to_string())
    }
}
