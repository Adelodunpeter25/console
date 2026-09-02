//! Prompt submission and the streaming agent run: pushing the optimistic
//! user message, resolving/creating the session, consuming the SSE event
//! stream into the transcript, and settling canonical state afterwards.

use std::rc::Rc;
use std::time::Duration;

use console_core::{
    AgentMessage, AgentSessionEvent, CreateSessionDto, ImageAttachment, RunPromptDto,
};
use futures_util::StreamExt;
use gpui::Context;

use super::ConsoleDesktopApp;
use super::user_prompt_history;

impl ConsoleDesktopApp {
    /// Publish streaming transcript changes at a bounded cadence instead of
    /// repainting once per provider chunk. The underlying transcript state is
    /// still updated immediately; only the UI notification is coalesced.
    fn schedule_stream_render(&mut self, pane_id: String, cx: &mut Context<Self>) {
        const STREAM_RENDER_INTERVAL: Duration = Duration::from_millis(33);

        if self.stream_render_pending_for_pane(&pane_id) {
            return;
        }
        self.set_stream_render_pending_for_pane(&pane_id, true);
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            cx.background_executor().timer(STREAM_RENDER_INTERVAL).await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.set_stream_render_pending_for_pane(&pane_id, false);
                        this.transcript_for_pane(&pane_id)
                            .update(cx, |transcript, cx| {
                                transcript.flush_stream_render(cx);
                            });
                    });
                }
            });
        })
        .detach();
    }

    pub fn submit_prompt(
        &mut self,
        prompt: String,
        attachments: Vec<ImageAttachment>,
        cx: &mut Context<Self>,
    ) {
        if prompt.trim().is_empty() {
            return;
        }
        let run_pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        // Fresh slate for this pane only: its active chat's error, or the
        // app-level banner if that is what is showing.
        self.clear_error_for_pane(&run_pane_id, cx);
        let active_session = self.active_session_for_pane(&run_pane_id).map(|s| s.to_string());
        self.clear_draft_for_session(active_session.as_deref());
        if let Some(ref sid) = active_session {
            self.revoke_sidebar_draft(sid);
        }
        self.composer_for_pane(&run_pane_id)
            .update(cx, |input, cx| {
                input.record_prompt_history(prompt.clone(), cx);
            });

        // Push user message bubble
        let user_msg = AgentMessage::User {
            content: prompt.clone(),
            attachments: if attachments.is_empty() {
                None
            } else {
                Some(attachments.clone())
            },
            created_at: Some(chrono::Utc::now().timestamp()),
        };
        self.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
            t.push_message(user_msg, cx);
            t.begin_streaming(cx);
        });

        let selected_model = self.pane_selected_model(&run_pane_id);
        let model_id = selected_model.as_ref().map(|m| m.model_id.clone());
        let provider = selected_model.as_ref().map(|m| m.provider.clone());
        let approval_mode = Some(self.pane_approval_mode(&run_pane_id).value().to_string());
        let active_sid = self.active_session_for_pane(&run_pane_id);
        let session_project_id = self.pane_project_id(&run_pane_id);
        let session_cwd = self
            .selected_project_for_pane(&run_pane_id)
            .map(|project| project.path.clone());
        let client = self.client.clone();

        self.composer_for_pane(&run_pane_id)
            .update(cx, |input, cx| {
                input.set_content("", cx);
            });
        // Clear only this pane's staged attachments; other splits keep their
        // chips for their own next prompt.
        self.set_attachments_for_pane(&run_pane_id, Vec::new());
        // Capture the run's start time once. Running state is keyed by session
        // id so it stays attached to the owning chat across pane switches. For
        // an existing chat we know the session id now; for a brand-new chat the
        // id is resolved after creation inside the spawned run.
        let run_started_at = chrono::Utc::now().timestamp();
        if let Some(ref sid) = active_sid {
            self.set_session_running(sid, Some(run_started_at));
        }
        cx.notify();

        // Spawn async streaming run
        cx.spawn(async move |entity, cx| {
            // Resolve or create session ID
            let session_id = match active_sid {
                Some(id) => id,
                None => {
                    match client.sessions.create(CreateSessionDto {
                        cwd: session_cwd,
                        project_id: session_project_id,
                        model_id: model_id.clone(),
                        provider: provider.clone(),
                        title: Some(prompt.chars().take(30).collect()),
                        approval_mode: approval_mode.clone(),
                    }).await {
                        Ok(s) => {
                            let sid = s.id.clone();
                            let new_session = s;
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.save_transcript_scroll_position(cx);
                                        this.apply_session_header_for_pane(&run_pane_id, &new_session, cx);
                                        if this.active_pane_id.as_deref() == Some(run_pane_id.as_str()) {
                                            this.selected_session_id = Some(sid.clone());
                                        }
                                        Rc::make_mut(&mut this.sessions).insert(0, new_session.clone());
                                        this.open_chat_tab_in_pane(&run_pane_id, sid.clone(), new_session.title.clone());
                                        // Now that the new session exists, mark it
                                        // running under its real id. For an existing
                                        // chat this was already set at submit time.
                                        this.set_session_running(&sid, Some(run_started_at));
                                        cx.notify();
                                    });
                                }
                            });
                            sid
                        }
                        Err(error) => {
                            let message = format!("Unable to create a session: {error}");
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.set_error(message, cx);
                                        this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                            t.finish_streaming(cx);
                                        });
                                    });
                                }
                            });
                            return;
                        }
                    }
                }
            };

            // The session that owns this run. All run-derived state below is
            // keyed by this id so a pane switching to another chat never
            // receives this run's streamed text, working status, permissions,
            // questions, todos, or notices.
            let run_session_id = session_id.clone();

            let run_dto = RunPromptDto {
                prompt,
                model_id,
                provider,
                approval_mode,
                attachments: if attachments.is_empty() { None } else { Some(attachments) },
            };

            let mut stream = match client.runs.stream_prompt(&session_id, run_dto).await {
                Ok(stream) => stream,
                Err(error) => {
                    let message = format!("Unable to start agent run: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_session_running(&run_session_id, None);
                                this.set_error_for_session(&run_session_id, message, cx);
                                this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                    t.finish_streaming(cx);
                                });
                            });
                        }
                    });
                    return;
                }
            };

            let mut server_run_may_be_active = false;
            while let Some(event_res) = stream.next().await {
                    let still_running = cx.update(|cx| {
                        entity.upgrade().map(|app| app.read(cx).is_session_running(&run_session_id)).unwrap_or(false)
                    });

                    if !still_running {
                        break;
                    }

                    match event_res {
                        Ok(event) => cx.update(|cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    if !this.is_session_running(&run_session_id) {
                                        return;
                                    }
                                    // The pane's transcript is a single entity
                                    // reused across chats loaded into that pane.
                                    // Only paint it when the pane is still showing
                                    // the session that owns this run; otherwise the
                                    // streamed text would leak onto whichever chat
                                    // the user switched to. Session-keyed state
                                    // (permissions, questions, todos, notices) is
                                    // still recorded unconditionally so it survives
                                    // the switch and resurfaces when the user
                                    // returns to this chat.
                                    let pane_shows_run = this
                                        .active_session_for_pane(&run_pane_id)
                                        .as_deref()
                                        == Some(run_session_id.as_str());
                                    let defer_render = matches!(
                                        &event,
                                        AgentSessionEvent::ModelStreamPart { part }
                                            if part.tool_call.is_none()
                                                && (part.text.is_some() || part.thinking.is_some())
                                    );
                                    this.process_agent_event(&run_session_id, &run_pane_id, event, cx);
                                    if defer_render && pane_shows_run {
                                        this.schedule_stream_render(run_pane_id.clone(), cx);
                                    } else {
                                        cx.notify();
                                    }
                                });
                            }
                        }),
                        Err(error) => {
                            let message = format!("Agent stream error: {error}");
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        // Keep the composer disabled while the backend
                                        // finishes the run that outlived this stream.
                                        this.set_session_running(&run_session_id, Some(run_started_at));
                                        this.set_error_for_session(&run_session_id, message, cx);
                                        if this
                                            .active_session_for_pane(&run_pane_id)
                                            .as_deref()
                                            == Some(run_session_id.as_str())
                                        {
                                            this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                                t.finish_streaming(cx);
                                            });
                                        }
                                    });
                                }
                            });
                            break;
                        }                }
            }

            // Replace optimistic client timestamps with canonical messages after
            // the backend has finished the run. This also handles an SSE parse
            // failure: the server intentionally continues the run after the
            // client disconnects, so we must wait before allowing another prompt.
            match client.sessions.wait_until_settled(&session_id).await {
                Ok(detail) => {
                    server_run_may_be_active = detail.header.status == Some(console_core::SessionStatus::Working);
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str()) {
                                    this.apply_session_header_for_pane(&run_pane_id, &detail.header, cx);
                                    this.composer_for_pane(&run_pane_id).update(cx, |input, cx| {
                                        input.set_prompt_history(
                                            user_prompt_history(&detail.messages),
                                            cx,
                                        );
                                    });
                                    this.save_transcript_scroll_position_for_pane(
                                        &run_pane_id,
                                        &session_id,
                                        cx,
                                    );
                                    this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                        t.set_messages(detail.messages, cx);
                                    });
                                }
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to refresh canonical message times: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_error_for_session(&session_id, message, cx)
                            });
                        }
                    });
                }
            }

            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let pane_shows_run = this
                            .active_session_for_pane(&run_pane_id)
                            .as_deref()
                            == Some(run_session_id.as_str());
                        if server_run_may_be_active {
                            this.set_session_running(&run_session_id, Some(run_started_at));
                            this.set_error_for_session(
                                &run_session_id,
                                "The server is still completing this run. Use Stop before sending another prompt.",
                                cx,
                            );
                            // The canonical set_messages above reset the
                            // transcript's streaming flag; re-engage the
                            // in-pane WorkingIndicator since the server is
                            // still working this run.
                            if pane_shows_run {
                                this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                    t.resume_streaming(run_started_at, cx);
                                });
                            }
                        } else {
                            this.set_session_running(&run_session_id, None);
                            // Errors are never blanket-cleared here: a banner set
                            // mid-run (or by another chat) must survive settling;
                            // banners dismiss themselves after five seconds.
                            if pane_shows_run {
                                this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                    t.finish_streaming(cx);
                                });
                            }
                        }
                        cx.notify();
                    });
                }
            });
        }).detach();
    }

    /// Process a single incoming agent run event for a given session and pane.
    fn process_agent_event(
        &mut self,
        run_session_id: &str,
        run_pane_id: &str,
        event: AgentSessionEvent,
        cx: &mut Context<Self>,
    ) {
        let pane_shows_run = self
            .active_session_for_pane(run_pane_id)
            .as_deref()
            == Some(run_session_id);

        match event {
            AgentSessionEvent::ModelStreamPart { part } => {
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        if let Some(text) = &part.text {
                            t.append_assistant_text(text, cx);
                        }
                        if let Some(thinking) = &part.thinking {
                            t.append_assistant_thinking(thinking, cx);
                        }
                        if let Some(preview) = part.tool_call {
                            t.upsert_assistant_tool_call(
                                console_core::ToolCall {
                                    id: preview.id,
                                    name: preview.name,
                                    arguments: preview.arguments.unwrap_or(serde_json::Value::Null),
                                    thought_signature: preview.thought_signature,
                                },
                                cx,
                            );
                        }
                    });
                }
            }
            AgentSessionEvent::ModelStreamEnd { turn, .. } => {
                if let Some(turn) = turn {
                    if pane_shows_run {
                        self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                            t.finalize_assistant_message(turn, cx);
                        });
                    }
                }
            }
            AgentSessionEvent::PermissionRequest { request } => {
                self.set_pending_permission_for_session(run_session_id, Some(request));
                self.set_pending_question_for_session(run_session_id, None);
            }
            AgentSessionEvent::AskQuestion { request } => {
                self.set_pending_question_for_session(run_session_id, Some(request));
                self.set_pending_permission_for_session(run_session_id, None);
                self.clear_question_selected_for_session(run_session_id);
                self.clear_question_inputs_for_session(run_session_id, cx);
            }
            AgentSessionEvent::ToolExecutionStart { calls } => {
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        t.upsert_assistant_tool_calls(calls, cx);
                    });
                }
            }
            AgentSessionEvent::ToolExecutionResult { result } => {
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        t.append_tool_results(vec![result], cx);
                    });
                }
            }
            AgentSessionEvent::ToolExecutionEnd { results } => {
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        t.append_tool_results(results, cx);
                    });
                }
                if self.right_sidebar_visible {
                    self.refresh_inspector(cx);
                }
            }
            AgentSessionEvent::TodoUpdate { items, .. } => {
                self.set_todo_items_for_session(run_session_id, items);
            }
            AgentSessionEvent::SubagentStart {
                subagent_id,
                parent_tool_call_id,
                name,
                role,
                prompt,
                max_turns,
            } => {
                let list = self
                    .session_subagents
                    .entry(run_session_id.to_string())
                    .or_insert_with(|| Rc::new(Vec::new()));
                let list_mut = Rc::make_mut(list);
                if let Some(existing) = list_mut.iter_mut().find(|s| s.subagent_id == subagent_id) {
                    existing.name = name;
                    existing.role = role;
                    existing.prompt = prompt;
                    existing.max_turns = max_turns;
                    existing.status = "running".to_string();
                } else {
                    list_mut.push(console_core::types::SubagentInfo {
                        subagent_id,
                        parent_tool_call_id,
                        name,
                        role,
                        prompt,
                        max_turns,
                        current_turn: 1,
                        status: "running".to_string(),
                        summary: None,
                        error: None,
                        activities: Vec::new(),
                    });
                }
            }
            AgentSessionEvent::SubagentActivity {
                subagent_id,
                turn_index,
                tool_call_id,
                tool_name,
                args,
                status,
                error,
            } => {
                if let Some(list) = self.session_subagents.get_mut(run_session_id) {
                    let list_mut = Rc::make_mut(list);
                    if let Some(subagent) = list_mut.iter_mut().find(|s| s.subagent_id == subagent_id) {
                        subagent.current_turn = subagent.current_turn.max(turn_index);
                        if let Some(act) = subagent.activities.iter_mut().find(|a| a.tool_call_id == tool_call_id) {
                            act.status = status;
                            if error.is_some() {
                                act.error = error;
                            }
                        } else {
                            subagent.activities.push(console_core::types::SubagentActivityItem {
                                turn_index,
                                tool_call_id,
                                tool_name,
                                args,
                                status,
                                error,
                            });
                        }
                    }
                }
            }
            AgentSessionEvent::SubagentEnd {
                subagent_id,
                status,
                summary,
                error,
                total_turns,
            } => {
                if let Some(list) = self.session_subagents.get_mut(run_session_id) {
                    let list_mut = Rc::make_mut(list);
                    if let Some(subagent) = list_mut.iter_mut().find(|s| s.subagent_id == subagent_id) {
                        subagent.status = status;
                        if summary.is_some() {
                            subagent.summary = summary;
                        }
                        if error.is_some() {
                            subagent.error = error;
                        }
                        if total_turns > 0 {
                            subagent.current_turn = total_turns;
                        }
                    }
                }
            }
            AgentSessionEvent::Compaction { summary, .. } => {
                self.set_agent_notice_for_session(
                    run_session_id,
                    Some(if summary.trim().is_empty() {
                        "Conversation context was compacted.".to_string()
                    } else {
                        format!("Context compacted: {}", summary)
                    }),
                );
            }
            AgentSessionEvent::TurnStart { .. } => {
                self.set_agent_notice_for_session(run_session_id, None);
            }
            AgentSessionEvent::TurnEnd { .. } => {
                self.set_pending_permission_for_session(run_session_id, None);
                self.set_pending_question_for_session(run_session_id, None);
                self.clear_question_selected_for_session(run_session_id);
                if self.right_sidebar_visible {
                    self.refresh_inspector(cx);
                }
            }
            AgentSessionEvent::SessionEnd => {
                self.set_session_running(run_session_id, None);
                self.set_pending_permission_for_session(run_session_id, None);
                self.set_pending_question_for_session(run_session_id, None);
                self.clear_question_selected_for_session(run_session_id);
                self.clear_question_inputs_for_session(run_session_id, cx);
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        t.finish_streaming(cx);
                    });
                }
                if self.right_sidebar_visible {
                    self.refresh_inspector(cx);
                }
            }
            AgentSessionEvent::SessionStart => {
                self.set_agent_notice_for_session(run_session_id, None);
            }
            AgentSessionEvent::Error { error } => {
                self.set_session_running(run_session_id, None);
                self.set_error_for_session(
                    run_session_id,
                    format!("Agent error: {}", error.message),
                    cx,
                );
                if pane_shows_run {
                    self.transcript_for_pane(run_pane_id).update(cx, |t, cx| {
                        t.finish_streaming(cx);
                    });
                }
            }
            _ => {}
        }
    }

    /// Attach to an in-flight server run for this session (re-attach).
    pub fn attach_session_run_for_pane(
        &mut self,
        pane_id: String,
        session_id: String,
        cx: &mut Context<Self>,
    ) {
        if self.is_session_running(&session_id) {
            return;
        }

        let now = chrono::Utc::now().timestamp_millis();
        self.set_session_running(&session_id, Some(now));

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let run_pane_id = pane_id.clone();
        let run_session_id = session_id.clone();

        if self.active_session_for_pane(&pane_id).as_deref() == Some(session_id.as_str()) {
            self.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                t.resume_streaming(now, cx);
            });
        }

        cx.spawn(async move |_, cx| {
            let mut stream = match client.runs.attach_run_stream(&session_id, None).await {
                Ok(stream) => stream,
                Err(_) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_session_running(&run_session_id, None);
                                if this.active_session_for_pane(&run_pane_id).as_deref()
                                    == Some(run_session_id.as_str())
                                {
                                    this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                        t.finish_streaming(cx);
                                    });
                                }
                                cx.notify();
                            });
                        }
                    });
                    return;
                }
            };

            while let Some(event_res) = stream.next().await {
                let still_running = cx.update(|cx| {
                    entity
                        .upgrade()
                        .map(|app| app.read(cx).is_session_running(&run_session_id))
                        .unwrap_or(false)
                });

                if !still_running {
                    break;
                }

                match event_res {
                    Ok(event) => {
                        cx.update(|cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    if !this.is_session_running(&run_session_id) {
                                        return;
                                    }
                                    let pane_shows_run = this
                                        .active_session_for_pane(&run_pane_id)
                                        .as_deref()
                                        == Some(run_session_id.as_str());
                                    let defer_render = matches!(
                                        &event,
                                        AgentSessionEvent::ModelStreamPart { part }
                                            if part.tool_call.is_none()
                                                && (part.text.is_some() || part.thinking.is_some())
                                    );

                                    this.process_agent_event(&run_session_id, &run_pane_id, event, cx);

                                    if defer_render && pane_shows_run {
                                        this.schedule_stream_render(run_pane_id.clone(), cx);
                                    } else {
                                        cx.notify();
                                    }
                                });
                            }
                        });
                    }
                    Err(_) => {
                        break;
                    }
                }
            }

            if let Ok(detail) = client.sessions.wait_until_settled(&session_id).await {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_session_running(&run_session_id, None);
                            if this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str()) {
                                this.apply_session_header_for_pane(&run_pane_id, &detail.header, cx);
                                this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                    t.set_messages(detail.messages, cx);
                                    t.finish_streaming(cx);
                                });
                            }
                            cx.notify();
                        });
                    }
                });
            } else {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_session_running(&run_session_id, None);
                            if this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str()) {
                                this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                                    t.finish_streaming(cx);
                                });
                            }
                            cx.notify();
                        });
                    }
                });
            }
        }).detach();
    }
}
