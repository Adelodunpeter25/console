use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ApprovalModeOption;

pub async fn get_approval_modes(client: &ApiClient) -> AppResult<Vec<ApprovalModeOption>> {
    client.get("/config/approval-modes").await
}
