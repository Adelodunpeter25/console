//! The app's native boundary, split the way the GPUI app splits it: `http/`
//! is the transport wiring, `services/` one module per console-core service,
//! `types.rs` the plain structs that cross into TSX.

pub mod http;
pub mod services;
pub mod types;

use nsh_core::prelude::*;
use nsh_core::JsonValue;

use services::auth;
use services::chat::Chat;
use services::workspace::WorkspacePersist;
use services::{catalog, fs_git};
use services::{projects::Projects, sessions::Sessions};
use types::{ProjectSummary, SessionSummary};

/// The Console backend, reached through `console-core`'s services. Every
/// function takes the backend's base URL; the desktop app's server discovery
/// replaces that when its state moves over.
#[module]
pub struct Console;

#[module]
impl Console {
    /// Fetches `{base_url}/health` through console-core and returns the
    /// response body; rejects when the server is unreachable.
    pub async fn health(base_url: String) -> Result<String, String> {
        http::health(&base_url).await
    }

    /// The registered projects, newest activity first.
    pub async fn projects(base_url: String) -> Result<Vec<ProjectSummary>, String> {
        Projects::new(&base_url).list().await
    }

    /// The chats of one project (or all projects, when `project_id` is
    /// empty), newest first.
    pub async fn sessions(base_url: String, project_id: String) -> Result<Vec<SessionSummary>, String> {
        Sessions::new(&base_url).list(&project_id).await
    }

    /// Saves one workspace's layout document (the pane-tree JSON, owned by
    /// the TSX store). See `services::workspace` for the persistence
    /// contract.
    pub fn save_workspace_layout(key: String, document: String) -> Result<(), String> {
        WorkspacePersist::save(&key, &document)
    }

    /// Loads one workspace's layout document, or `None` when the workspace
    /// was never saved (a fresh project's first launch).
    pub fn load_workspace_layout(key: String) -> Result<Option<String>, String> {
        Ok(WorkspacePersist::load(&key))
    }

    /// Deletes one workspace's layout document (the workspace was closed or
    /// its worktree removed). Missing keys are fine.
    pub fn delete_workspace_layout(key: String) -> Result<(), String> {
        WorkspacePersist::delete(&key);
        Ok(())
    }

    /// Starts one agent run and forwards each `AgentSessionEvent` to
    /// `on_event` as camelCase JSON. The promise settles when the run's
    /// stream ends, so awaiting it is the run's lifecycle.
    pub async fn run_prompt(
        base_url: String,
        session_id: String,
        prompt: JsonValue,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        Chat::run_prompt(&base_url, &session_id, prompt, on_event).await
    }

    /// Reattaches to a run already in flight (an app restart mid-run), from
    /// `since` events in.
    pub async fn attach_run(
        base_url: String,
        session_id: String,
        since: Option<u64>,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        Chat::attach_run(&base_url, &session_id, since, on_event).await
    }

    /// One session's stored message history, as the server serializes it.
    pub async fn chat_messages(base_url: String, session_id: String) -> Result<String, String> {
        Chat::messages(&base_url, &session_id).await
    }

    /// Creates a session (the launcher's submit) and returns its header.
    pub async fn create_session(
        base_url: String,
        project_id: String,
        title: Option<String>,
    ) -> Result<String, String> {
        Chat::create_session(&base_url, &project_id, title).await
    }

    /// Answers a pending permission request (the card's Allow / Deny).
    pub async fn approve_permission(
        base_url: String,
        session_id: String,
        request_id: String,
        allow: bool,
    ) -> Result<(), String> {
        Chat::approve_permission(&base_url, &session_id, &request_id, allow).await
    }

    /// Answers a pending question (the card's option pick, custom answer, or
    /// skip). `answer` is the JSON value the server accepts.
    pub async fn answer_question(
        base_url: String,
        session_id: String,
        request_id: String,
        answer: JsonValue,
    ) -> Result<(), String> {
        Chat::answer_question(&base_url, &session_id, &request_id, answer).await
    }

    /// Whether a paired device credential is on disk for this server.
    pub fn auth_paired(base_url: String) -> bool {
        auth::paired(&base_url)
    }

    /// Exchanges a one-time pairing grant (the code the server shows) for
    /// this device's long-lived credential, stores it for the normalized
    /// server URL, and returns the paired device and server identity.
    pub async fn auth_pair_with_code(
        host: String,
        code: String,
        device_name: String,
    ) -> Result<String, String> {
        auth::pair_with_code(&host, &code, &device_name).await
    }

    /// Forgets this server's credential.
    pub fn auth_unpair(base_url: String) -> Result<(), String> {
        auth::unpair(&base_url)
    }

