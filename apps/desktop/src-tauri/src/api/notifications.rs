use futures_util::StreamExt;
use reqwest::Response;

use crate::api::ApiClient;
use crate::config::api_base;
use crate::error::{AppError, AppResult};
use crate::models::NotificationEvent;

/// Long-lived SSE subscription to the backend notification stream.
/// Re-emits each `notification` frame through `on_event` (which the caller
/// uses to show a native OS notification).
pub async fn stream_notifications<F>(client: &ApiClient, mut on_event: F) -> AppResult<()>
where
    F: FnMut(NotificationEvent) + Send + 'static,
{
    let url = format!("{}/notifications/stream", api_base());
    let resp = client
        .raw_client()
        .get(&url)
        .send()
        .await?;

    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await?;
        return Err(AppError::Server(format!("HTTP {}: {}", status, text)));
    }

    parse_sse_stream(resp, &mut on_event).await
}

/// Decode a complete SSE frame from accumulated `data:` lines and forward it.
fn flush_event<F: FnMut(NotificationEvent)>(data_lines: &mut Vec<String>, on_event: &mut F) {
    if data_lines.is_empty() {
        return;
    }
    let data = data_lines.join("\n");
    data_lines.clear();
    // Heartbeat "ping" frames carry no data (`data:` with empty value) — skip.
    if data.trim().is_empty() {
        return;
    }
    match serde_json::from_str::<NotificationEvent>(&data) {
        Ok(event) => on_event(event),
        Err(err) => {
            // Anything that fails to parse is a schema mismatch worth surfacing.
            eprintln!(
                "notification SSE deserialize error: {err}; payload={}",
                &data[..data.len().min(500)]
            );
        }
    }
}

async fn parse_sse_stream<F>(resp: Response, on_event: &mut F) -> AppResult<()>
where
    F: FnMut(NotificationEvent) + Send + 'static,
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
