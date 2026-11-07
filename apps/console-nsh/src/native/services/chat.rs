//! Chat runs, through console-core's `RunService` and `SessionService`.
//!
//! The GPUI app consumed `AgentSessionEvent`s as a Rust stream
//! (`console_core::Transport::start_run`); the NSH app's TSX cannot hold a
//! stream, so each run here forwards its events through a JS callback
//! (`on_event`, called once per event with the event's camelCase JSON — the
//! same shape `AgentSessionEvent` serializes to, which the desktop's state
//! matches on). The returned promise settles when the run's stream ends (or
//! rejects with the transport error), so the caller's `await` is the run's
//! lifecycle: one `run_prompt`/`attach_run` per run, no polling loop.

use console_core::SessionService;
use console_core::services::{ChatEventStream, Transport};
use console_core::RunService;
use futures_util::StreamExt;
use nsh_core::element::Callback;
use nsh_core::JsonValue;

use crate::native::http;

/// Forwards `event` to the TSX callback as its serialized JSON. A failed
/// post (the app tore the callback down mid-run) ends the loop: nothing is
/// left to stream to.
async fn forward(
    on_event: &Callback<JsonValue, ()>,
    event: console_core::AgentSessionEvent,
) -> bool {
    match serde_json::to_value(&event) {
        Ok(json) => {
            on_event.post(json);
            true
        }
        Err(_) => true,
    }
}

async fn drain(
    mut stream: ChatEventStream,
    on_event: Callback<JsonValue, ()>,
) {
    while let Some(item) = stream.next().await {
        match item {
            Ok(event) => {
                if !forward(&on_event, event).await {
                    return;
                }
            }
            Err(err) => {
                eprintln!("console-nsh: run stream error: {err}");
                return;
            }
        }
    }
}

pub struct Chat;

impl Chat {
    /// Starts an agent run and forwards its events until the stream ends.
    pub async fn run_prompt(
        base_url: &str,
        session_id: &str,
        prompt: JsonValue,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        let dto: console_core::RunPromptDto =
            serde_json::from_value(prompt).map_err(|err| format!("bad prompt: {err}"))?;
        let client = console_core::ConsoleClient::new(Some(base_url.to_owned()));
        let stream = Transport::default()
            .start_run(&client, session_id, &dto)
            .await
            .map_err(|err| err.to_string())?;
        drain(stream, on_event).await;
        Ok(())
    }

    /// Reattaches to a run already in flight (an app restart mid-run), from
    /// `since` events in. The desktop's reattach does the same.
    pub async fn attach_run(
        base_url: &str,
        session_id: &str,
        since: Option<u64>,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        let client = console_core::ConsoleClient::new(Some(base_url.to_owned()));
        let stream = Transport::default()
            .attach_run(&client, session_id, since)
            .await
            .map_err(|err| err.to_string())?;
        drain(stream, on_event).await;
        Ok(())
    }

    /// One session's message history, as the server stores it (the TSX
    /// transcript state maps it into its own message shapes).
    pub async fn messages(base_url: &str, session_id: &str) -> Result<String, String> {
        let service = SessionService::new(http::transport(base_url));
        let detail = service
            .get(session_id, None)
            .await
            .map_err(|err| err.to_string())?;
        serde_json::to_string(&detail).map_err(|err| err.to_string())
    }

    /// Creates a session (the launcher's submit) and returns its header.
    pub async fn create_session(
        base_url: &str,
        project_id: &str,
        title: Option<String>,
    ) -> Result<String, String> {
        let service = SessionService::new(http::transport(base_url));
        let dto = console_core::CreateSessionDto {
            title,
            project_id: Some(project_id.to_string()),
            cwd: None,
            model_id: None,
            provider: None,
            approval_mode: None,
            thinking_level: None,
            worktree: None,
        };
        let header = service
            .create(dto)
            .await
            .map_err(|err| err.to_string())?;
        serde_json::to_string(&header).map_err(|err| err.to_string())
    }

    /// Answers a pending permission request (the card's Allow / Deny).
    pub async fn approve_permission(
        base_url: &str,
        session_id: &str,
        request_id: &str,
        allow: bool,
    ) -> Result<(), String> {
        let service = RunService::new(http::transport(base_url));
        service
            .approve_permission(
                session_id,
                console_core::ApproveToolPermissionDto {
                    request_id: request_id.to_string(),
                    allow,
                },
            )
            .await
            .map_err(|err| err.to_string())?;
        Ok(())
    }

    /// Answers a pending question (the card's option pick, custom answer, or
    /// skip). `answer` is the JSON value the server accepts: a string for a
    /// single pick, an array of strings for multi-select.
    pub async fn answer_question(
        base_url: &str,
        session_id: &str,
        request_id: &str,
        answer: JsonValue,
    ) -> Result<(), String> {
        let service = RunService::new(http::transport(base_url));
        service
            .answer_question(
                session_id,
                console_core::AnswerQuestionDto {
                    request_id: request_id.to_string(),
                    answer,
                },
            )
            .await
            .map_err(|err| err.to_string())?;
        Ok(())
    }
}
