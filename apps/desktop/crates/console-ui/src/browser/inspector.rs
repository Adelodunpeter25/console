//! Browser inspector data types and script utilities.

use serde::{Deserialize, Serialize};

pub const INSPECTOR_SCRIPT: &str = include_str!("inspector.js");

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct InspectRect {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BrowserElementInspection {
    pub component_name: Option<String>,
    pub source_location: Option<String>,
    pub selector: String,
    pub html_snippet: String,
    pub url: String,
    pub title: String,
    pub bounds: InspectRect,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum InspectorIpcMessage {
    #[serde(rename = "element_inspected")]
    ElementInspected(BrowserElementInspection),
    #[serde(rename = "inspect_cancelled")]
    InspectCancelled,
}
