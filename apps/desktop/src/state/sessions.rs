//! Session bookkeeping: applying a loaded session's header to the UI,
//! persisting settings changes, answering agent questions, and loading a
//! session's messages into the transcript.

use std::collections::HashSet;
use std::rc::Rc;

use console_core::{
    ApprovalMode, ProjectInfo, ProjectInfoExt, SelectedModel, SessionHeader, SessionHeaderExt,
    UpdateSessionDto,
};
use console_ui::utils::{SidebarSortMode, group_indices_by_date, group_indices_by_project};
use gpui::{Context, Window};

use super::ConsoleDesktopApp;
use super::user_prompt_history;

impl ConsoleDesktopApp {
    /// Store a fresh session list and bring the Dock badge in line with it.
    /// Every place that replaces the list goes through here, so the badge can
    /// never drift from what the sidebar shows.
    pub(crate) fn set_session_list(
        &mut self,
        sessions: Vec<SessionHeader>,
        cx: &mut Context<Self>,
    ) {
        // Decided on the fresh list, before it replaces the local one: the
        // server is the one that knows a chat finished while the app was away,
        // and a stale local copy would hide exactly that.
        let active = self
            .active_pane_id
            .as_deref()
            .and_then(|pane| self.active_session_for_pane(pane));
        let read_active = console_core::types::notification::fresh_list_needs_read(
            &sessions,
            active.as_deref(),
            self.is_window_active,
        );
        self.sessions = Rc::new(sessions);
        self.refresh_dock_badge();
        if read_active && let Some(active) = active {
            // The refresh this triggers returns the chat read, so it ends there.
            self.force_session_read(&active, cx);
        }
    }

    /// Recount the Dock badge from the current session list. Called after any
    /// local edit that changes a chat's unread or settled state, so the badge
    /// follows an optimistic change at once instead of at the next refresh.
    pub(crate) fn refresh_dock_badge(&self) {
        super::macos_notifications::set_dock_badge(console_core::types::notification::badge_count(
            &self.sessions,
        ));
    }

    pub fn load_sessions(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        // Chats known when the request left: one created while it was in
        // flight is missing from a response that raced it.
        let known_at_request: HashSet<String> =
            self.sessions.iter().map(|s| s.id.clone()).collect();
        cx.spawn(
            async move |entity, cx| match client.sessions.list(None, None).await {
                Ok(sessions) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let created_meanwhile = this
                                    .sessions
                                    .iter()
                                    .any(|s| !known_at_request.contains(&s.id));
                                this.set_session_list(sessions, cx);
                                // Chats that no longer exist cannot be
                                // restored to, so their remembered positions
                                // are dropped.
                                this.prune_transcript_scroll_positions(cx);
                                // A worktree whose chats are all gone loses its
                                // workspace too — unless the response may predate
                                // a chat just created (it would look dead).
                                if !created_meanwhile {
                                    this.sweep_dead_worktree_workspaces(cx);
                                }
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
        for root in self.project_workspace_roots.values_mut() {
            console_ui::workspace::ops::rename_tabs(
                root,
                |tab| {
                    matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: tab_session_id, .. }
                        if tab_session_id == &session_id)
                },
                title.clone(),
            );
        }
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
                        project_id: None,
                        model_id: None,
                        provider: None,
                        approval_mode: None,
                        thinking_level: None,
                        pinned: None,
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

    pub fn start_sidebar_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.sidebar_visible = true;
        self.sidebar_search_active = true;
        if self.sidebar_search_query.is_empty() {
            self.sidebar_search_input.update(cx, |input, cx| {
                input.clear(cx);
            });
        } else {
            self.sidebar_search_input.update(cx, |input, cx| {
                input.select_all_text(cx);
            });
        }
        let focus = self.sidebar_search_input.read(cx).focus();
        window.focus(&focus, cx);
        cx.notify();
    }

