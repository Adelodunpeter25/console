use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ProjectInfo;

pub async fn list_projects(client: &ApiClient) -> AppResult<Vec<ProjectInfo>> {
    client.get("/projects").await
}

pub async fn add_project(client: &ApiClient, path: &str) -> AppResult<ProjectInfo> {
    let body = serde_json::json!({ "path": path });
    client.post("/projects", &body).await
}
