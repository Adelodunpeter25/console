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
