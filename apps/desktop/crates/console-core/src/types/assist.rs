use serde::{Deserialize, Serialize};

use console_proto::FileSearchResult;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SlashCommandInfo {
    pub name: String,
    pub description: String,
    pub builtin: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileSearchResponse {
    pub root: String,
    pub query: String,
    pub items: Vec<FileSearchResult>,
}
