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
}
