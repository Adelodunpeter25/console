//! Render models for workspace image visuals (`read_file`).

use serde::Deserialize;

use crate::types::agent::VisualArtifact;

pub const SUPPORTED_VISUAL_VERSION: u32 = 1;

#[derive(Clone, Debug, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct VisualSpec {
    pub version: u32,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub subtitle: Option<String>,
    #[serde(default)]
    pub blocks: Vec<VisualBlock>,
}

#[derive(Clone, Debug, Deserialize, PartialEq)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum VisualBlock {
    Image {
        path: String,
        #[serde(default)]
        alt: Option<String>,
    },
}

#[derive(Clone, Debug, PartialEq)]
pub enum VisualParse {
    Ok(VisualSpec),
    Unsupported,
    Invalid,
}

pub fn parse_visual(artifact: &VisualArtifact) -> VisualParse {
    if artifact.version != SUPPORTED_VISUAL_VERSION {
        return VisualParse::Unsupported;
    }
    match serde_json::from_value::<VisualSpec>(artifact.spec.clone()) {
        Ok(spec) if spec.version == SUPPORTED_VISUAL_VERSION => VisualParse::Ok(spec),
        Ok(_) => VisualParse::Unsupported,
        Err(_) => VisualParse::Invalid,
    }
}
