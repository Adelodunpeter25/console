use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProjectInfo {
    pub id: String,
    pub name: String,
    pub path: String,
    pub created_at: i64,
    pub updated_at: i64,
}

impl ProjectInfo {
    pub fn matches_session(&self, session: &crate::types::session::SessionHeader) -> bool {
        self.matches_session_parts(session.project_id.as_deref(), &session.cwd)
    }

    pub fn matches_session_parts(&self, project_id: Option<&str>, cwd: &str) -> bool {
        project_id == Some(self.id.as_str()) || (!cwd.is_empty() && cwd == self.path)
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitStatusSummary {
    pub branch: String,
    pub modified_count: usize,
    pub staged_count: usize,
    pub untracked_count: usize,
    pub is_clean: bool,
}
