use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{FileSearchResponse, SlashCommandInfo};

pub async fn list_commands(client: &ApiClient, session_id: &str) -> AppResult<Vec<SlashCommandInfo>> {
    client.get(&format!("/assist/{}/commands", session_id)).await
}

pub async fn search_files(
    client: &ApiClient,
    session_id: &str,
    query: &str,
) -> AppResult<FileSearchResponse> {
    client
        .get_with_query(
            &format!("/assist/{}/search", session_id),
            &[("q", query)],
        )
        .await
}
