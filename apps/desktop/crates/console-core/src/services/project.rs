use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use serde_json::{Value, json};
use std::fmt;

/// Request body for `POST /api/projects/init`.
///
/// `parentDir` is camelCase because the server reads these bodies with
/// protojson (proto/console/v1/project.proto), which is not serde's default.
/// The server owns the target directory: it joins parent + name, makes the
/// folder, and registers whatever it created — so the client never sends a
/// full path and cannot register a directory it did not cause.
pub fn init_project_body(name: &str, parent_dir: &str) -> Value {
    json!({ "name": name, "parentDir": parent_dir })
}

/// Request body for `POST /api/projects/clone`.
///
/// `name` is omitted rather than sent as null when the caller has none: the
/// server derives the folder from the URL's last path segment in that case.
/// The URL may be HTTPS or SSH-style (`git@github.com:org/repo`) — the server
/// rewrites the latter to HTTPS through its injected git config.
pub fn clone_project_body(url: &str, parent_dir: &str, name: Option<&str>) -> Value {
    let mut body = json!({ "url": url, "parentDir": parent_dir });
    if let Some(name) = name.filter(|name| !name.trim().is_empty()) {
        body["name"] = json!(name);
    }
    body
}

/// Error marker for the server's 409 reply.
///
/// A clone of a private repo with no stored token answers 409 rather than 400
/// precisely so the dialog can point the user at Accounts. A 400 would read
/// as "you typed the URL wrong" and send them to edit the wrong field, so
/// this case must stay distinguishable from an ordinary validation failure.
#[derive(Debug)]
pub struct GithubNotConnected {
    /// Server-authored wording, already written to be shown verbatim.
    pub message: String,
}

impl fmt::Display for GithubNotConnected {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.message)
    }
}

impl std::error::Error for GithubNotConnected {}

/// Whether an error from [`ProjectService::clone_project`] means "GitHub is
/// not connected" — the signal for the dialog to offer the Accounts handoff
/// instead of showing a plain failure.
pub fn is_github_not_connected(err: &anyhow::Error) -> bool {
    err.downcast_ref::<GithubNotConnected>().is_some()
}

/// Map a failed bootstrap response to an error, tagging the 409 case so the
/// caller can branch on it. Everything else keeps the server's message, which
/// is written to be displayed as-is.
fn bootstrap_error(status: reqwest::StatusCode, message: Option<String>) -> anyhow::Error {
    let message = message.unwrap_or_else(|| format!("Request failed ({status})"));
    if status == reqwest::StatusCode::CONFLICT {
        return anyhow::Error::new(GithubNotConnected {
            message: message.clone(),
        });
    }
    anyhow!(message)
}

#[derive(Clone)]
pub struct ProjectService {
    transport: HttpTransport,
}

impl ProjectService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self) -> Result<Vec<ProjectInfo>> {
        let url = self.transport.url("/api/projects").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list projects")?;

        let body: ApiResponse<Vec<ProjectInfo>> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse projects response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list projects".into())
            ))
        }
    }

    pub async fn add(&self, path: &str) -> Result<ProjectInfo> {
        let url = self.transport.url("/api/projects").await;
        let payload = serde_json::json!({ "path": path });

        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to add project")?;

        let body: ApiResponse<ProjectInfo> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse add project response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Project data is missing"))
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to add project".into())
            ))
        }
    }

    /// `POST /api/projects/init` — create a blank project server-side.
    ///
    /// The server makes `<parent_dir>/<name>`, runs `git init`, and registers
    /// the result, then returns it in the same shape as [`Self::add`]. Use
    /// this when the folder does not exist yet; `add` is for registering a
    /// path that is already on disk.
    pub async fn init_blank(&self, name: &str, parent_dir: &str) -> Result<ProjectInfo> {
        let url = self.transport.url("/api/projects/init").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&init_project_body(name, parent_dir))
            .send()
            .await
            .context("Failed to create project")?;

        let status = resp.status();
        let body: ApiResponse<ProjectInfo> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse create project response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Project data is missing"))
        } else {
            Err(bootstrap_error(status, body.error))
        }
    }

    /// `POST /api/projects/clone` — clone a remote repo and register it.
    ///
    /// `name` is optional; omit it to let the server name the folder after the
    /// repo. A public repo clones with no credentials. A private repo with no
    /// stored token fails with a [`GithubNotConnected`] error — check it with
    /// [`is_github_not_connected`].
    ///
    /// The call is synchronous server-side: this future stays pending for as
    /// long as the clone runs, which is what the dialog's busy state waits on.
    /// The shared client sets no request timeout, so a slow clone is not cut
    /// short.
    pub async fn clone_project(
        &self,
        url: &str,
        parent_dir: &str,
        name: Option<&str>,
    ) -> Result<ProjectInfo> {
        let endpoint = self.transport.url("/api/projects/clone").await;
        let resp = self
            .transport
            .client()
            .post(&endpoint)
            .headers(self.transport.build_headers().await)
            .json(&clone_project_body(url, parent_dir, name))
            .send()
            .await
            .context("Failed to clone repository")?;

        let status = resp.status();
        let body: ApiResponse<ProjectInfo> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse clone project response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Project data is missing"))
        } else {
            Err(bootstrap_error(status, body.error))
        }
    }

    /// `DELETE /api/projects/:id` — remove a project workspace by id.
    pub async fn remove(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/projects/{}", id)).await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to remove project")?;

        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("Failed to parse remove project response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to remove project".into())
            ))
        }
    }
}
