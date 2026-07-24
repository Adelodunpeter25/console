use futures_util::StreamExt;
use reqwest::Response;

use crate::api::ApiClient;
use crate::config::api_base;
use crate::error::{AppError, AppResult};
use crate::models::AgentSessionEvent;
use crate::models::RunPromptDto;

pub async fn run_agent_stream<F>(
    client: &ApiClient,
    session_id: &str,
    dto: &RunPromptDto,
    mut on_event: F,
) -> AppResult<()>
where
    F: FnMut(AgentSessionEvent) + Send + 'static,
{
    let url = format!("{}/sessions/{}/run", api_base(), session_id);
    let resp = client
        .raw_client()
        .post(&url)
        .json(dto)
        .send()
        .await?;

    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await?;
        return Err(AppError::Server(format!("HTTP {}: {}", status, text)));
    }

    parse_sse_stream(resp, &mut on_event).await
}

pub async fn abort_run(client: &ApiClient, session_id: &str) -> AppResult<serde_json::Value> {
    client
        .post(&format!("/sessions/{}/abort", session_id), &serde_json::json!({}))
        .await
}

async fn parse_sse_stream<F>(resp: Response, on_event: &mut F) -> AppResult<()>
where
    F: FnMut(AgentSessionEvent) + Send + 'static,
{
    let mut stream = resp.bytes_stream();
    let mut buffer = String::new();
    let mut data_lines: Vec<String> = Vec::new();

    while let Some(chunk_result) = stream.next().await {
        let chunk = chunk_result.map_err(|e| AppError::Sse(e.to_string()))?;
        buffer.push_str(&String::from_utf8_lossy(&chunk));

        while let Some(pos) = buffer.find('\n') {
            let line = buffer[..pos].to_string();
            buffer = buffer[pos + 1..].to_string();

            let trimmed = line.trim();

            if trimmed.is_empty() {
                if let Some(data) = data_lines.join("\n").strip_prefix("data: ") {
                    if let Ok(event) = serde_json::from_str::<AgentSessionEvent>(data) {
                        on_event(event);
                    }
                }
                data_lines.clear();
                continue;
            }

            if let Some(rest) = trimmed.strip_prefix("data: ") {
                data_lines.push(rest.to_string());
            }
        }
    }

    Ok(())
}
