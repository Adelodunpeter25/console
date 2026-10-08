use serde::{Deserialize, Serialize};

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

/// Canonical wire types from the shared protobuf schema
/// (proto/console/v1/catalog.proto). context_window narrows to u32 and
/// stays a JSON number; thinking levels stay plain strings on the wire, so
/// [`Model::supported_thinking_levels`] and
/// [`Model::default_thinking_level`] convert them to the client-side
/// [`ThinkingLevel`] enum, skipping unknown strings.
pub use console_proto::{Model, ProviderCatalogEntry, ProviderModelsResponse};

/// Thinking levels a catalog model supports, as client-side enums.
/// Unknown/absent levels are skipped; an empty list means "no declared
/// levels", which callers fall back to per-provider defaults for.
pub fn model_thinking_levels(model: &Model) -> Vec<ThinkingLevel> {
    model
        .supported_thinking_levels
        .iter()
        .filter_map(|raw| ThinkingLevel::from_str(raw))
        .collect()
}

/// The model's default thinking level, if it names one the client knows.
pub fn model_default_thinking_level(model: &Model) -> Option<ThinkingLevel> {
    model
        .default_thinking_level
        .as_deref()
        .and_then(ThinkingLevel::from_str)
}

// Canonical wire type from the shared protobuf schema (proto/console/v1).
// prost field names match the old hand-written struct, so call sites are
// unchanged. serde impls come from pbjson (protojson naming).
pub use console_proto::ModelFavorite;

/// The provider/model pair a session runs on. Selected in the UI picker and
/// persisted onto the session header, so it lives beside the catalog types
/// rather than inside the UI crate.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SelectedModel {
    pub provider: String,
    pub model_id: String,
}
