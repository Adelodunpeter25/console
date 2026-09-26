use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use std::time::Duration;


#[derive(Clone)]
pub struct SessionService {
    transport: HttpTransport,
}

impl SessionService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(
        &self,
        cwd: Option<&str>,
        project_id: Option<&str>,
    ) -> Result<Vec<SessionHeader>> {
        let mut url = self.transport.url("/api/sessions").await;
        let mut query = Vec::new();
        if let Some(c) = cwd {
            query.push(format!("cwd={}", urlencoding::encode(c)));
        }
        if let Some(p) = project_id {
            query.push(format!("projectId={}", urlencoding::encode(p)));
        }
        if !query.is_empty() {
            url.push('?');
            url.push_str(&query.join("&"));
        }

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list sessions")?;

        let body: ApiResponse<Vec<SessionHeader>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse sessions response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list sessions".into())
            ))
        }
    }

    /// Fetch a session header plus a page of message history.
    ///
    /// `options.limit` defaults to 50; `options.before` paginates older
    /// batches. The returned `next_cursor` is the rowid to pass as `before`
    /// to fetch the next older page; `has_more` is false when the start of
    /// the session's history has been reached.
    pub async fn get(
        &self,
        id: &str,
        options: Option<SessionPageOptions>,
    ) -> Result<SessionDetailResponse> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let mut req = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await);
        if let Some(opts) = options {
            if let Some(limit) = opts.limit {
                req = req.query(&[("limit", limit)]);
            }
            if let Some(before) = opts.before {
                req = req.query(&[("before", before)]);
            }
        }
        let resp = req.send().await.context("Failed to get session")?;

        let body: ApiResponse<SessionDetailResponse> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse session response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Session data is missing"))
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to get session".into())
            ))
        }
    }

    pub async fn get_paginated(
        &self,
        id: &str,
        limit: Option<usize>,
        before: Option<i64>,
    ) -> Result<SessionDetailResponse> {
        self.get(
            id,
            Some(SessionPageOptions {
                limit: limit.and_then(|l| u32::try_from(l).ok()),
                before,
            }),
        )
        .await
    }

    /// `GET /api/sessions/:id/changes` — session file changes. When
    /// `turn_index` is `None`, returns all turns aggregated; otherwise
    /// scopes to that single turn.
    pub async fn get_changes(
        &self,
        id: &str,
        turn_index: Option<u64>,
    ) -> Result<Vec<SessionFileChange>> {
        let mut url = self
            .transport
            .url(&format!("/api/sessions/{}/changes", id))
            .await;
        if let Some(turn) = turn_index {
            url.push_str(&format!("?turnIndex={}", turn));
        }
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get session changes")?;

        let body: ApiResponse<Vec<SessionFileChange>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse session changes response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Session changes data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to load session changes".into())
            ))
        }
    }

    /// `GET /api/sessions/:id/changes/diff` — cached unified diff text for a
    /// single file change, scoped to `(path, turn_index)`. Avoids any git
    /// subprocess call; the diff is served straight from the DB.
    pub async fn get_change_diff(
        &self,
        id: &str,
        path: &str,
        turn_index: u64,
    ) -> Result<String> {
        let url = self
            .transport
            .url(&format!(
                "/api/sessions/{}/changes/diff?path={}&turnIndex={}",
                id,
                urlencoding::encode(path),
                turn_index
            ))
            .await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get session change diff")?;

        #[derive(serde::Deserialize)]
        struct DiffPayload {
            #[serde(rename = "diffText")]
            diff_text: String,
        }

        let body: ApiResponse<DiffPayload> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse session change diff response")?;
        if body.success {
            body.data
                .map(|d| d.diff_text)
                .ok_or_else(|| anyhow!("Session change diff data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to load session change diff".into())
            ))
        }
    }

    /// `POST /api/sessions/:id/changes/reviewed` — mark or unmark a single
    /// file change as reviewed, scoped to `(path, turn_index)`.
    pub async fn set_change_reviewed(
        &self,
        id: &str,
        path: &str,
        turn_index: u64,
        reviewed: bool,
    ) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/changes/reviewed", id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&serde_json::json!({
                "path": path,
                "turnIndex": turn_index,
                "reviewed": reviewed,
            }))
            .send()
            .await
            .context("Failed to set session change reviewed state")?;

        if resp.status().is_success() {
            return Ok(());
        }

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse set change reviewed response")?;
        Err(anyhow!(
            body.error
                .unwrap_or_else(|| "Failed to set change reviewed state".into())
        ))
    }

    /// `GET /api/sessions/:id/todos` — persisted todo checklist for a session.
    pub async fn get_todos(&self, id: &str) -> Result<Vec<TodoItem>> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/todos", id))
            .await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get session todos")?;

        let body: ApiResponse<Vec<TodoItem>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse session todos response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to load session todos".into())
            ))
        }
    }

    /// Reload a session until the backend has finished persisting an active run.
    /// This is used after an SSE disconnect so the UI does not immediately
    /// submit another prompt against a still-running server session.
    pub async fn wait_until_settled(&self, id: &str) -> Result<SessionDetailResponse> {
        const MAX_ATTEMPTS: usize = 120;
        const POLL_INTERVAL: Duration = Duration::from_millis(250);

        for attempt in 0..MAX_ATTEMPTS {
            let detail = self.get(id, None).await?;
            if detail.header.status != Some(SessionStatus::Working) || attempt + 1 == MAX_ATTEMPTS {
                return Ok(detail);
            }
            tokio::time::sleep(POLL_INTERVAL).await;
        }

        unreachable!("session settlement loop always returns");
    }

    pub async fn create(&self, payload: CreateSessionDto) -> Result<SessionHeader> {
        let url = self.transport.url("/api/sessions").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to create session")?;

        let body: ApiResponse<SessionHeader> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse create session response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Created session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to create session".into())
            ))
        }
    }

    /// `POST /api/sessions/:id/worktree` — convert an existing, message-less
    /// session in place into a worktree session (branch off its current cwd,
    /// re-point cwd at the new worktree). The session id never changes;
    /// unlike `create` with a worktree spec, this never creates a new row.
    pub async fn attach_worktree(
        &self,
        id: &str,
        spec: CreateWorktreeSpec,
    ) -> Result<SessionHeader> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/worktree", id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&spec)
            .send()
            .await
            .context("Failed to attach worktree")?;

        let body: ApiResponse<SessionHeader> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse attach worktree response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Attached worktree session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to attach worktree".into())
            ))
        }
    }

    pub async fn update(&self, id: &str, payload: UpdateSessionDto) -> Result<SessionHeader> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let resp = self
            .transport
            .client()
            .patch(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to update session")?;

        let body: ApiResponse<SessionHeader> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse update session response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Updated session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to update session".into())
            ))
        }
    }

    pub async fn delete(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to delete session")?;

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse delete session response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to delete session".into())
            ))
        }
    }

    /// `GET /api/sessions?onlyDeleted=true` — soft-deleted sessions (trash).
    pub async fn list_deleted(&self, project_id: Option<&str>) -> Result<Vec<SessionHeader>> {
        let mut url = self.transport.url("/api/sessions").await;
        let mut query = vec!["onlyDeleted=true".to_string()];
        if let Some(p) = project_id {
            query.push(format!("projectId={}", urlencoding::encode(p)));
        }
        url.push('?');
        url.push_str(&query.join("&"));

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list deleted sessions")?;

        let body: ApiResponse<Vec<SessionHeader>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse deleted sessions response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to list deleted sessions".into()
            })))
        }
    }

    /// `POST /api/sessions/:id/restore` — bring a soft-deleted chat back.
    pub async fn restore(&self, id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/restore", id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to restore session")?;

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse restore session response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to restore session".into())
            ))
        }
    }

    /// `DELETE /api/sessions/:id/permanent` — irreversibly delete a
    /// soft-deleted chat and its messages.
    pub async fn permanent_delete(&self, id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/sessions/{}/permanent", id))
            .await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to permanently delete session")?;

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse permanent delete response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to permanently delete session".into()
            })))
        }
    }
}
