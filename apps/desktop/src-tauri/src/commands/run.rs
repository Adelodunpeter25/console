use tauri::{AppHandle, Emitter};

use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{ImageAttachment, RunPromptDto};

#[tauri::command]
pub async fn run_agent(
    app: AppHandle,
    session_id: String,
    prompt: String,
    model_id: Option<String>,
    provider: Option<String>,
    approval_mode: Option<String>,
    attachments: Option<Vec<ImageAttachment>>,
) -> AppResult<()> {
    let client = ApiClient::new();
    let dto = RunPromptDto {
        prompt,
        model_id,
        provider,
        approval_mode,
        attachments,
    };

    // Emit on a per-session channel so concurrent/switched sessions can't
    // cross-talk into the wrong conversation.
    let channel = format!("agent-event:{session_id}");

    crate::api::run::run_agent_stream(&client, &session_id, &dto, move |event| {
        let _ = app.emit(&channel, &event);
    })
    .await
}

#[tauri::command]
pub async fn abort_run(session_id: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::run::abort_run(&client, &session_id).await
}

#[tauri::command]
pub async fn answer_question(
    session_id: String,
    request_id: String,
    answer: serde_json::Value,
) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::run::answer_question(&client, &session_id, &request_id, &answer).await
}

#[tauri::command]
pub async fn approve_permission(
    session_id: String,
    request_id: String,
    allow: bool,
) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::run::approve_permission(&client, &session_id, &request_id, allow).await
}
