use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::FsTreeEntry;

pub async fn browse_directory(client: &ApiClient, path: Option<&str>) -> AppResult<Vec<FsTreeEntry>> {
    match path {
        Some(p) => client.get_with_query("/fs/browse", &[("path", p)]).await,
        None => client.get("/fs/browse").await,
    }
}

pub async fn pick_folder(client: &ApiClient) -> AppResult<serde_json::Value> {
    client.post("/fs/pick-folder", &serde_json::json!({})).await
}

pub async fn get_directory_tree(
    client: &ApiClient,
    path: Option<&str>,
    depth: Option<u32>,
) -> AppResult<serde_json::Value> {
    let depth_str;
    let mut query: Vec<(&str, &str)> = Vec::new();
    if let Some(p) = path {
        query.push(("path", p));
    }
    if let Some(d) = depth {
        depth_str = d.to_string();
        query.push(("depth", &depth_str));
    }
    if query.is_empty() {
        client.get("/fs/tree").await
    } else {
        client.get_with_query("/fs/tree", &query).await
    }
}

pub async fn read_file(
    client: &ApiClient,
    path: &str,
    start_line: Option<u32>,
    end_line: Option<u32>,
) -> AppResult<serde_json::Value> {
    let start_str;
    let end_str;
    let mut query: Vec<(&str, &str)> = vec![("path", path)];
    if let Some(s) = start_line {
        start_str = s.to_string();
        query.push(("startLine", &start_str));
    }
    if let Some(e) = end_line {
        end_str = e.to_string();
        query.push(("endLine", &end_str));
    }
    client.get_with_query("/fs/file", &query).await
}

pub async fn write_file(client: &ApiClient, path: &str, content: &str) -> AppResult<serde_json::Value> {
    let body = serde_json::json!({ "path": path, "content": content });
    client.post("/fs/file", &body).await
}

pub async fn delete_file(client: &ApiClient, path: &str) -> AppResult<serde_json::Value> {
    client.delete_with_query("/fs/file", &[("path", path)]).await
}

pub async fn create_directory(client: &ApiClient, path: &str) -> AppResult<serde_json::Value> {
    let body = serde_json::json!({ "path": path });
    client.post("/fs/dir", &body).await
}

pub async fn delete_directory(client: &ApiClient, path: &str) -> AppResult<serde_json::Value> {
    client.delete_with_query("/fs/dir", &[("path", path)]).await
}
