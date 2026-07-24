use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ProviderCatalogEntry;

pub async fn list_providers(client: &ApiClient) -> AppResult<Vec<ProviderCatalogEntry>> {
    client.get("/providers").await
}

pub async fn get_models(client: &ApiClient, provider_id: &str) -> AppResult<serde_json::Value> {
    client
        .get(&format!("/providers/{}/models", provider_id))
        .await
}
