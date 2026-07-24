use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{CreateSessionDto, SessionDetailResponse, SessionHeader, UpdateSessionDto};

pub async fn list_sessions(client: &ApiClient, cwd: Option<&str>, project_id: Option<&str>) -> AppResult<Vec<SessionHeader>> {
    let mut query: Vec<(&str, &str)> = Vec::new();
    if let Some(cwd) = cwd {
        query.push(("cwd", cwd));
    }
    if let Some(project_id) = project_id {
        query.push(("projectId", project_id));
    }

    if query.is_empty() {
        client.get("/sessions").await
    } else {
        client.get_with_query("/sessions", &query).await
    }
}

pub async fn create_session(client: &ApiClient, dto: &CreateSessionDto) -> AppResult<SessionHeader> {
    client.post("/sessions", dto).await
}

pub async fn get_session(client: &ApiClient, id: &str) -> AppResult<SessionDetailResponse> {
    client.get(&format!("/sessions/{}", id)).await
}

pub async fn update_session(client: &ApiClient, id: &str, dto: &UpdateSessionDto) -> AppResult<SessionHeader> {
    client.patch(&format!("/sessions/{}", id), dto).await
}

pub async fn delete_session(client: &ApiClient, id: &str) -> AppResult<serde_json::Value> {
    client.delete(&format!("/sessions/{}", id)).await
}
