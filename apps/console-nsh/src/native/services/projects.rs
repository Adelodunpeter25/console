//! Projects, through console-core's `ProjectService`.

use console_core::ProjectService;

use crate::native::http;
use crate::native::types::ProjectSummary;

pub struct Projects {
    service: ProjectService,
}

impl Projects {
    pub fn new(base_url: &str) -> Self {
        Self {
            service: ProjectService::new(http::transport(base_url)),
        }
    }

    /// The registered projects, newest activity first. Rejects when the
    /// backend fails, so the UI can tell "none" apart from "down".
    pub async fn list(&self) -> Result<Vec<ProjectSummary>, String> {
        Ok(self
            .service
            .list()
            .await
            .map_err(|err| err.to_string())?
            .into_iter()
            .map(|p| ProjectSummary {
                id: p.id,
                name: p.name,
                path: p.path,
            })
            .collect())
    }
}
