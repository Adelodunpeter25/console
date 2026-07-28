use serde::{Deserialize, Serialize};

use super::session::{SessionDetailResponse, SessionHeader};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApiResponse<T> {
    pub success: bool,
    pub data: Option<T>,
    pub error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateSessionDto {
    pub cwd: String,
    pub project_id: Option<String>,
    pub model_id: Option<String>,
    pub provider: Option<String>,
    pub title: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSessionDto {
    pub title: Option<String>,
    pub model_id: Option<String>,
    pub provider: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RunPromptDto {
    pub prompt: String,
    pub model_id: Option<String>,
    pub provider: Option<String>,
    pub approval_mode: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OAuthLoginUrlDto {
    pub provider: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OAuthCallbackDto {
    pub provider: String,
    pub code: String,
    pub state: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AnswerQuestionDto {
    pub request_id: String,
    pub answer: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApproveToolPermissionDto {
    pub request_id: String,
    pub allow: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProjectInfo {
    pub id: Option<String>,
    pub name: String,
    pub path: String,
    pub created_at: Option<f64>,
    pub updated_at: Option<f64>,
    pub last_modified: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthStatusResponse {
    pub gemini: ProviderAuthStatus,
    pub antigravity: ProviderAuthStatus,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderAuthStatus {
    pub logged_in: bool,
    pub email: Option<String>,
    pub project_id: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FsTreeEntry {
    pub name: String,
    pub path: String,
    pub is_dir: bool,
    pub size: Option<u64>,
    pub children: Option<Vec<FsTreeEntry>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BrowseResult {
    pub path: String,
    pub parent_path: Option<String>,
    pub entries: Vec<FsTreeEntry>,
}

pub type SessionListResponse = ApiResponse<Vec<SessionHeader>>;
pub type SessionDetailApiResponse = ApiResponse<SessionDetailResponse>;
pub type ProjectListResponse = ApiResponse<Vec<ProjectInfo>>;
pub type AuthStatusApiResponse = ApiResponse<AuthStatusResponse>;
