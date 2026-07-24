use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ProviderCatalogEntry;

#[tauri::command]
pub async fn list_providers() -> AppResult<Vec<ProviderCatalogEntry>> {
    let client = ApiClient::new();
    crate::api::providers::list_providers(&client).await
}

#[tauri::command]
pub async fn get_provider_models(provider_id: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::providers::get_models(&client, &provider_id).await
}
