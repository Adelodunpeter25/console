use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitFileEntry {
    pub path: String,
    pub status: String,
    pub staged: bool,
    pub additions: Option<u64>,
    pub deletions: Option<u64>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitStatusSummary {
    pub branch: String,
    pub clean: bool,
    pub files: Vec<GitFileEntry>,
}

impl GitStatusSummary {
    pub fn modified_count(&self) -> usize {
        self.files
            .iter()
            .filter(|f| f.status == "M" && !f.staged)
            .count()
    }
    pub fn staged_count(&self) -> usize {
        self.files.iter().filter(|f| f.staged).count()
    }
    pub fn untracked_count(&self) -> usize {
        self.files.iter().filter(|f| f.status == "?").count()
    }
    pub fn is_clean(&self) -> bool {
        self.clean
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitDiffResponse {
    pub path: Option<String>,
    pub diff: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitBranchInfo {
    pub name: String,
    pub current: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GitBranchesResponse {
    pub branches: Vec<GitBranchInfo>,
    pub is_git_repository: bool,
}
