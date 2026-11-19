//! The per-pane chat snapshot: everything the workspace chat render needs
//! about the active session, gathered in one pass over app state.
//!
//! The view maps this struct to elements instead of digging into app fields —
//! which also means the session header and the active session id are resolved
//! once per frame instead of at every call site (todo count, footer provider,
//! launch state each used to re-walk the session list).

use std::rc::Rc;

use console_core::types::agent::AskQuestionRequest;
use console_core::types::{ContextSnapshot, UsageReport};
use console_core::{
    Automation, BackgroundWork, QueuedPrompt, SubagentInfo, TodoItem, WebhookSubscription,
};
use gpui::{Context, Window};

use super::ConsoleDesktopApp;

/// One surfaced banner error: its message and whether it stays until the
/// caller clears it (persistent errors carry Retry/Discard in the view).
#[derive(Clone)]
pub(crate) struct PaneError {
    pub message: String,
    pub persistent: bool,
}

/// Derived per-pane chat state for one render pass.
pub(crate) struct PaneChatSnapshot {
    /// The pane's active session, if it has one.
    pub session_id: Option<String>,
    /// Worktree launch state from the session header: "provisioning" while
    /// the server cuts the worktree, "failed" with Retry/Discard. Absent when
    /// there is nothing in flight.
    pub launch_state: Option<String>,
    pub error: Option<PaneError>,
    pub notice: Option<String>,
    pub todos: Rc<Vec<TodoItem>>,
    pub todos_completed: usize,
    /// Unfinished todos exist, so the tasks pill shows.
    pub todos_active: bool,
    pub subagents: Rc<Vec<SubagentInfo>>,
    pub subagents_running: bool,
    pub shells: Vec<BackgroundWork>,
    pub automations: Rc<Vec<Automation>>,
    pub webhooks: Rc<Vec<WebhookSubscription>>,
    pub queued_prompts: Vec<QueuedPrompt>,
    pub suggestions: Vec<String>,
    pub pending_question: Option<AskQuestionRequest>,
    pub usage_report: Option<UsageReport>,
    pub usage_loading: bool,
    pub context_snapshot: Option<ContextSnapshot>,
    /// The active session already has messages — the footer locks its
    /// project/cwd selector.
    pub session_has_messages: bool,
}

impl ConsoleDesktopApp {
    /// Gather the chat snapshot for `pane_id`. Pure: reads state, mutates
    /// nothing, so it is safe to call from render.
    pub(crate) fn pane_chat_snapshot(&self, pane_id: &str) -> PaneChatSnapshot {
        // One lookup each for the active session and its header; every
        // derived field reads from these instead of re-walking the lists.
        let session_id = self.active_session_for_pane(pane_id);
        let header = session_id
            .as_deref()
            .and_then(|sid| self.sessions.iter().find(|s| s.id == sid));
        let provider = header
            .map(|s| s.provider.clone())
            .unwrap_or_else(|| "claude".to_string());

        let todos: Rc<Vec<TodoItem>> = Rc::new(self.todo_items_for_pane(pane_id));
        let todos_completed = todos
            .iter()
            .filter(|item| {
                matches!(
                    item.status.to_ascii_lowercase().as_str(),
                    "done" | "completed" | "complete"
                )
            })
            .count();
        let todos_active = !todos.is_empty() && todos_completed < todos.len();

        let subagents: Rc<Vec<SubagentInfo>> = session_id
            .as_deref()
            .and_then(|sid| self.session_subagents.get(sid))
            .cloned()
            .unwrap_or_default();
        let subagents_running = subagents.iter().any(|sub| sub.status == "running");

        let shells = session_id
            .as_deref()
            .map(|sid| self.running_shells_for_session(sid))
            .unwrap_or_default();

        let automations = Rc::new(
            session_id
                .as_deref()
                .map(|sid| self.automations_for_session(sid))
                .unwrap_or_default(),
        );
        let webhooks = Rc::new(
            session_id
                .as_deref()
                .map(|sid| self.webhooks_for_session(sid))
                .unwrap_or_default(),
        );

        let usage_report = self
            .usage_reports
            .as_ref()
            .and_then(|reports| reports.get(&provider).cloned().flatten());

        PaneChatSnapshot {
            session_id: session_id.clone(),
            launch_state: header.and_then(|s| s.launch_state.clone()),
            error: self.error_for_pane(pane_id).map(|error| PaneError {
                message: error.message.clone(),
                persistent: error.persistent,
            }),
            notice: self.agent_notice_for_pane(pane_id),
            todos,
            todos_completed,
            todos_active,
            subagents,
            subagents_running,
            shells,
            automations,
            webhooks,
            queued_prompts: self.queued_prompts_for_pane(pane_id),
            suggestions: self.suggestions_for_pane(pane_id),
            pending_question: self.pending_question_for_pane(pane_id),
            usage_report,
            usage_loading: self.usage_loading,
            context_snapshot: session_id
                .as_deref()
                .and_then(|sid| self.context_usage.get(sid).cloned()),
            session_has_messages: self.session_has_messages(pane_id),
        }
    }

    /// Drop pill popovers whose pills just disappeared: a popover for a
    /// vanished tasks/subagents/shells pill could never close itself and
    /// would reopen stale on the next run. Pure state behavior — called from
    /// render but owns no element building.
    pub(crate) fn gc_pane_pill_menus(
        &mut self,
        pane_id: &str,
        snapshot: &PaneChatSnapshot,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if !snapshot.todos_active && self.pane_tasks_menu(pane_id).is_open() {
            self.pane_tasks_menu(pane_id).close(window, cx);
        }
        if snapshot.subagents.is_empty() && self.pane_subagents_menu(pane_id).is_open() {
            self.pane_subagents_menu(pane_id).close(window, cx);
        }
        if snapshot.shells.is_empty() && self.pane_shells_menu(pane_id).is_open() {
            self.pane_shells_menu(pane_id).close(window, cx);
        }
        if snapshot.automations.is_empty()
            && snapshot.webhooks.is_empty()
            && self.pane_automations_menu(pane_id).is_open()
        {
            self.pane_automations_menu(pane_id).close(window, cx);
        }
    }
}
