//! Streaming-surface plumbing: every chat/watch/terminal stream goes through
//! here. It used to switch between SSE/WS and WebTransport (QUIC);
//! WebTransport is removed, so each method now delegates to its SSE/WS
//! service directly.

use crate::ConsoleClient;
use crate::services::terminal::{TerminalHandle, TerminalService};
use crate::types::events::AgentSessionEvent;
use crate::types::terminal::{TerminalSize, TerminalSpawnParams};
use crate::types::{ForwardedPort, FsChangeEvent, GitStatusSummary, RunPromptDto};
use anyhow::Result;
use futures_util::Stream;
use std::pin::Pin;

/// Chat run events.
pub type ChatEventStream = Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>;

/// Shared streaming plumbing. Cloning is a refcount bump; it carries no
/// connection state — every stream is its own HTTP connection.
#[derive(Clone, Copy, Debug, Default)]
pub struct Transport;

impl Transport {
    /// Start an agent run.
    pub async fn start_run(
        &self,
        client: &ConsoleClient,
        session_id: &str,
        prompt: &RunPromptDto,
    ) -> Result<ChatEventStream> {
        Ok(Box::pin(
            client
                .runs
                .stream_prompt(session_id, prompt.clone())
                .await?,
        ))
    }

    /// Attach to a run in flight.
    pub async fn attach_run(
        &self,
        client: &ConsoleClient,
        session_id: &str,
        since: Option<u64>,
    ) -> Result<ChatEventStream> {
        Ok(Box::pin(
            client.runs.attach_run_stream(session_id, since).await?,
        ))
    }

    pub async fn watch_fs(
        &self,
        client: &ConsoleClient,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<FsChangeEvent>> + Send>>> {
        client.fs.watch_events(path).await
    }

    pub async fn watch_git_status(
        &self,
        client: &ConsoleClient,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<GitStatusSummary>> + Send>>> {
        client.git.watch_status(path).await
    }

    pub async fn watch_ports(
        &self,
        client: &ConsoleClient,
        project_id: Option<&str>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<Vec<ForwardedPort>>> + Send>>> {
        client.ports.watch(project_id).await
    }

    pub async fn watch_notifications(
        &self,
        client: &ConsoleClient,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<crate::types::NotificationEvent>> + Send>>> {
        client.notifications.stream().await
    }

    pub async fn watch_script_run(
        &self,
        client: &ConsoleClient,
        project_id: &str,
        run_id: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<crate::types::ScriptRunEvent>> + Send>>> {
        client.scripts.stream_run(project_id, run_id).await
    }

    /// Spawn a terminal.
    pub async fn spawn_terminal(
        &self,
        client: &ConsoleClient,
        params: TerminalSpawnParams,
        initial_size: TerminalSize,
    ) -> Result<TerminalHandle> {
        TerminalService::new(client.transport().clone())
            .spawn(params, initial_size)
            .await
    }
}