    pub fn clear_sidebar_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.sidebar_search_active = false;
        self.sidebar_search_query.clear();
        self.sidebar_search_task = None;
        self.sidebar_search_input.update(cx, |input, cx| {
            input.clear(cx);
        });
        let focus = self.active_composer_input().read(cx).focus();
        window.focus(&focus, cx);
        cx.notify();
    }

    pub fn reset_or_close_sidebar_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let has_text = !self.sidebar_search_input.read(cx).content().is_empty();
        if has_text {
            self.sidebar_search_query.clear();
            self.sidebar_search_task = None;
            self.sidebar_search_input.update(cx, |input, cx| {
                input.clear(cx);
            });
            let focus = self.sidebar_search_input.read(cx).focus();
            window.focus(&focus, cx);
            cx.notify();
        } else {
            self.clear_sidebar_search(window, cx);
        }
    }

    pub fn open_first_search_result(&mut self, cx: &mut Context<Self>) {
        let ids = self.visible_session_ids();
        if let Some(first_id) = ids.into_iter().next() {
            self.select_and_open_session(first_id, cx);
        }
    }

    pub fn on_sidebar_search_query_changed(&mut self, query: String, cx: &mut Context<Self>) {
        self.sidebar_search_query = query.clone();
        self.sidebar_list_state.scroll_to_reveal_item(0);
        let client = self.client.clone();
        let trimmed = query.trim().to_string();
        if trimmed.is_empty() {
            self.sidebar_search_task = None;
            cx.notify();
            return;
        }
        self.sidebar_search_task = Some(cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(150))
                .await;
            if let Ok(results) = client.sessions.search_by_title(&trimmed).await {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if this.sidebar_search_query.trim() == trimmed {
                                this.merge_sidebar_search_results(results, cx);
                            }
                        });
                    }
                });
            }
        }));
        cx.notify();
    }

    pub fn merge_sidebar_search_results(
        &mut self,
        server_sessions: Vec<SessionHeader>,
        cx: &mut Context<Self>,
    ) {
        let existing_ids: HashSet<String> = self.sessions.iter().map(|s| s.id.clone()).collect();
        let new_sessions: Vec<SessionHeader> = server_sessions
            .into_iter()
            .filter(|s| !existing_ids.contains(&s.id))
            .collect();
        if !new_sessions.is_empty() {
            let mut all = (*self.sessions).clone();
            all.extend(new_sessions);
            self.sessions = Rc::new(all);
        }
        cx.notify();
    }

    pub(crate) fn toggle_pin_session(&mut self, session_id: String, cx: &mut Context<Self>) {
        let Some(current) = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .map(|session| session.pinned_at.is_some())
        else {
            return;
        };
        let pinned = !current;
        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        {
            session.pinned_at = if pinned {
                Some(chrono::Utc::now().timestamp_millis())
            } else {
                None
            };
            // Pinned and settled are mutually exclusive: pinning unsettles.
            if pinned {
                session.settled_at = None;
            }
        }
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            match client
                .sessions
                .update(
                    &session_id,
                    UpdateSessionDto {
                        title: None,
                        cwd: None,
                        project_id: None,
                        model_id: None,
                        provider: None,
                        approval_mode: None,
                        thinking_level: None,
                        pinned: Some(pinned),
                    },
                )
                .await
            {
                Ok(header) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if let Some(session) = Rc::make_mut(&mut this.sessions)
                                    .iter_mut()
                                    .find(|session| session.id == session_id)
                                {
                                    session.pinned_at = header.pinned_at;
                                    session.settled_at = header.settled_at;
                                }
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to update pin: {error}");
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

    /// Settle a chat (optimistic, like the pin toggle). The server refuses
    /// while a run is in flight or a prompt is queued; the local mark rolls
    /// back and the server's message shows on the error banner.
    pub(crate) fn settle_session(&mut self, session_id: String, cx: &mut Context<Self>) {
        self.set_settled(session_id, true, cx);
    }

    /// Bring a settled chat back to active. Unsettling has no busy limit.
    pub(crate) fn unsettle_session(&mut self, session_id: String, cx: &mut Context<Self>) {
        self.set_settled(session_id, false, cx);
    }

    /// Toggle the settled mark on the chat the user is looking at: the active
    /// pane's chat, falling back to the last session picked in the sidebar.
    /// A settled chat comes back to active; an active one files away.
    pub(crate) fn toggle_settle_selected_session(&mut self, cx: &mut Context<Self>) {
        let Some(session_id) = self.settle_target_session_id() else {
            return;
        };
        let settled = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .is_some_and(|session| session.settled_at.is_some());
        self.set_settled(session_id, !settled, cx);
    }

    /// The chat a settle shortcut acts on. Drafts are excluded: the sidebar
    /// offers no settle affordance on them, and settling one would list it
    /// in the Drafts and the Settled section at once.
    fn settle_target_session_id(&self) -> Option<String> {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let session_id = self
            .active_session_for_pane(&pane_id)
            .or_else(|| self.selected_session_id.clone())?;
        if self.sidebar_draft_ids.contains(&session_id) {
            return None;
        }
        Some(session_id)
    }

    fn set_settled(&mut self, session_id: String, settled: bool, cx: &mut Context<Self>) {
        let Some(previous) = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .map(|session| (session.settled_at, session.pinned_at))
        else {
            return;
        };
        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        {
            session.settled_at = if settled {
                Some(chrono::Utc::now().timestamp_millis())
            } else {
                None
            };
            // Pinned and settled are mutually exclusive: settling unpins.
            if settled {
                session.pinned_at = None;
            }
        }
        // A settled chat drops out of the Dock badge count, an unsettled one
        // comes back into it.
        self.refresh_dock_badge();
        // Settling a chat closes its tab(s), active or not. The optimistic
        // mark lands first so the pane sync below sees the settled session.
        if settled {
            self.close_session_chat_tabs(&session_id, cx);
        }
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            let outcome = if settled {
                client.sessions.settle(&session_id).await
            } else {
                client.sessions.unsettle(&session_id).await
            };
            match outcome {
                Ok(()) => {}
                Err(error) => {
                    let message = format!("Unable to settle: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                // Roll the optimistic mark back before
                                // showing the server's refusal.
                                if let Some(session) = Rc::make_mut(&mut this.sessions)
                                    .iter_mut()
                                    .find(|session| session.id == session_id)
                                {
                                    (session.settled_at, session.pinned_at) = previous;
                                }
                                this.refresh_dock_badge();
                                this.set_error_for_session(&session_id, message, cx)
                            });
                        }
                    });
                }
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

        // Optimistically sync provider changes to open chat tabs, local session state, and usage meter
        if let Some(ref prov) = payload.provider
            && !prov.is_empty()
        {
            console_ui::workspace::ops::sync_chat_provider(
                &mut self.workspace_root,
                |tab| {
                    matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: sid, .. }
                            if sid == &session_id)
                },
                prov.clone(),
            );
            for root in self.project_workspace_roots.values_mut() {
                console_ui::workspace::ops::sync_chat_provider(
                    root,
                    |tab| {
                        matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id: sid, .. }
                                if sid == &session_id)
                    },
                    prov.clone(),
                );
            }
            self.persist_workspaces();
            self.maybe_fetch_usage(&session_id, cx);
            self.fetch_context_usage(&session_id, cx);
            cx.notify();
        }

        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        {
            if let Some(prov) = payload.provider.as_deref().filter(|p| !p.is_empty()) {
                session.provider = prov.to_string();
            }
            if let Some(ref mid) = payload.model_id {
                session.model_id = mid.clone();
            }
            if payload.thinking_level.is_some() {
                session.thinking_level = payload
                    .thinking_level
                    .as_ref()
                    .map(|t| t.as_str().to_string());
                cx.notify();
            }
        }

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let sid = session_id.clone();
        let pid = pane_id.clone();
        cx.spawn(
            async move |_entity, cx| match client.sessions.update(&sid, payload).await {
                Ok(header) => {
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.apply_session_header_for_pane(&pid, &header, cx);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to update session settings: {error}");
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_error_for_session(&sid, message, cx)
                            });
                        }
                    });
                }
            },
        )
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
        self.send_question_answer(session_id, question, answer, cx);
    }

    /// Skip the pending question owned by `session_id`. The server records an
    /// empty answer as "User skipped this question." Required questions
    /// (`skippable == false`) cannot be skipped.
    pub fn skip_pending_question_for_session(
        &mut self,
        session_id: String,
        cx: &mut Context<Self>,
    ) {
        let Some(question) = self.pending_questions.remove(&session_id) else {
            return;
        };
        if question.skippable == Some(false) {
            self.set_pending_question_for_session(&session_id, Some(question));
            return;
        }
        self.send_question_answer(
            session_id,
            question,
            serde_json::Value::String(String::new()),
            cx,
        );
    }

    fn send_question_answer(
        &mut self,
        session_id: String,
        question: console_core::AskQuestionRequest,
        answer: serde_json::Value,
        cx: &mut Context<Self>,
    ) {
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
        self.set_pane_approval_mode(pane_id, ApprovalMode::from_value(&header.approval_mode));
        if let Some(level) = header
            .thinking_level
            .as_deref()
            .and_then(console_core::ThinkingLevel::from_str)
        {
            self.set_pane_thinking_level(pane_id, Some(level));
        }

        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == header.id)
        {
            session.cwd = header.cwd.clone();
            session.project_id = header.project_id.clone();
            session.model_id = header.model_id.clone();
            session.provider = header.provider.clone();
            session.approval_mode = header.approval_mode.clone();
            session.thinking_level = header.thinking_level.clone();
            session.status = header.status.clone();
            session.updated_at = header.updated_at;
            // Keep the sidebar row and any open chat tabs in step when the
            // backend renames the session (e.g. after the first turn) or updates
            // the provider/model.
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
                for root in self.project_workspace_roots.values_mut() {
                    console_ui::workspace::ops::rename_tabs(
                        root,
                        |tab| {
                            matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id, .. }
                                if session_id == &header.id)
                        },
                        header.title.clone(),
                    );
                }
                cx.notify();
            }
            // Sync provider changes to all open chat tabs for this session.
            if !header.provider.is_empty() {
                console_ui::workspace::ops::sync_chat_provider(
                    &mut self.workspace_root,
                    |tab| {
                        matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id, .. }
                            if session_id == &header.id)
                    },
                    header.provider.clone(),
                );
                for root in self.project_workspace_roots.values_mut() {
                    console_ui::workspace::ops::sync_chat_provider(
                        root,
                        |tab| {
                            matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id, .. }
                                if session_id == &header.id)
                        },
                        header.provider.clone(),
                    );
                }
                cx.notify();
            }
        }

        // A confirmed pick retires the pending override so later headers flow
        // through normally; anything still pending keeps protecting reopens.
        if self
            .pending_project_override
            .get(&header.id)
            .is_some_and(|ov| ov.as_deref() == header.project_id.as_deref())
        {
            self.pending_project_override.remove(&header.id);
        }

        // Keep the footer's project picker in step with the loaded chat: the
        // session's project, resolved by cwd path first (the backend only
        // persists `cwd` when the workspace changes), then by project id.
        self.sync_project_from_session_for_pane(pane_id, header, cx);

        // The footer's branches describe the session's own folder (a worktree
        // session's cwd differs from its project path), so keep them loaded
        // against that — skipped when already loaded for this folder.
        if !header.cwd.is_empty()
            && !self.pending_project_override.contains_key(&header.id)
            && self.pane_project_id(pane_id).is_some()
            && self
                .workspace_pane_states
                .get(pane_id)
                .is_some_and(|state| state.branch_cwd.as_deref() != Some(header.cwd.as_str()))
        {
            self.reload_branches_for_pane(pane_id.to_string(), header.cwd.clone(), cx);
        }

        // Fetch usage data for the current provider
        self.maybe_fetch_usage(&header.id, cx);
        self.fetch_context_usage(&header.id, cx);

        // Tool-call rows render paths relative to the session's working
        // directory; empty when the backend has not reported one yet.
        let cwd = (!header.cwd.is_empty()).then(|| header.cwd.clone());
        self.transcript_for_pane(pane_id)
            .update(cx, |transcript, _| {
                transcript.set_session_cwd(cwd);
            });

        self.maybe_refresh_inspector(cx);
    }

    /// Point a pane's project picker at the project a loaded session belongs
    /// to, so opening a chat never shows a stale "No project".
    fn sync_project_from_session_for_pane(
        &mut self,
        pane_id: &str,
        header: &SessionHeader,
        cx: &mut Context<Self>,
    ) {
        // A pending user pick beats a possibly stale server header (e.g. a
        // just-cleared No project whose update hasn't landed): apply it and
        // skip header resolution plus the branch fetch.
        if let Some(override_project) = self.pending_project_override.get(&header.id).cloned() {
            if override_project != self.pane_project_id(pane_id) {
                if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
                    state.selected_project_id = override_project.clone();
                    Rc::make_mut(&mut state.branches).clear();
                    state.branch_loaded = true;
                    state.branch_is_git_repository = false;
                }
                self.selected_project_id = override_project;
                cx.notify();
            }
            return;
        }
        let resolved = self
            .projects
            .iter()
            .find(|project| project.path == header.cwd)
            .or_else(|| {
                header
                    .project_id
                    .as_deref()
                    .and_then(|id| self.projects.iter().find(|project| project.id == id))
            });
        let target_id = resolved.map(|project| {
            match console_core::session_checkout_path(header, Some(&project.path)) {
                Some(path) => console_core::worktree_workspace_key(&project.id, path),
                None => project.id.clone(),
            }
        });
        // Same project (worktree or not): the pane's workspace already fits
        // this chat. Switching workspaces is `switch_workspace_for_session`'s
        // job, never a side effect of applying a header.
        let same_project = |a: &Option<String>, b: &Option<String>| match (a, b) {
            (Some(a), Some(b)) => {
                console_core::project_id_of_workspace_key(a)
                    == console_core::project_id_of_workspace_key(b)
            }
            (None, None) => true,
            _ => false,
        };
        if same_project(&target_id, &self.pane_project_id(pane_id)) {
            return;
        }
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.selected_project_id = target_id.clone();
            Rc::make_mut(&mut state.branches).clear();
            state.branch_loaded = resolved.is_none();
            state.branch_is_git_repository = false;
        }
        self.selected_project_id = target_id;
        let Some(project) = resolved.cloned() else {
            cx.notify();
            return;
        };

        self.reload_branches_for_pane(pane_id.to_string(), project.path.clone(), cx);
        cx.notify();
    }

    /// Resolve the project a session belongs to: its stored project id first,
    /// then a cwd path match. Shared by sidebar opens and sidebar drops so a
    /// foreign session always switches workspaces instead of mixing folders.
    pub(crate) fn target_project_for_session(&self, session_id: &str) -> Option<ProjectInfo> {
        // A pending user pick beats a possibly stale server header. `None`
        // (cleared to No project) resolves to no project immediately.
        if let Some(override_project) = self.pending_project_override.get(session_id) {
            let pid = override_project.as_ref()?;
            if let Some(p) = self.projects.iter().find(|p| &p.id == pid) {
                return Some(p.clone());
            }
            // Project gone (deleted?): fall through to normal order.
        }
        let target_session = self.sessions.iter().find(|s| s.id == session_id)?;
        if let Some(pid) = &target_session.project_id
            && let Some(p) = self.projects.iter().find(|p| &p.id == pid)
        {
            return Some(p.clone());
        }
        self.projects
            .iter()
            .find(|p| !target_session.cwd.is_empty() && p.path == target_session.cwd)
            .cloned()
    }

    /// The workspace key a session opens in: its worktree's workspace when it
    /// runs in one, else its project's. A project the user just picked in the
    /// footer wins (and means the project's own folder), like it does for
    /// `target_project_for_session`.
    pub(crate) fn target_workspace_for_session(&self, session_id: &str) -> Option<String> {
        let project = self.target_project_for_session(session_id)?;
        if self.pending_project_override.contains_key(session_id) {
            return Some(project.id);
        }
        let session = self.sessions.iter().find(|s| s.id == session_id)?;
        Some(
            match console_core::session_checkout_path(session, Some(&project.path)) {
                Some(path) => console_core::worktree_workspace_key(&project.id, path),
                None => project.id,
            },
        )
    }

    /// Reconcile a folder-change confirmation from the server: adopt the
    /// returned header as truth, and retire the pending override when the
    /// header matches the user's pick (a newer pick keeps its own entry).
    /// Only touches in-memory state on match — a stale response must never
    /// regress a newer user intent.
    pub(crate) fn confirm_project_override(&mut self, session_id: &str, header: &SessionHeader) {
        let satisfied = self
            .pending_project_override
            .get(session_id)
            .is_some_and(|ov| ov.as_deref() == header.project_id.as_deref());
        if !satisfied {
            return;
        }
        self.pending_project_override.remove(session_id);
        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        {
            session.project_id = header.project_id.clone();
            session.cwd = header.cwd.clone();
        }
    }

    /// Remember which pane holds focus for the current workspace, so splits
    /// keep their focus across workspace switches.
    pub(crate) fn remember_active_pane(&mut self) {
        if let Some(pane_id) = self.active_pane_id.clone() {
            self.project_active_panes
                .insert(self.selected_project_id.clone(), pane_id);
        }
    }

    /// Pane to focus now that `workspace_root` holds `project_id`'s tree: that
    /// workspace's remembered pane when still present, else the current pane
    /// when present, else the first leaf.
    pub(crate) fn restore_active_pane(&mut self, project_id: &Option<String>) {
        let remembered = self
            .project_active_panes
            .get(project_id)
            .cloned()
            .filter(|id| {
                self.workspace_root
                    .leaves()
                    .iter()
                    .any(|leaf| &leaf.id == id)
            });
        let current = self.active_pane_id.clone().filter(|id| {
            self.workspace_root
                .leaves()
                .iter()
                .any(|leaf| &leaf.id == id)
        });
        let first = self.workspace_root.first_leaf().map(|leaf| leaf.id.clone());
        self.active_pane_id = remembered.or(current).or(first);
    }

    /// Put the app in the workspace `id`'s chat belongs to — its project's, or
    /// its worktree's — stashing the workspace it leaves and restoring (or
    /// creating) the target. Returns whether the workspace changed. Does not
    /// open the chat tab or load messages; callers do that themselves.
    pub(crate) fn switch_workspace_for_session(
        &mut self,
        id: &str,
        active_pane_id: &str,
        cx: &mut Context<Self>,
    ) -> bool {
        let target_session = self.sessions.iter().find(|s| s.id == id).cloned();
        let target_project = self.target_project_for_session(id);
        // The workspace key: the project id, or project + worktree path.
        let target_project_id = self.target_workspace_for_session(id);

        // A workspace switch occurs if target_project_id differs from current project,
        // or if switching from an unassociated workspace into a project when tabs are already open.
        let is_different_project = match (&self.selected_project_id, &target_project_id) {
            (Some(current), Some(target)) => current != target,
            (Some(_), None) => true,
            (None, Some(_)) => self
                .workspace_root
                .leaves()
                .iter()
                .any(|l| !l.tabs.is_empty()),
            (None, None) => false,
        };

        let tab_id = format!("chat:{id}");
        if is_different_project {
            // A chat lives in exactly one workspace: drop stale copies from
            // the current tree before stashing so the old folder doesn't keep
            // the moved tab, and from every other cached workspace.
            console_ui::workspace::ops::close_matching_tabs(&mut self.workspace_root, |t| {
                t.id() == tab_id
            });
            for (key, root) in self.project_workspace_roots.iter_mut() {
                if key != &target_project_id {
                    console_ui::workspace::ops::close_matching_tabs(root, |t| t.id() == tab_id);
                }
            }
            // Remember this workspace's focused pane before leaving it.
            self.remember_active_pane();
            // Save current project's workspace tabs
            self.project_workspace_roots.insert(
                self.selected_project_id.clone(),
                self.workspace_root.clone(),
            );

            // Switch active project
            self.selected_project_id = target_project_id.clone();
            if let Some(state) = self.workspace_pane_states.get_mut(active_pane_id) {
                state.selected_project_id = target_project_id.clone();
                Rc::make_mut(&mut state.branches).clear();
                state.branch_loaded = target_project.is_none();
                state.branch_is_git_repository = false;
            }

            // Restore previously opened tabs for this project, or start a clean leaf
            if let Some(saved_root) = self
                .project_workspace_roots
                .get(&target_project_id)
                .cloned()
            {
                self.workspace_root = saved_root;
            } else {
                self.workspace_root = console_core::WorkspaceNode::leaf(active_pane_id);
            }
            // Back on this workspace's remembered split focus before opening.
            self.restore_active_pane(&target_project_id);
            // Heal legacy mixed saves and keep any existing copy's folder in step.
            console_ui::workspace::ops::retain_project_tabs(
                &mut self.workspace_root,
                &target_project_id,
            );
            console_ui::workspace::ops::set_tab_project(
                &mut self.workspace_root,
                &tab_id,
                target_project_id.clone(),
            );

            // Refresh git branch info for the new project — against the
            // session's own folder, which for a worktree chat is its worktree.
            if let Some(project) = &target_project {
                let cwd = target_session
                    .as_ref()
                    .map(|s| s.cwd.clone())
                    .filter(|cwd| !cwd.is_empty())
                    .unwrap_or_else(|| project.path.clone());
                self.reload_branches_for_pane(active_pane_id.to_string(), cwd, cx);
            }
            self.persist_layout();
            self.persist_workspaces();
        } else if self.selected_project_id.is_none() && target_project_id.is_some() {
            self.selected_project_id = target_project_id.clone();
            if let Some(state) = self.workspace_pane_states.get_mut(active_pane_id) {
                state.selected_project_id = target_project_id.clone();
            }
            self.persist_layout();
            self.persist_workspaces();
        } else {
            // Same workspace: drop foreign tabs and keep the reopened tab's
            // folder in step so close/reopen can't strand it in the wrong place.
            console_ui::workspace::ops::retain_project_tabs(
                &mut self.workspace_root,
                &self.selected_project_id,
            );
            console_ui::workspace::ops::set_tab_project(
                &mut self.workspace_root,
                &tab_id,
                target_project_id.clone(),
            );
            // A reopened No-project chat must not inherit a drifted pane
            // project: pin pane + global to explicit None and persist.
            if target_project_id.is_none() {
                let mut changed = false;
                if self.pane_project_id(active_pane_id).is_some() {
                    if let Some(state) = self.workspace_pane_states.get_mut(active_pane_id) {
                        state.selected_project_id = None;
                        std::rc::Rc::make_mut(&mut state.branches).clear();
                        state.branch_loaded = true;
                        state.branch_is_git_repository = false;
                    }
                    changed = true;
                }
                if self.selected_project_id.is_some() {
                    self.selected_project_id = None;
                    changed = true;
                }
                if changed {
                    self.persist_layout();
                    self.persist_workspaces();
                }
            }
        }

        is_different_project
    }

    pub fn select_and_open_session(&mut self, id: String, cx: &mut Context<Self>) {
        super::macos_notifications::clear_for_session(&id);
        let active_pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let prev_sid = self
            .active_session_for_pane(&active_pane_id)
            .map(|s| s.to_string());

        // The chat this pane is showing keeps its position: the new one
        // replaces its transcript.
        if let Some(prev_sid) = prev_sid.as_deref() {
            self.save_transcript_scroll_position_for_pane(&active_pane_id, prev_sid, cx);
        }

        let target_session = self.sessions.iter().find(|s| s.id == id).cloned();
        let is_different_project = self.switch_workspace_for_session(&id, &active_pane_id, cx);

        self.selected_session_id = Some(id.clone());
        let title = target_session
            .as_ref()
            .map(|s| s.display_title().to_string())
            .unwrap_or_else(|| "Chat".to_string());
        self.open_chat_tab(id.clone(), title);

        if !is_different_project && prev_sid.as_deref() == Some(&id) {
            // Already on screen, so nothing reloads. Opening it from a banner
            // is the user looking at the result, so report the read here: the
            // loader is what reports it for every other way of opening a chat.
            self.report_session_read(&id, cx);
            cx.notify();
            return;
        }

        self.sidebar_draft_ids.remove(&id);
        self.restore_draft_for_pane(&active_pane_id, Some(&id), cx);
        self.active_transcript_view().update(cx, |t, cx| {
            t.set_messages(Vec::new(), cx);
            t.clear_session_changes(cx);
        });
        self.load_session_messages(id, cx);
        cx.notify();
    }

    /// Visible sidebar session ids top-to-bottom, mirroring the sidebar
    /// render: sessions with confirmed drafts live in Drafts (excluded here)
    /// and collapsed date groups are skipped.
    pub fn visible_session_ids(&self) -> Vec<String> {
        let draft_ids: HashSet<String> = self
            .draft_summaries()
            .into_iter()
            .filter_map(|d| d.session_id)
            .collect();
        let timestamp = |index: usize| {
            let session = &self.sessions[index];
            if draft_ids.contains(&session.id) {
                0
            } else {
                session.sort_timestamp()
            }
        };
        let is_live_pinned = |index: usize| {
            let session = &self.sessions[index];
            session.pinned_at.is_some() && !draft_ids.contains(&session.id)
        };
        let is_searching =
            self.sidebar_search_active && !self.sidebar_search_query.trim().is_empty();
        if is_searching {
            let query_lower = self.sidebar_search_query.trim().to_lowercase();
            let matches_query = |s: &SessionHeader| {
                query_lower.is_empty()
                    || s.display_title().to_lowercase().contains(&query_lower)
                    || s.title.to_lowercase().contains(&query_lower)
            };
            let mut ids = Vec::new();
            for (index, session) in self.sessions.iter().enumerate() {
                if is_live_pinned(index) && matches_query(session) {
                    ids.push(session.id.clone());
                }
            }
            match self.sidebar_sort_mode {
                SidebarSortMode::Date => {
                    let grouped = group_indices_by_date(self.sessions.len(), timestamp);
                    for (_, positions) in grouped {
                        for idx in positions {
                            let session = &self.sessions[idx];
                            if !is_live_pinned(idx) && matches_query(session) {
                                ids.push(session.id.clone());
                            }
                        }
                    }
                }
                SidebarSortMode::Project => {
                    let grouped = group_indices_by_project(
                        self.sessions.len(),
                        |index| {
                            let session = &self.sessions[index];
                            self.projects
                                .iter()
                                .find(|project| project.matches_session(session))
                                .map(|project| project.id.clone())
                        },
                        timestamp,
                    );
                    for (_, positions) in grouped {
                        for idx in positions {
                            let session = &self.sessions[idx];
                            if !is_live_pinned(idx) && matches_query(session) {
                                ids.push(session.id.clone());
                            }
                        }
                    }
                }
            }
            for session in self.sessions.iter() {
                if session.settled_at.is_some()
                    && matches_query(session)
                    && !ids.contains(&session.id)
                {
                    ids.push(session.id.clone());
                }
            }
            return ids;
        }
        let mut ids = Vec::new();
        if !self.pinned_collapsed {
            for (index, session) in self.sessions.iter().enumerate() {
                // Settled chats are hidden from the sidebar, so they take no
                // slot in the cmd+1-9 mapping either.
                if is_live_pinned(index) && session.settled_at.is_none() {
                    ids.push(session.id.clone());
                }
            }
        }
        match self.sidebar_sort_mode {
            SidebarSortMode::Date => {
                let grouped = group_indices_by_date(self.sessions.len(), timestamp);
                for (group, positions) in grouped {
                    if self.collapsed_groups.contains(&group) {
                        continue;
                    }
                    for idx in positions {
                        let session = &self.sessions[idx];
                        if !draft_ids.contains(&session.id)
                            && !is_live_pinned(idx)
                            && session.settled_at.is_none()
                        {
                            ids.push(session.id.clone());
                        }
                    }
                }
            }
            SidebarSortMode::Project => {
                let grouped = group_indices_by_project(
                    self.sessions.len(),
                    |index| {
                        let session = &self.sessions[index];
                        self.projects
                            .iter()
                            .find(|project| project.matches_session(session))
                            .map(|project| project.id.clone())
                    },
                    timestamp,
                );
                for (key, positions) in grouped {
                    let collapse_key = key.unwrap_or_else(|| "none".to_string());
                    if self.collapsed_projects.contains(&collapse_key) {
                        continue;
                    }
                    for idx in positions {
                        let session = &self.sessions[idx];
                        if !draft_ids.contains(&session.id)
                            && !is_live_pinned(idx)
                            && session.settled_at.is_none()
                        {
                            ids.push(session.id.clone());
                        }
                    }
                }
            }
        }
        ids
    }

    /// Browser-tab style jump: 0..7 go to nth visible session, index 8 goes
    /// to the last one when there are more than nine. No-op when empty.
    pub fn select_session_by_visible_index(&mut self, index: usize, cx: &mut Context<Self>) {
        let ids = self.visible_session_ids();
        if ids.is_empty() {
            return;
        }
        let target = if index == 8 && ids.len() > 9 {
            ids.last().cloned()
        } else {
            ids.get(index).cloned()
        };
        if let Some(id) = target {
            self.select_and_open_session(id, cx);
        }
    }

    pub fn load_session_messages(&mut self, session_id: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        self.load_session_messages_for_pane(pane_id, session_id, cx);
    }

    /// Fork the pane's session through one assistant message into a new idle
    /// chat sharing cwd/worktree, then open the fork in that pane. No-op
    /// while the session is running (the server rejects active runs).
    pub fn fork_session_for_pane(
        &mut self,
        pane_id: String,
        message_id: String,
        cx: &mut Context<Self>,
    ) {
        let Some(session_id) = self.active_session_for_pane(&pane_id) else {
            return;
        };
        if self.is_session_running(&session_id) {
            return;
        }
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            match client.sessions.fork(&session_id, &message_id).await {
                Ok(header) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let new_id = header.id.clone();
                                Rc::make_mut(&mut this.sessions).insert(0, header);
                                this.active_pane_id = Some(pane_id.clone());
                                this.select_and_open_session(new_id, cx);
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to fork session: {error}");
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

    /// The window just came to the front: the chat in the active pane is now in
    /// front of the user, so whatever finished while they were away is seen.
    ///
    /// The read is forced rather than gated on the local list. A run that ended
    /// while the window was in the background made the chat unread on the
    /// server, but the app only heard "the run ended" and still holds
    /// `unread: false`, so a gated report would skip it. One idempotent request
    /// per window focus is cheap, and the server ignores it for a read chat.
    pub(crate) fn report_read_for_active_chat(&mut self, cx: &mut Context<Self>) {
        let Some(session_id) = self
            .active_pane_id
            .as_deref()
            .and_then(|pane| self.active_session_for_pane(pane))
        else {
            return;
        };
        self.force_session_read(&session_id, cx);
    }

    /// Tell the server this chat was shown, which clears its unread dot (and a
    /// lingering needs-attention) on every device. The local header is cleared
    /// first so the dot does not linger until the next list refresh, and the
    /// call is skipped for a chat already read, so reopening a quiet chat
    /// costs no request. Best-effort: a failed report only means the dot
    /// clears on the next refresh instead.
    pub(crate) fn report_session_read(&mut self, session_id: &str, cx: &mut Context<Self>) {
        let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        else {
            return;
        };
        if !console_core::types::notification::needs_read_report(session) {
            return;
        }
        session.unread = false;
        // A needs-attention chat with no run behind it settles to done on the
        // server when read. A chat that is genuinely waiting on a question
        // keeps the alert: its run is still active, and the next refresh says
        // so.
        if session.status == "needs_attention" && !self.running_sessions.contains_key(session_id) {
            session.status = "done".to_string();
        }
        super::macos_notifications::clear_for_session(session_id);
        self.refresh_dock_badge();
        cx.notify();

        let client = self.client.clone();
        let session_id = session_id.to_string();
        cx.spawn(async move |_entity, _cx| {
            if let Err(error) = client.sessions.mark_read(&session_id).await {
                log::warn!("mark read failed for {session_id}: {error}");
            }
        })
        .detach();
    }

    /// Report a read for a chat that just finished while on screen, after a
    /// short beat. The server writes the run's final status and its activity
    /// time slightly after the stream's end event, and a read that lands first
    /// would be overwritten by that activity and leave the chat unread.
    pub(crate) fn schedule_read_after_finish(&mut self, session_id: &str, cx: &mut Context<Self>) {
        let session_id = session_id.to_string();
        cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(600))
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Still the chat in front of the user, in a focused window.
                        let still_viewing = this.is_window_active
                            && this
                                .active_pane_id
                                .as_deref()
                                .and_then(|pane| this.active_session_for_pane(pane))
                                .as_deref()
                                == Some(session_id.as_str());
                        if still_viewing {
                            // The local header is stale: the list has not been
                            // refreshed since the run ended, so force the call.
                            this.force_session_read(&session_id, cx);
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Like [`Self::report_session_read`] but always calls the server: the
    /// local header may not know yet that the run just made the chat unread.
    fn force_session_read(&mut self, session_id: &str, cx: &mut Context<Self>) {
        if let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        {
            session.unread = false;
        }
        super::macos_notifications::clear_for_session(session_id);
        self.refresh_dock_badge();
        cx.notify();
        let client = self.client.clone();
        let session_id = session_id.to_string();
        cx.spawn(
            async move |entity, cx| match client.sessions.mark_read(&session_id).await {
                Ok(()) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.load_sessions(cx));
                        }
                    });
                }
                Err(error) => log::warn!("mark read failed for {session_id}: {error}"),
            },
        )
        .detach();
    }

    /// The row menu's read-state entry: clears an unread chat, or brings the
    /// dot back on a read one.
    pub(crate) fn toggle_session_read(&mut self, session_id: String, cx: &mut Context<Self>) {
        let unread = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .is_some_and(|session| session.unread);
        if unread {
            self.report_session_read(&session_id, cx);
        } else {
            self.mark_session_unread(session_id, cx);
        }
    }

    /// Make a read chat unread again ("Mark as unread" in the row menu). The
    /// dot appears at once; a server refusal (a running or settled chat)
    /// rolls it back and shows the reason.
    pub(crate) fn mark_session_unread(&mut self, session_id: String, cx: &mut Context<Self>) {
        let Some(session) = Rc::make_mut(&mut self.sessions)
            .iter_mut()
            .find(|session| session.id == session_id)
        else {
            return;
        };
        if session.unread {
            return;
        }
        session.unread = true;
        self.refresh_dock_badge();
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            if let Err(error) = client.sessions.mark_unread(&session_id).await {
                let message = format!("Unable to mark as unread: {error}");
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(session) = Rc::make_mut(&mut this.sessions)
                                .iter_mut()
                                .find(|session| session.id == session_id)
                            {
                                session.unread = false;
                            }
                            this.refresh_dock_badge();
                            this.set_error_for_session(&session_id, message, cx)
                        });
                    }
                });
            }
        })
        .detach();
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
        self.pull_draft(session_id.clone(), cx);
        self.report_session_read(&session_id, cx);
        let client = self.client.clone();
        let saved_position = self.transcript_scroll_positions.get(&session_id).cloned();
        cx.spawn(async move |entity, cx| {
            // Fetched together and applied in one update, so the transcript
            // is measured once with its change chips instead of re-measuring
            // the last row when the changes land later.
            let (detail, changes) = futures::join!(
                client.sessions.get(
                    &session_id,
                    Some(console_core::SessionPageOptions {
                        limit: Some(50),
                        before: None,
                    }),
                ),
                client.sessions.get_changes_without_diff(&session_id),
            );
            let changes = changes
                .map_err(|err| log::warn!("Failed to fetch turn changes: {}", err))
                .ok();
            match detail {
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
                                if this.active_inspector_target().0.as_deref()
                                    == Some(session_id.as_str())
                                {
                                    this.fetch_inspector_session_changes(cx);
                                }
                                transcript.update(cx, |t, cx| {
                                    if let Some(changes) = changes {
                                        t.set_session_changes(Rc::new(changes), cx);
                                    }
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
                                    let pane_for_prompts = pane_id.clone();
                                    let app_for_prompts = app_entity_for_older.clone();
                                    t.set_on_request_prompts(move |_window, cx| {
                                        if let Some(app) = app_for_prompts.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.load_prompts_for_pane(
                                                    pane_for_prompts.clone(),
                                                    cx,
                                                );
                                            });
                                        }
                                    });
                                    let pane_for_jump = pane_id.clone();
                                    let app_for_jump = app_entity_for_older.clone();
                                    t.set_on_jump_to_prompt(move |from_end, _window, cx| {
                                        if let Some(app) = app_for_jump.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.jump_to_prompt_for_pane(
                                                    pane_for_jump.clone(),
                                                    from_end,
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
                                            position.message_id.as_deref(),
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

                                if detail.header.status == "working"
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

                                // Shells started before this window attached.
                                this.load_background_work(session_id.clone(), cx);

                                this.load_suggestions(session_id.clone(), cx);

                                // Hydrate any staged next-turn prompt so a queue
                                // set before restart or on another device shows
                                // immediately without waiting for queueUpdated SSE.
                                let queue_client = client.clone();
                                let queue_pane_id = pane_id.clone();
                                let queue_session_id = session_id.clone();
                                cx.spawn(async move |entity, cx| {
                                    if let Ok(q) = queue_client
                                        .runs
                                        .get_queued_prompts(&queue_session_id)
                                        .await
                                    {
                                        let _ = cx.update(|cx| {
                                            if let Some(app) = entity.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    if this
                                                        .active_session_for_pane(&queue_pane_id)
                                                        .as_deref()
                                                        != Some(queue_session_id.as_str())
                                                    {
                                                        return;
                                                    }
                                                    this.set_queue_from_server(
                                                        &queue_session_id,
                                                        q,
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
