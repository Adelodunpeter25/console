//! Session bookkeeping: applying a loaded session's header to the UI,
//! persisting settings changes, answering agent questions, and loading a
//! session's messages into the transcript.

use std::rc::Rc;

use console_core::{ApprovalMode, SelectedModel, SessionHeader, UpdateSessionDto};
use gpui::{Context, Window};

use super::ConsoleDesktopApp;
use super::user_prompt_history;

impl ConsoleDesktopApp {
    pub fn load_sessions(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(
            async move |entity, cx| match client.sessions.list(None, None).await {
                Ok(sessions) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.sessions = Rc::new(sessions);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load sessions: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            },
        )
        .detach();
    }

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
            .map(|session| session.display_title().to_string())
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
            session.status = header.status.clone();
            session.updated_at = header.updated_at;
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
                cx.notify();
            }
        }

        // Keep the footer's project picker in step with the loaded chat: the
        // session's project, resolved by cwd path first (the backend only
        // persists `cwd` when the workspace changes), then by project id.
        self.sync_project_from_session_for_pane(pane_id, header, cx);

        // Tool-call rows render paths relative to the session's working
        // directory; empty when the backend has not reported one yet.
        let cwd = (!header.cwd.is_empty()).then(|| header.cwd.clone());
        self.transcript_for_pane(pane_id)
            .update(cx, |transcript, _| {
                transcript.set_session_cwd(cwd);
            });

        if self.right_sidebar_visible {
            self.refresh_inspector(cx);
        }
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

    pub fn select_and_open_session(&mut self, id: String, cx: &mut Context<Self>) {
        let active_pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let prev_sid = self
            .active_session_for_pane(&active_pane_id)
            .map(|s| s.to_string());

        self.save_transcript_scroll_position(cx);
        self.selected_session_id = Some(id.clone());
        let title = self
            .sessions
            .iter()
            .find(|s| s.id == id)
            .map(|s| s.display_title().to_string())
            .unwrap_or_else(|| "Chat".to_string());
        self.open_chat_tab(id.clone(), title);

        if prev_sid.as_deref() == Some(&id) {
            cx.notify();
            return;
        }

        let draft = self.get_draft_for_session(Some(&id)).map(|s| s.to_string());
        self.active_composer_input().update(cx, |input, cx| {
            input.set_prompt_history(Vec::new(), cx);
            if let Some(draft_text) = draft {
                input.set_content(draft_text, cx);
            } else {
                input.clear(cx);
            }
        });
        self.active_transcript_view().update(cx, |t, cx| {
            t.set_messages(Vec::new(), cx);
        });
        self.load_session_messages(id, cx);
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
    /// Uses tail pagination (limit 50) for fast initial paint; older pages
    /// auto-chain while the user holds at the top so the first prompt is
    /// reachable even for >100-msg sessions.
    pub fn load_session_messages_for_pane(
        &mut self,
        pane_id: String,
        session_id: String,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let saved_position = self.transcript_scroll_positions.get(&session_id).copied();
        cx.spawn(async move |entity, cx| {
            match client
                .sessions
                .get_paginated(&session_id, Some(50), None)
                .await
            {
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
                                if let Some(state) = this.workspace_pane_states.get_mut(&pane_id) {
                                    state.loaded_session_id = Some(session_id.clone());
                                }
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
                                let app_entity_for_older = entity.clone();
                                transcript.update(cx, |t, cx| {
                                    t.set_messages(detail.messages, cx);
                                    t.set_pagination(detail.has_more, detail.next_cursor);
                                    // Wire Load older handler for this pane/session
                                    let pane_for_older = pane_id.clone();
                                    let app_entity = app_entity_for_older.clone();
                                    t.set_on_load_older(move |_window, cx| {
                                        if let Some(app) = app_entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.load_older_messages_for_pane(
                                                    pane_for_older.clone(),
                                                    cx,
                                                );
                                            });
                                        }
                                    });
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
                                // Store pagination for scroll-based loading
                                this.set_pagination_for_session(
                                    &session_id,
                                    crate::state::pagination::SessionPaginationState {
                                        has_more: detail.has_more,
                                        next_cursor: detail.next_cursor,
                                    },
                                    cx,
                                );

                                if detail.header.status
                                    == Some(console_core::SessionStatus::Working)
                                    && !this.is_session_running(&session_id)
                                {
                                    this.attach_session_run_for_pane(
                                        pane_id.clone(),
                                        session_id.clone(),
                                        cx,
                                    );
                                }

                                // Hydrate the persisted todo checklist so a
                                // reopened session shows its outstanding tasks
                                // right away instead of waiting for the next
                                // run's todoUpdate (docs/todo-integration.md).
                                let todo_client = client.clone();
                                let todo_pane_id = pane_id.clone();
                                let todo_session_id = session_id.clone();
                                cx.spawn(async move |entity, cx| {
                                    if let Ok(items) =
                                        todo_client.sessions.get_todos(&todo_session_id).await
                                    {
                                        let _ = cx.update(|cx| {
                                            if let Some(app) = entity.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    // Ignore a slower response after
                                                    // this pane has switched to
                                                    // another active tab.
                                                    if this
                                                        .active_session_for_pane(&todo_pane_id)
                                                        .as_deref()
                                                        != Some(todo_session_id.as_str())
                                                    {
                                                        return;
                                                    }
                                                    this.set_todo_items_for_session(
                                                        &todo_session_id,
                                                        items,
                                                    );
                                                    cx.notify();
                                                });
                                            }
                                        });
                                    }
                                })
                                .detach();

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
