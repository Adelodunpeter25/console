use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelRoleMapping {
    #[serde(default)]
    pub default: Option<String>,
    #[serde(default)]
    pub plan: Option<String>,
    #[serde(default)]
    pub vision: Option<String>,
    #[serde(default)]
    pub smol: Option<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConsoleSettings {
    #[serde(default)]
    pub model_roles: ModelRoleMapping,
}
