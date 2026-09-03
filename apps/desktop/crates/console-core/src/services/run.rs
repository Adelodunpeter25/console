use crate::types::*;
use crate::utils::{HttpTransport, SseStreamReader};
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct RunService {
    transport: HttpTransport,
}

impl RunService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn abort(&self, session_id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/abort", session_id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to abort run")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse abort response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to abort run".into())
            ))
        }
    }

    pub async fn answer_question(
        &self,
        session_id: &str,
        payload: AnswerQuestionDto,
    ) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/answer", session_id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to answer question")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse answer response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to answer question".into())
            ))
        }
    }

    pub async fn approve_permission(
        &self,
        session_id: &str,
        payload: ApproveToolPermissionDto,
    ) -> Result<bool> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/approve", session_id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to approve tool permission")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse permission response")?;
        if body.success {
            Ok(payload.allow)
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to approve permission".into())
            ))
        }
    }

    pub async fn stream_prompt(
        &self,
        session_id: &str,
        payload: RunPromptDto,
    ) -> Result<impl futures_util::Stream<Item = Result<AgentSessionEvent>>> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/run", session_id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to initiate agent run stream")?;

        if !resp.status().is_success() {
            let err_text = resp.text().await.unwrap_or_default();
            return Err(anyhow!("Run failed with status: {}", err_text));
        }

        Ok(SseStreamReader::parse_stream(resp))
    }

    pub async fn attach_run_stream(
        &self,
        session_id: &str,
        since: Option<u64>,
    ) -> Result<impl futures_util::Stream<Item = Result<AgentSessionEvent>>> {
        let path = if let Some(seq) = since {
            format!("/api/sessions/{}/run/stream?since={}", session_id, seq)
        } else {
            format!("/api/sessions/{}/run/stream", session_id)
        };
        let url = self.transport.url(&path).await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to attach to agent run stream")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let err_text = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "Attach stream failed with status {}: {}",
                status,
                err_text
            ));
        }

        Ok(SseStreamReader::parse_stream(resp))
    }
}
