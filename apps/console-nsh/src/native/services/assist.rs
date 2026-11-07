//! The assistant's search surface, through console-core's `AssistService` —
//! the composer's `@` mention popup's workspace file index.

use crate::native::http;
use console_core::AssistService;

/// Searches workspace files (`FileSearchResponse` JSON; the popup filters
/// client-side from a query-less load, per-query searches can refine it).
pub async fn search_files(
    base_url: &str,
    session_id: Option<&str>,
    query: &str,
    root: Option<&str>,
    include_dirs: bool,
) -> Result<String, String> {
    let service = AssistService::new(http::transport(base_url));
    let result = service
        .search_files(session_id, query, root, include_dirs)
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}
