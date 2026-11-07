//! Files and diffs, through console-core's `FsService` and `GitService` —
//! the read surface the viewer tabs and inspector need. Every response is
//! the service's own serde JSON, passed through untouched: the TSX types are
//! the same shapes the desktop's UI matches on.

use console_core::{FsService, GitService};

use crate::native::http;

/// Browses one directory (`None` = the server's default). Entries with
/// names, kinds and sizes.
pub async fn browse(base_url: &str, path: Option<&str>) -> Result<String, String> {
    let service = FsService::new(http::transport(base_url));
    let result = service.browse(path).await.map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// One file's content and metadata (the file viewer's source).
pub async fn read_file(base_url: &str, path: &str) -> Result<String, String> {
    let service = FsService::new(http::transport(base_url));
    let result = service
        .read_file(path)
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// The git status summary for a working tree: branch, ahead/behind,
/// changed files. The server requires the repo path.
pub async fn git_status(base_url: &str, path: &str) -> Result<String, String> {
    let service = GitService::new(http::transport(base_url));
    let result = service
        .get_status(Some(path))
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// One file's diff against HEAD (the diff viewer's source), or the whole
/// tree's when `file_path` is empty. The server requires the repo path.
pub async fn git_diff(
    base_url: &str,
    repo_path: &str,
    file_path: Option<&str>,
) -> Result<String, String> {
    let service = GitService::new(http::transport(base_url));
    let result = service
        .get_diff(Some(repo_path), file_path)
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// The working tree's branches (the checkout menu's source). The server
/// requires the repo path.
pub async fn git_branches(base_url: &str, path: &str) -> Result<String, String> {
    let service = GitService::new(http::transport(base_url));
    let result = service
        .list_branches(Some(path))
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}
