//! Durable file/diff comments (`/api/projects/:id/comments`, `/api/comments/:id`).

use anyhow::{Context, Result, anyhow};
use console_proto::{CreateFileCommentRequest, FileComment, UpdateFileCommentRequest};
use serde::de::DeserializeOwned;

use crate::types::ApiResponse;
use crate::utils::HttpTransport;

#[derive(Clone)]
pub struct FileCommentService {
    transport: HttpTransport,
}

impl FileCommentService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self, project_id: &str) -> Result<Vec<FileComment>> {
        let url = self
            .transport
            .url(&format!(
                "/api/projects/{}/comments",
                encode_segment(project_id)
            ))
            .await;
        let response = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to load comments")?;
        self.decode(response, "Failed to load comments").await
    }

    pub async fn create(
        &self,
        project_id: &str,
        request: &CreateFileCommentRequest,
    ) -> Result<FileComment> {
        let url = self
            .transport
            .url(&format!(
                "/api/projects/{}/comments",
                encode_segment(project_id)
            ))
            .await;
        let response = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(request)
            .send()
            .await
            .context("Failed to save comment")?;
        self.decode(response, "Failed to save comment").await
    }

    pub async fn update_text(&self, id: &str, text: &str) -> Result<FileComment> {
        let url = self
            .transport
            .url(&format!("/api/comments/{}", encode_segment(id)))
            .await;
        let response = self
            .transport
            .client()
            .patch(&url)
            .headers(self.transport.build_headers().await)
            .json(&UpdateFileCommentRequest {
                text: text.to_string(),
            })
            .send()
            .await
            .context("Failed to update comment")?;
        self.decode(response, "Failed to update comment").await
    }

    pub async fn delete(&self, id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/comments/{}", encode_segment(id)))
            .await;
        let response = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to delete comment")?;
        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse comment response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to delete comment".into())
            ))
        }
    }

    async fn decode<T: DeserializeOwned + Send + 'static>(
        &self,
        response: reqwest::Response,
        fallback: &str,
    ) -> Result<T> {
        let body: ApiResponse<T> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse comment response")?;
        match (body.success, body.data) {
            (true, Some(data)) => Ok(data),
            _ => Err(anyhow!(body.error.unwrap_or_else(|| fallback.into()))),
        }
    }
}

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
