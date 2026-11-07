//! The launcher and settings data: drafts, the provider/model catalog, model
//! favorites, usage and settings, through console-core's services. Every
//! response is the service's own serde JSON, passed through untouched — the
//! TSX types are the shapes the desktop's UI matches on.

use console_core::services::{DraftService, ModelFavoriteService, SettingsService};
use console_core::{ProviderService, UsageService};
use nsh_core::JsonValue;

use crate::native::http;

/// One chat's saved composer draft (`null` when there is none).
pub async fn draft(base_url: &str, session_id: &str) -> Result<String, String> {
    let service = DraftService::new(http::transport(base_url));
    let result = service
        .get(session_id)
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// Saves one chat's composer draft. `draft` is the wire JSON of the proto
/// `ChatDraft` (which carries its own `sessionId`); the desktop's draft
/// sync writes the same shape.
pub async fn put_draft(base_url: &str, draft: JsonValue) -> Result<(), String> {
    let service = DraftService::new(http::transport(base_url));
    let parsed: console_proto::ChatDraft =
        serde_json::from_value(draft).map_err(|err| format!("bad draft: {err}"))?;
    service
        .put(&parsed)
        .await
        .map_err(|err| err.to_string())?;
    Ok(())
}

/// The provider catalog: every provider with its available models (the
/// model picker's source).
pub async fn providers(base_url: &str) -> Result<String, String> {
    let service = ProviderService::new(http::transport(base_url));
    let result = service.list().await.map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// One provider's models.
pub async fn provider_models(base_url: &str, provider_id: &str) -> Result<String, String> {
    let service = ProviderService::new(http::transport(base_url));
    let result = service
        .get_models(provider_id)
        .await
        .map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// The user's favorite models (the picker's star state).
pub async fn model_favorites(base_url: &str) -> Result<String, String> {
    let service = ModelFavoriteService::new(http::transport(base_url));
    let result = service.list().await.map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// Usage reports per provider (`null` where a provider has no report).
pub async fn usage(base_url: &str) -> Result<String, String> {
    let service = UsageService::new(http::transport(base_url));
    let result = service.get_all().await.map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}

/// The user's console settings.
pub async fn settings(base_url: &str) -> Result<String, String> {
    let service = SettingsService::new(http::transport(base_url));
    let result = service.get().await.map_err(|err| err.to_string())?;
    serde_json::to_string(&result).map_err(|err| err.to_string())
}
