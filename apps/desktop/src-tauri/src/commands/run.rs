use tauri::{AppHandle, Emitter};

use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::RunPromptDto;

#[tauri::command]
pub async fn run_agent(
    app: AppHandle,
    session_id: String,
    prompt: String,
    model_id: Option<String>,
    provider: Option<String>,
    approval_mode: Option<String>,
) -> AppResult<()> {
    let client = ApiClient::new();
    let dto = RunPromptDto {
        prompt,
        model_id,
        provider,
        approval_mode,
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
