// Canonical wire type from the shared protobuf schema
// (proto/console/v1/project.proto). Note the timestamps: protojson encodes
// int64 as JSON strings, unlike the old hand-shaped numbers.
//
// The session-matching helpers lived on the hand-written struct as inherent
// methods; generated types are foreign so they move to this extension trait.
// It is re-exported alongside ProjectInfo, so call sites only add the trait
// to their existing console_core import.
pub use console_proto::ProjectInfo;

pub trait ProjectInfoExt {
    fn matches_session(&self, session: &crate::types::session::SessionHeader) -> bool;
    fn matches_session_parts(&self, project_id: Option<&str>, cwd: &str) -> bool;
}

impl ProjectInfoExt for ProjectInfo {
    fn matches_session(&self, session: &crate::types::session::SessionHeader) -> bool {
        self.matches_session_parts(session.project_id.as_deref(), &session.cwd)
    }

    fn matches_session_parts(&self, project_id: Option<&str>, cwd: &str) -> bool {
        (!cwd.is_empty() && cwd == self.path) || project_id == Some(self.id.as_str())
    }
}

/// One repository in the connected GitHub account, as the project pickers
/// show it. Serde names are camelCase because the server serializes its
/// repo list as plain JSON, not through the proto schema.
#[derive(Debug, Clone, PartialEq, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GithubRepo {
    pub full_name: String,
    pub name: String,
    #[serde(default)]
    pub private: bool,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub html_url: String,
    /// HTTPS clone endpoint — what `clone_project` and
    /// `create_session_from_repo` take as their url.
    #[serde(default)]
    pub clone_url: String,
    #[serde(default)]
    pub pushed_at: String,
}

impl GithubRepo {
    /// The URL to clone, falling back to the HTTPS form derived from the
    /// full name when the server's clone_url field is absent.
    pub fn clone_target(&self) -> String {
        if !self.clone_url.is_empty() {
            self.clone_url.clone()
        } else {
            format!("https://github.com/{}.git", self.full_name)
        }
    }
}

/// What `POST /api/sessions/from-repo` returns: the registered (or reused)
/// project plus the fresh session the client should open and prompt.
#[derive(Debug, Clone, serde::Deserialize)]
pub struct RepoTask {
    #[serde(default)]
    pub project: ProjectInfo,
    #[serde(default)]
    pub session: crate::types::session::SessionHeader,
}