    /// The devices paired with this server (an authenticated read).
    pub async fn auth_devices(base_url: String) -> Result<JsonValue, String> {
        let devices = auth::devices(&base_url).await?;
        serde_json::to_value(&devices).map_err(|err| err.to_string())
    }

    /// Splits one pasted pairing line into its host and code (`{host, code}`)
    /// — one line fills both pairing fields, the desktop settings form's
    /// behaviour.
    pub fn auth_split_pairing_line(text: String) -> Result<String, String> {
        let (host, code) = auth::split_pairing_line(&text);
        serde_json::to_string(&serde_json::json!({ "host": host, "code": code }))
            .map_err(|err| err.to_string())
    }

    /// Soft-deletes one chat (trash; restorable for 7 days).
    pub async fn delete_session(base_url: String, session_id: String) -> Result<(), String> {
        Sessions::new(&base_url).delete(&session_id).await
    }

    /// The soft-deleted chats (trash), optionally one project's.
    pub async fn deleted_sessions(
        base_url: String,
        project_id: Option<String>,
    ) -> Result<Vec<SessionSummary>, String> {
        Sessions::new(&base_url)
            .list_deleted(project_id.as_deref())
            .await
    }

    /// Brings a soft-deleted chat back.
    pub async fn restore_session(base_url: String, session_id: String) -> Result<(), String> {
        Sessions::new(&base_url).restore(&session_id).await
    }

    /// Marks a finished chat settled (the server refuses while a run is in
    /// flight or a prompt is queued).
    pub async fn settle_session(base_url: String, session_id: String) -> Result<(), String> {
        Sessions::new(&base_url).settle(&session_id).await
    }

    /// Brings a settled chat back to active.
    pub async fn unsettle_session(base_url: String, session_id: String) -> Result<(), String> {
        Sessions::new(&base_url).unsettle(&session_id).await
    }

    /// Forks a chat at one of its messages and returns the forked header.
    pub async fn fork_session(
        base_url: String,
        session_id: String,
        message_id: String,
    ) -> Result<String, String> {
        Sessions::new(&base_url).fork(&session_id, &message_id).await
    }

    /// Saves one chat's composer draft (the desktop's draft sync shape; the
    /// draft JSON carries its own `sessionId`).
    pub async fn save_draft(base_url: String, draft: JsonValue) -> Result<(), String> {
        catalog::put_draft(&base_url, draft).await
    }

    /// Browses one directory (`null` = the server's default).
    pub async fn fs_browse(base_url: String, path: Option<String>) -> Result<String, String> {
        fs_git::browse(&base_url, path.as_deref()).await
    }

    /// One file's content and metadata (the file viewer's source).
    pub async fn fs_read_file(base_url: String, path: String) -> Result<String, String> {
        fs_git::read_file(&base_url, &path).await
    }

    /// The git status summary for a working tree (the repo path is required
    /// by the server).
    pub async fn git_status(base_url: String, path: String) -> Result<String, String> {
        fs_git::git_status(&base_url, &path).await
    }

    /// One file's diff against HEAD (`null` file = the whole tree's).
    pub async fn git_diff(
        base_url: String,
        repo_path: String,
        file_path: Option<String>,
    ) -> Result<String, String> {
        fs_git::git_diff(&base_url, &repo_path, file_path.as_deref()).await
    }

    /// The working tree's branches (the checkout menu's source; the repo
    /// path is required by the server).
    pub async fn git_branches(base_url: String, path: String) -> Result<String, String> {
        fs_git::git_branches(&base_url, &path).await
    }

    /// One chat's saved composer draft (`null` when there is none).
    pub async fn draft(base_url: String, session_id: String) -> Result<String, String> {
        catalog::draft(&base_url, &session_id).await
    }

    /// The provider catalog with its models (the model picker's source).
    pub async fn providers(base_url: String) -> Result<String, String> {
        catalog::providers(&base_url).await
    }

    /// One provider's models.
    pub async fn provider_models(base_url: String, provider_id: String) -> Result<String, String> {
        catalog::provider_models(&base_url, &provider_id).await
    }

    /// The user's favorite models (the picker's star state).
    pub async fn model_favorites(base_url: String) -> Result<String, String> {
        catalog::model_favorites(&base_url).await
    }

    /// Sets (or clears, with `false`) one favorite model — the picker's star.
    pub async fn set_model_favorite(
        base_url: String,
        provider: String,
        model_id: String,
        favorite: bool,
    ) -> Result<(), String> {
        catalog::set_model_favorite(&base_url, &provider, &model_id, favorite).await
    }

    /// Usage reports per provider (`null` where a provider has no report).
    pub async fn usage(base_url: String) -> Result<String, String> {
        catalog::usage(&base_url).await
    }

    /// The user's console settings.
    pub async fn settings(base_url: String) -> Result<String, String> {
        catalog::settings(&base_url).await
    }
}
