//! Project script service: list, start, observe, and stop the run scripts
//! a project defines in its `console.toml`.
//!
//! Run requests reference scripts by id only. The command always comes from
//! the server-side parsed configuration — the UI never sends one.

use crate::types::{ApiResponse, ProjectScriptsResult, ScriptRun, ScriptRunEvent};
use crate::utils::{HttpTransport, SseStreamReader};
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct ProjectScriptsService {
    transport: HttpTransport,
}

impl ProjectScriptsService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    fn endpoint(project_id: &str, suffix: &str) -> String {
        format!(
            "/api/projects/{}/scripts{}",
            urlencoding::encode(project_id),
            suffix
        )
    }

    /// List the normalized script definitions for a project. A project
    /// without `console.toml` returns an empty list with `source: "missing"`.
    pub async fn list(&self, project_id: &str) -> Result<ProjectScriptsResult> {
        let url = self.transport.url(&Self::endpoint(project_id, "")).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch project scripts")?;

        let body: ApiResponse<ProjectScriptsResult> = response
            .json()
            .await
            .context("Failed to parse project scripts")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Project scripts response contained no data"))
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to fetch project scripts".into()
            })))
        }
    }

    /// Start a script by id. The server loads the command from its own
    /// parsed `console.toml`.
    pub async fn start_run(&self, project_id: &str, script_id: &str) -> Result<ScriptRun> {
        let url = self
            .transport
            .url(&Self::endpoint(
                project_id,
                &format!("/{}/runs", urlencoding::encode(script_id)),
            ))
            .await;
        let response = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to start project script")?;

        let body: ApiResponse<ScriptRun> = response
            .json()
            .await
            .context("Failed to parse started run")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Start run response contained no data"))
        } else {
            Err(anyhow!(body
                .error
                .unwrap_or_else(|| "Failed to start project script".into())))
        }
    }

    /// List runs the server retains for a project (including active ones,
    /// so a reopened desktop can reconnect).
    pub async fn list_runs(&self, project_id: &str) -> Result<Vec<ScriptRun>> {
        let url = self.transport.url(&Self::endpoint(project_id, "/runs")).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch project script runs")?;

        let body: ApiResponse<Vec<ScriptRun>> = response
            .json()
            .await
            .context("Failed to parse project script runs")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to fetch project script runs".into()
            })))
        }
    }

    /// Fetch one run record, including its retained stdout/stderr snapshot.
    pub async fn get_run(&self, project_id: &str, run_id: &str) -> Result<ScriptRun> {
        let url = self
            .transport
            .url(&Self::endpoint(
                project_id,
                &format!("/runs/{}", urlencoding::encode(run_id)),
            ))
            .await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch project script run")?;

        let body: ApiResponse<ScriptRun> = response
            .json()
            .await
            .context("Failed to parse project script run")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Run response contained no data"))
        } else {
            Err(anyhow!(body
                .error
                .unwrap_or_else(|| "Failed to fetch project script run".into())))
        }
    }

    /// Request the server stop a run. The final status still arrives as the
    /// terminal stream event; stopping here only asks for termination.
    pub async fn stop_run(&self, project_id: &str, run_id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&Self::endpoint(
                project_id,
                &format!("/runs/{}/stop", urlencoding::encode(run_id)),
            ))
            .await;
        let response = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to stop project script run")?;

        let body: ApiResponse<serde_json::Value> = response
            .json()
            .await
            .context("Failed to parse stop run response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(body
                .error
                .unwrap_or_else(|| "Failed to stop project script run".into())))
        }
    }

    /// Subscribe to live status/output/exit events for a run. The server
    /// terminates the stream after the final exit event; disconnecting here
    /// never stops the server-side process.
    pub async fn stream_run(
        &self,
        project_id: &str,
        run_id: &str,
    ) -> Result<std::pin::Pin<Box<dyn futures_util::Stream<Item = Result<ScriptRunEvent>> + Send>>>
    {
        let url = self
            .transport
            .url(&Self::endpoint(
                project_id,
                &format!("/runs/{}/stream", urlencoding::encode(run_id)),
            ))
            .await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to connect to project script run stream")?
            .error_for_status()
            .context("Project script run stream returned an error")?;

        Ok(Box::pin(SseStreamReader::parse_typed_stream(response)))
    }
}
