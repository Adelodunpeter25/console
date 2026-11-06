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

    /// The registered projects, newest activity first.
    pub async fn list(&self) -> Vec<ProjectSummary> {
        self.service
            .list()
            .await
            .unwrap_or_default()
            .into_iter()
            .map(|p| ProjectSummary {
                id: p.id,
                name: p.name,
                path: p.path,
            })
            .collect()
    }
}
