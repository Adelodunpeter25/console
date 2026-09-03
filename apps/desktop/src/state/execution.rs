use console_core::types::agent::{AskQuestionRequest, PermissionRequest};
use console_core::{ImageAttachment, SessionStatus};
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// A session is running iff it is present in `running_sessions`. Keyed by
    /// session id so the indicator stays with the owning chat across pane
    /// switches.
    pub(crate) fn is_session_running(&self, session_id: &str) -> bool {
        self.running_sessions.contains_key(session_id)
    }

    /// Mark a session as running (`Some(started_at)`) or idle (`None`). The
    /// started_at drives the sidebar's "Working for Ns" label.
    pub(crate) fn set_session_running(&mut self, session_id: &str, started_at: Option<i64>) {
        match started_at {
            Some(t) => {
                self.running_sessions.insert(session_id.to_string(), t);
                if let Some(session) = Rc::make_mut(&mut self.sessions)
                    .iter_mut()
                    .find(|s| s.id == session_id)
                {
                    session.status = Some(SessionStatus::Working);
                }
            }
            None => {
                self.running_sessions.remove(session_id);
                if let Some(session) = Rc::make_mut(&mut self.sessions)
                    .iter_mut()
                    .find(|s| s.id == session_id)
                {
                    if session.status == Some(SessionStatus::Working) {
                        session.status = None;
                    }
                }
            }
        }
    }

    /// Snapshot of all currently running sessions and their start times, for
    /// the sidebar to highlight every running chat (one per pane).
    pub(crate) fn running_sessions_snapshot(&self) -> HashMap<String, i64> {
        self.running_sessions.clone()
    }

    /// Advance and return the monotonic run token for a session.
    pub(crate) fn next_run_token_for_session(&mut self, session_id: &str) -> u64 {
        let token = self
            .session_run_tokens
            .entry(session_id.to_string())
            .or_insert(0);
        *token = token.wrapping_add(1);
        *token
    }

    /// Read the current run token for a session without advancing it.
    pub(crate) fn current_run_token_for_session(&self, session_id: &str) -> u64 {
        self.session_run_tokens.get(session_id).copied().unwrap_or(0)
    }

    /// Whether the session currently displayed in a pane is running. Used by
    /// the composer to show Stop vs Send for the chat the pane is actually
    /// showing, not whichever chat happened to start its run from this pane.
    pub(crate) fn is_active_session_running_for_pane(&self, pane_id: &str) -> bool {
        self.active_session_for_pane(pane_id)
            .is_some_and(|sid| self.is_session_running(&sid))
    }

    pub(crate) fn stream_render_pending_for_pane(&self, pane_id: &str) -> bool {
        self.stream_render_pending.get(pane_id).copied().unwrap_or(false)
    }

    pub(crate) fn set_stream_render_pending_for_pane(&mut self, pane_id: &str, pending: bool) {
        if pending {
            self.stream_render_pending.insert(pane_id.to_string(), true);
        } else {
            self.stream_render_pending.remove(pane_id);
        }
    }

    pub(crate) fn pending_permission_for_pane(&self, pane_id: &str) -> Option<PermissionRequest> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.pending_permissions.get(&sid).cloned())
    }

    pub(crate) fn set_pending_permission_for_session(
        &mut self,
        session_id: &str,
        perm: Option<PermissionRequest>,
    ) {
        if let Some(p) = perm {
            self.pending_permissions.insert(session_id.to_string(), p);
        } else {
            self.pending_permissions.remove(session_id);
        }
    }

    pub(crate) fn pending_question_for_pane(&self, pane_id: &str) -> Option<AskQuestionRequest> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.pending_questions.get(&sid).cloned())
    }

    pub(crate) fn set_pending_question_for_session(
        &mut self,
        session_id: &str,
        q: Option<AskQuestionRequest>,
    ) {
        if let Some(q) = q {
            self.pending_questions.insert(session_id.to_string(), q);
        } else {
            self.pending_questions.remove(session_id);
        }
    }

    pub(crate) fn question_selected_for_pane(&self, pane_id: &str) -> HashSet<String> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.question_selected.get(&sid).cloned())
            .unwrap_or_default()
    }

    pub(crate) fn set_question_selected_for_session(
        &mut self,
        session_id: &str,
        selected: HashSet<String>,
    ) {
        if selected.is_empty() {
            self.question_selected.remove(session_id);
        } else {
            self.question_selected.insert(session_id.to_string(), selected);
        }
    }

    pub(crate) fn question_selected_for_session(
        &self,
        session_id: &str,
    ) -> HashSet<String> {
        self.question_selected
            .get(session_id)
            .cloned()
            .unwrap_or_default()
    }

    pub(crate) fn clear_question_selected_for_session(&mut self, session_id: &str) {
        self.question_selected.remove(session_id);
    }

    pub(crate) fn attachments_for_pane(&self, pane_id: &str) -> Rc<Vec<ImageAttachment>> {
        self.attachments
            .get(pane_id)
            .cloned()
            .unwrap_or_else(|| Rc::new(Vec::new()))
    }

    pub(crate) fn set_attachments_for_pane(&mut self, pane_id: &str, items: Vec<ImageAttachment>) {
        if items.is_empty() {
            self.attachments.remove(pane_id);
        } else {
            self.attachments.insert(pane_id.to_string(), Rc::new(items));
        }
    }

    /// Append staged images to a pane's chips without cloning the existing
    /// payload vec when it is shared with an in-flight render.
    pub(crate) fn append_attachments_for_pane(
        &mut self,
        pane_id: &str,
        mut items: Vec<ImageAttachment>,
    ) {
        if items.is_empty() {
            return;
        }
        match self.attachments.get_mut(pane_id) {
            Some(existing) => Rc::make_mut(existing).append(&mut items),
            None => {
                self.attachments.insert(pane_id.to_string(), Rc::new(items));
            }
        }
    }

    pub(crate) fn agent_notice_for_pane(&self, pane_id: &str) -> Option<String> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.agent_notices.get(&sid).cloned())
    }

    pub(crate) fn set_agent_notice_for_session(&mut self, session_id: &str, notice: Option<String>) {
        if let Some(n) = notice {
            self.agent_notices.insert(session_id.to_string(), n);
        } else {
            self.agent_notices.remove(session_id);
        }
    }

    /// Sessions waiting for a permission or question response, for the sidebar
    /// to highlight each waiting chat independently.
    pub(crate) fn waiting_sessions_snapshot(&self) -> HashSet<String> {
        let mut out: HashSet<String> = self.pending_permissions.keys().cloned().collect();
        out.extend(self.pending_questions.keys().cloned());
        out
    }
}
