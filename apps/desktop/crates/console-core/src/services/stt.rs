use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct SttService {
    transport: HttpTransport,
}

impl SttService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// `GET /api/stt/status` — whether speech-to-text is configured on this
    /// server, and which account it would spend against. The status carries
    /// no token; there is nothing here to accidentally log or cache.
    pub async fn status(&self) -> Result<SttStatus> {
        let url = self.transport.url("/api/stt/status").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch speech-to-text status")?;

        let body: ApiResponse<SttStatus> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse speech-to-text status response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch speech-to-text status".into())
            ))
        }
    }

    /// `PUT /api/stt/config` — store the Cloudflare account pair on the
    /// server. Both halves are required every time: the token is never
    /// returned, so an account-only update could not preserve it.
    ///
    /// The pair is written straight into the request body and never retained
    /// in app state — the caller drops it as soon as this returns.
    pub async fn set_config(
        &self,
        account_id: &str,
        api_token: &str,
    ) -> Result<SetSttConfigResponse> {
        let url = self.transport.url("/api/stt/config").await;
        let resp = self
            .transport
            .client()
            .put(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({
                "accountId": account_id,
                "apiToken": api_token,
            }))
            .send()
            .await
            .context("Failed to save speech-to-text config")?;

        let body: ApiResponse<SetSttConfigResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse set speech-to-text config response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to save speech-to-text config".into())
            ))
        }
    }

    /// `DELETE /api/stt/config` — remove the stored pair. Idempotent. Cannot
    /// remove a pair supplied by the server's environment, which is why the
    /// status payload carries a `source`: the UI hides the remove action
    /// rather than offering one that silently does nothing.
    pub async fn clear_config(&self) -> Result<SetSttConfigResponse> {
        let url = self.transport.url("/api/stt/config").await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to remove speech-to-text config")?;

        let body: ApiResponse<SetSttConfigResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse clear speech-to-text config response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to remove speech-to-text config".into())
            ))
        }
    }

    /// `POST /api/stt/transcribe` — transcribe one recorded WAV clip. The
    /// audio lives only for this request: it is sent once and dropped, and
    /// nothing here retains it.
    pub async fn transcribe(&self, wav: &[u8]) -> Result<SttTranscript> {
        let url = self.transport.url("/api/stt/transcribe").await;
        let part = reqwest::multipart::Part::bytes(wav.to_vec())
            .file_name("dictation.wav")
            .mime_str("audio/wav")
            .context("Unsupported audio type")?;
        let form = reqwest::multipart::Form::new().part("audio", part);
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .multipart(form)
            .send()
            .await
            .context("Failed to transcribe recording")?;

        let body: ApiResponse<SttTranscript> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse transcription response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Transcription failed".into())
            ))
        }
    }
}
