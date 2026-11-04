//! Synced per-chat composer drafts (`/api/drafts/:sessionId`).

use anyhow::{Context, Result, anyhow};
use console_proto::{ChatDraft, PutDraftResponse};
use serde::Deserialize;

use crate::types::ApiResponse;
use crate::utils::HttpTransport;

#[derive(Clone)]
pub struct DraftService {
    transport: HttpTransport,
}

#[derive(Deserialize)]
struct GetDraftData {
    draft: Option<ChatDraft>,
}

impl DraftService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// The chat's stored draft, or `None` when it has never had one.
    pub async fn get(&self, session_id: &str) -> Result<Option<ChatDraft>> {
        let url = self
            .transport
            .url(&format!("/api/drafts/{}", encode_segment(session_id)))
            .await;
        let response = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to load draft")?;
        let body: ApiResponse<GetDraftData> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse draft response")?;
        if body.success {
            Ok(body.data.and_then(|data| data.draft))
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| "Failed to load draft".into())))
        }
    }

    /// Store `draft`. `applied == false` means the server held a newer one,
    /// returned in `draft` for the caller to adopt.
    pub async fn put(&self, draft: &ChatDraft) -> Result<PutDraftResponse> {
        let url = self
            .transport
            .url(&format!("/api/drafts/{}", encode_segment(&draft.session_id)))
            .await;
        let response = self
            .transport
            .client()
            .put(&url)
            .headers(self.transport.build_headers().await)
            .json(draft)
            .send()
            .await
            .context("Failed to save draft")?;
        let body: ApiResponse<PutDraftResponse> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse draft save response")?;
        match (body.success, body.data) {
            (true, Some(data)) => Ok(data),
            (_, _) => Err(anyhow!(body.error.unwrap_or_else(|| "Failed to save draft".into()))),
        }
    }
}

/// Session ids are generated tokens, but the path segment is still escaped.
fn encode_segment(value: &str) -> String {
    value
        .bytes()
        .map(|byte| match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                (byte as char).to_string()
            }
            _ => format!("%{byte:02X}"),
        })
        .collect()
}
