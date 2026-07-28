use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ApprovalModeOption;

#[tauri::command]
pub async fn get_approval_modes() -> AppResult<Vec<ApprovalModeOption>> {
    let client = ApiClient::new();
    crate::api::config::get_approval_modes(&client).await
}
