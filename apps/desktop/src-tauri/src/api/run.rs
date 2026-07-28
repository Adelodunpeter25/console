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

/// Decode a complete SSE event from accumulated `data:` lines and forward it.
/// Each `data:` line already had its `data: ` prefix stripped before being
/// pushed, so the joined buffer is raw JSON — no further prefix stripping.
fn flush_event<F: FnMut(AgentSessionEvent)>(
    data_lines: &mut Vec<String>,
    on_event: &mut F,
) {
    if data_lines.is_empty() {
        return;
    }
    let data = data_lines.join("\n");
    data_lines.clear();
    match serde_json::from_str::<AgentSessionEvent>(&data) {
        Ok(event) => on_event(event),
        Err(err) => {
            // Surface parse failures instead of silently dropping the frame —
            // a camelCase/schema mismatch used to make the UI look "stuck"
            // with only the optimistic user message.
            eprintln!("agent SSE deserialize error: {err}; payload={}", &data[..data.len().min(500)]);
            on_event(AgentSessionEvent::Error {
                error: crate::models::AgentSessionError {
                    message: format!("Failed to parse agent event: {err}"),
                    data: Some(serde_json::json!({ "raw": data })),
                },
            });
        }
    }
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

            // A blank line terminates the current event frame.
            if trimmed.is_empty() {
                flush_event(&mut data_lines, on_event);
                continue;
            }

            // Accept both "data: {...}" and "data:{...}" (no space).
            if let Some(rest) = trimmed.strip_prefix("data:") {
                data_lines.push(rest.trim_start().to_string());
            }
        }
    }

    // Flush any trailing event that wasn't followed by a blank line.
    flush_event(&mut data_lines, on_event);

    Ok(())
}
