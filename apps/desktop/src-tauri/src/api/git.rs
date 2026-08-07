use crate::api::ApiClient;
use crate::error::AppResult;

pub async fn get_git_status(client: &ApiClient, path: Option<&str>) -> AppResult<serde_json::Value> {
    match path {
        Some(p) => client.get_with_query("/git/status", &[("path", p)]).await,
        None => client.get("/git/status").await,
    }
}
