use serde::{Deserialize, Deserializer, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ThinkingLevel {
    None,
    Minimal,
    Low,
    Medium,
    High,
    #[serde(rename = "xhigh")]
    XHigh,
    Max,
}

impl ThinkingLevel {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Minimal => "minimal",
            Self::Low => "low",
            Self::Medium => "medium",
            Self::High => "high",
            Self::XHigh => "xhigh",
            Self::Max => "max",
        }
    }

    pub fn label(&self) -> &'static str {
        match self {
            Self::None => "None",
            Self::Minimal => "Minimal",
            Self::Low => "Low",
            Self::Medium => "Medium",
            Self::High => "High",
            Self::XHigh => "Xhigh",
            Self::Max => "Max",
        }
    }

    pub fn from_str(s: &str) -> Option<Self> {
        match s.to_ascii_lowercase().as_str() {
            "none" | "off" => Some(Self::None),
            "minimal" => Some(Self::Minimal),
            "low" => Some(Self::Low),
            "medium" | "med" => Some(Self::Medium),
            "high" => Some(Self::High),
            "xhigh" | "extra_high" | "extra-high" => Some(Self::XHigh),
            "max" => Some(Self::Max),
            _ => None,
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Model {
    pub id: String,
    pub provider: String,
    pub context_window: usize,
    pub supports_images: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub supported_thinking_levels: Option<Vec<ThinkingLevel>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub default_thinking_level: Option<ThinkingLevel>,
}

// Canonical wire type from the shared protobuf schema (proto/console/v1).
// prost field names match the old hand-written struct, so call sites are
// unchanged. serde impls come from pbjson (protojson naming).
pub use console_proto::ModelFavorite;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderCatalogEntry {
    pub name: String,
    pub display_name: String,
    pub description: String,
    /// A backend with an empty model list may emit `null`; that must decode
    /// to empty, never fail the whole catalog response.
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub models: Vec<Model>,
    pub auth_method: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderModelsResponse {
    pub provider: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub models: Vec<Model>,
}

/// `#[serde(default)]` only covers missing fields — explicit `null` needs a
/// custom deserializer to land on the default instead of erroring.
fn deserialize_null_default<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    let opt = Option::<T>::deserialize(deserializer)?;
    Ok(opt.unwrap_or_default())
}

/// The provider/model pair a session runs on. Selected in the UI picker and
/// persisted onto the session header, so it lives beside the catalog types
/// rather than inside the UI crate.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SelectedModel {
    pub provider: String,
    pub model_id: String,
}
