use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{AuthStatusResponse, OAuthCallbackDto, OAuthLoginUrlDto};

pub async fn get_auth_status(client: &ApiClient) -> AppResult<AuthStatusResponse> {
    client.get("/auth/status").await
}

pub async fn get_login_url(client: &ApiClient, dto: &OAuthLoginUrlDto) -> AppResult<serde_json::Value> {
    client.post("/auth/login/url", dto).await
}

pub async fn handle_callback(client: &ApiClient, dto: &OAuthCallbackDto) -> AppResult<serde_json::Value> {
    client.post("/auth/login/callback", dto).await
}
