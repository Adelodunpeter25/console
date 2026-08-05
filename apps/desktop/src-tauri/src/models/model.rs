use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Model {
    pub id: String,
    pub provider: String,
    pub context_window: u64,
    pub supports_images: Option<bool>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderCatalogEntry {
    pub name: String,
    pub display_name: String,
    pub description: String,
    pub models: Vec<Model>,
}
