//! Session bookkeeping: applying a loaded session's header to the UI,
//! persisting settings changes, answering agent questions, and loading a
//! session's messages into the transcript.

use std::rc::Rc;

use console_core::{ApprovalMode, SelectedModel, SessionHeader, UpdateSessionDto};
use gpui::{Context, Window};

use super::ConsoleDesktopApp;
use super::user_prompt_history;

impl ConsoleDesktopApp {
    pub(crate) fn begin_session_rename(
        &mut self,
        session_id: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let Some(title) = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .map(|session| {
                if session.title.trim().is_empty() {
                    "New Chat".to_string()
                } else {
                    session.title.clone()
                }
            })
        else {
            return;
        };

        self.session_rename_id = Some(session_id);
        self.session_rename_input.update(cx, |input, cx| {
            input.set_content(title, cx);
            input.select_all_text(cx);
        });
        let focus = self.session_rename_input.read(cx).focus();
        window.on_next_frame(move |window, cx| window.focus(&focus, cx));
        cx.notify();
    }

    pub(crate) fn cancel_session_rename(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.session_rename_id.take().is_some() {
            let focus = self.active_composer_input().read(cx).focus();
            window.focus(&focus, cx);
            cx.notify();
        }
    }

    pub(crate) fn commit_session_rename(&mut self, cx: &mut Context<Self>) {
        let Some(session_id) = self.session_rename_id.take() else {
            return;
        };
        let title = self
            .session_rename_input
            .read(cx)
            .content()
            .trim()
            .to_owned();
        if title.is_empty() {
            cx.notify();
            return;
        }

        let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        else {
            return;
        };
        if session.title == title {
            cx.notify();
            return;
        }
        session.title = title.clone();
        console_ui::workspace::ops::rename_tabs(
            &mut self.workspace_root,
            |tab| {
                matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: tab_session_id, .. }
                    if tab_session_id == &session_id)
            },
            title.clone(),
        );
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            if let Err(error) = client
                .sessions
                .update(
                    &session_id,
                    UpdateSessionDto {
                        title: Some(title),
                        cwd: None,
                        model_id: None,
                        provider: None,
                        approval_mode: None,
                    },
                )
                .await
            {
                let message = format!("Unable to rename session: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_error_for_session(&session_id, message, cx)
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Persist a settings change (model, provider, approval mode) on a pane's
    /// active session. Surfaces failures through the error banner.
    pub fn update_session_settings_for_pane(
        &mut self,
        pane_id: String,
        payload: UpdateSessionDto,
        cx: &mut Context<Self>,
    ) {
        let Some(session_id) = self.active_session_for_pane(&pane_id) else {
            return;
        };
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            if let Err(error) = client.sessions.update(&session_id, payload).await {
                let message = format!("Unable to update session settings: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_error_for_session(&session_id, message, cx)
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Answer the pending question owned by `session_id`. The id is resolved
    /// by the caller (the pane whose card rendered the question) so focus in
    /// another split can never retarget the answer.
    pub fn answer_pending_question_for_session(
        &mut self,
        session_id: String,
        answer: serde_json::Value,
        cx: &mut Context<Self>,
    ) {
        let Some(question) = self.pending_questions.remove(&session_id) else {
            return;
        };
        if answer
            .as_str()
            .map(|value| value.trim().is_empty())
            .unwrap_or(false)
        {
            self.set_pending_question_for_session(&session_id, Some(question));
            return;
        }

        self.clear_question_selected_for_session(&session_id);
        self.clear_question_inputs_for_session(&session_id, cx);
        self.clear_error_for_session(&session_id, cx);
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            if let Err(error) = client
                .runs
                .answer_question(
                    &session_id,
                    console_core::AnswerQuestionDto {
                        request_id: question.request_id,
                        answer,
                    },
                )
                .await
            {
                let message = format!("Unable to answer the agent question: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_error_for_session(&session_id, message, cx)
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Apply a session header to one pane's controls, plus shared title/sidebar
    /// metadata and that pane's project picker.
    pub(crate) fn apply_session_header_for_pane(
        &mut self,
        pane_id: &str,
        header: &SessionHeader,
        cx: &mut Context<Self>,
    ) {
        if !header.model_id.is_empty() && !header.provider.is_empty() {
            self.set_pane_model(
                pane_id,
                Some(SelectedModel {
                    provider: header.provider.clone(),
                    model_id: header.model_id.clone(),
                }),
            );
        }
        self.set_pane_approval_mode(
            pane_id,
            header
                .approval_mode
                .as_deref()
                .map(ApprovalMode::from_value)
                .unwrap_or_default(),
        );

        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == header.id)
        {
            session.model_id = header.model_id.clone();
            session.provider = header.provider.clone();
            session.approval_mode = header.approval_mode.clone();
            // Keep the sidebar row and any open chat tabs in step when the
            // backend renames the session (e.g. after the first turn).
            if !header.title.trim().is_empty() && session.title != header.title {
                session.title = header.title.clone();
                console_ui::workspace::ops::rename_tabs(
                    &mut self.workspace_root,
                    |tab| {
                        matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id, .. }
                            if session_id == &header.id)
                    },
                    header.title.clone(),
                );
            }
        }

        // Keep the footer's project picker in step with the loaded chat: the
        // session's project, resolved by cwd path first (the backend only
        // persists `cwd` when the workspace changes), then by project id.
        self.sync_project_from_session_for_pane(pane_id, header, cx);

        // Tool-call rows render paths relative to the session's working
        // directory; empty when the backend has not reported one yet.
        let cwd = (!header.cwd.is_empty()).then(|| header.cwd.clone());
        self.transcript_for_pane(pane_id).update(cx, |transcript, _| {
            transcript.set_session_cwd(cwd);
        });
    }

    /// Point a pane's project picker at the project a loaded session belongs
    /// to, so opening a chat never shows a stale "No project".
    fn sync_project_from_session_for_pane(
        &mut self,
        pane_id: &str,
        header: &SessionHeader,
        cx: &mut Context<Self>,
    ) {
        let resolved = self
            .projects
            .iter()
            .find(|project| project.path == header.cwd)
            .or_else(|| {
                header
                    .project_id
                    .as_deref()
                    .and_then(|id| self.projects.iter().find(|project| &project.id == id))
            });
        let target_id = resolved.map(|project| project.id.clone());
        if target_id == self.pane_project_id(pane_id) {
            return;
        }
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.selected_project_id = target_id;
            Rc::make_mut(&mut state.branches).clear();
            state.branch_loaded = resolved.is_none();
            state.branch_is_git_repository = false;
        }
        let Some(project) = resolved.cloned() else {
            cx.notify();
            return;
        };

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let pane_id = pane_id.to_string();
        cx.spawn(async move |_entity, cx| {
            match client.git.list_branches(Some(&project.path)).await {
                Ok(branches) => cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) = this.workspace_pane_states.get_mut(&pane_id) {
                                state.branches = Rc::new(branches.branches);
                                state.branch_loaded = true;
                                state.branch_is_git_repository = branches.is_git_repository;
                            }
                            cx.notify();
                        });
                    }
                }),
                Err(_) => cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) = this.workspace_pane_states.get_mut(&pane_id) {
                                Rc::make_mut(&mut state.branches).clear();
                                state.branch_loaded = true;
                                state.branch_is_git_repository = false;
                            }
                            cx.notify();
                        });
                    }
                }),
            }
        })
        .detach();
        cx.notify();
    }

    pub fn load_session_messages(&mut self, session_id: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        self.load_session_messages_for_pane(pane_id, session_id, cx);
    }

    /// Load a chat into one pane without replacing another pane's transcript.
    pub fn load_session_messages_for_pane(
        &mut self,
        pane_id: String,
        session_id: String,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let saved_position = self.transcript_scroll_positions.get(&session_id).copied();
        cx.spawn(async move |entity, cx| {
            match client.sessions.get(&session_id).await {
                Ok(detail) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                // Ignore a slower response after this pane has
                                // switched to another active tab.
                                if this.active_session_for_pane(&pane_id).as_deref()
                                    != Some(session_id.as_str())
                                {
                                    return;
                                }
                                this.apply_session_header_for_pane(&pane_id, &detail.header, cx);
                                let composer = this.composer_for_pane(&pane_id);
                                let transcript = this.transcript_for_pane(&pane_id);
                                let running_started_at = this
                                    .is_session_running(&session_id)
                                    .then(|| this.running_sessions.get(&session_id).copied())
                                    .flatten();
                                composer.update(cx, |input, cx| {
                                    input.set_prompt_history(
                                        user_prompt_history(&detail.messages),
                                        cx,
                                    );
                                });
                                transcript.update(cx, |t, cx| {
                                    t.set_messages(detail.messages, cx);
                                    if let Some(started_at) = running_started_at {
                                        t.resume_streaming(started_at, cx);
                                    }
                                    if let Some(position) = saved_position {
                                        t.restore_scroll_anchor(
                                            position.row_index,
                                            position.offset_in_row,
                                            position.at_tail,
                                            cx,
                                        );
                                    }
                                });

                                if detail.header.status == Some(console_core::SessionStatus::Working)
                                    && !this.is_session_running(&session_id)
                                {
                                    this.attach_session_run_for_pane(
                                        pane_id.clone(),
                                        session_id.clone(),
                                        cx,
                                    );
                                }

                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load session: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_error_for_session(&session_id, message, cx)
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }
}
