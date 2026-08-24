use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Model {
    pub id: String,
    pub provider: String,
    pub context_window: usize,
    pub supports_images: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelFavorite {
    pub provider: String,
    pub model_id: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderCatalogEntry {
    pub name: String,
    pub display_name: String,
    pub description: String,
    pub models: Vec<Model>,
    pub auth_method: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderModelsResponse {
    pub provider: String,
    pub models: Vec<Model>,
}

/// The provider/model pair a session runs on. Selected in the UI picker and
/// persisted onto the session header, so it lives beside the catalog types
/// rather than inside the UI crate.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SelectedModel {
    pub provider: String,
    pub model_id: String,
}
