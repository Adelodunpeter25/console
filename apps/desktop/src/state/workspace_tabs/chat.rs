use console_core::types::git::GitBranchInfo;
use console_core::{
    ApprovalMode, SelectedModel, ThinkingLevel, TodoItem, UpdateSessionDto, WorkspaceTabConfig,
};
use console_ui::chat::TranscriptView;
use console_ui::input::{ComposerAttachmentPaste, ComposerEvent, ComposerInput};
use console_ui::model_picker::PickerTab;
use console_ui::primitives::menu::ContextMenuHandle;
use console_ui::workspace::ops as workspace_ops;
use gpui::{AppContext, Context, Entity, Focusable as _, Window};
use std::rc::Rc;

use crate::state::app::ConsoleDesktopApp;
use crate::types::WorkspacePaneState;

impl ConsoleDesktopApp {
    pub(crate) fn ensure_workspace_pane_state(
        &mut self,
        pane_id: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.workspace_pane_states.contains_key(pane_id) {
            return;
        }

        let transcript_view = cx.new(|cx| TranscriptView::new(cx));
        let composer_input = cx.new(|cx| ComposerInput::new(window, cx));
        let question_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Type your answer...")
        });
        let model_search = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Search models...")
        });
        let entity = cx.entity().downgrade();
        let pane_id_owned = pane_id.to_string();
        let model_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            let search = model_search.clone();
            let pane_id_owned = pane_id_owned.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = pane_id_owned.clone();
                        cx.defer(move |cx| {
                            if let Some(window) = cx.active_window() {
                                let _ = window.update(cx, |_, window, cx| {
                                    app.update(cx, |this, cx| {
                                        this.approval_menu.close(window, cx);
                                        if let PickerTab::Provider(name) =
                                            this.pane_picker_tab(&pane_id)
                                        {
                                            this.load_models_for_provider(&name, cx);
                                        }
                                    });
                                });
                            }
                        });
                    }
                    search.update(cx, |input, cx| input.clear(cx));
                    let focus = search.read(cx).focus();
                    let weak = entity.clone();
                    let pane_id_owned = pane_id_owned.clone();
                    window.on_next_frame(move |window, _| {
                        window.on_next_frame(move |window, cx| {
                            let still_open = weak.upgrade().is_some_and(|app| {
                                app.read(cx).pane_model_menu(&pane_id_owned).is_open()
                            });
                            if still_open {
                                window.focus(&focus, cx);
                            }
                        });
                    });
                }
            }
        });
        let approval_menu = ContextMenuHandle::new(cx);
        let project_menu = ContextMenuHandle::new(cx);
        let branch_menu = ContextMenuHandle::new(cx);
        let usage_pane_id = pane_id.to_string();
        let usage_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, _window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = usage_pane_id.clone();
                        app.update(cx, |this, cx| {
                            let session_id = this.active_session_for_pane(&pane_id).unwrap_or_default();
                            this.maybe_fetch_usage(&session_id, cx);
                        });
                    }
                }
            }
        });
        transcript_view.update(cx, |transcript, _| {
            super::super::transcript_wiring::wire_preview_image(transcript, entity.clone());
            super::super::transcript_wiring::wire_open_file_for_pane(
                transcript,
                entity.clone(),
                pane_id_owned.clone(),
            );
        });
        let submit_pane_id = pane_id.to_string();
        let edit_pane_id = pane_id.to_string();
        let steer_pane_id = pane_id.to_string();
        self._subscriptions.push(cx.subscribe(
            &composer_input,
            move |this, input, event: &ComposerEvent, cx| match event {
                ComposerEvent::Submit(prompt, context_files) => {
                    let context_files = context_files.clone();
                    if this.is_active_session_running_for_pane(&submit_pane_id) {
                        let attachments = (*this.attachments_for_pane(&submit_pane_id)).clone();
                        this.queue_prompt_for_pane(
                            submit_pane_id.clone(),
                            prompt.clone(),
                            attachments,
                            context_files.clone(),
                            cx,
                        );
                        return;
                    }
                    this.active_pane_id = Some(submit_pane_id.clone());
                    this.selected_session_id = this.active_session_for_pane(&submit_pane_id);
                    let attachments = (*this.attachments_for_pane(&submit_pane_id)).clone();
                    this.submit_prompt_with_context(prompt.clone(), attachments, context_files, cx);
                }
                ComposerEvent::SubmitSteer(prompt, context_files) => {
                    this.submit_steer_for_pane(
                        steer_pane_id.clone(),
                        prompt.clone(),
                        context_files.clone(),
                        cx,
                    );
                }
                ComposerEvent::Edited => {
                    // Save raw text for crash safety; does NOT update sidebar_draft_ids.
                    let input = input.read(cx);
                    let text = input.content().to_string();
                    let mentions = input.mentions().to_vec();
                    let session_id = this.active_session_for_pane(&edit_pane_id);
                    let attachments = this.attachments_for_pane(&edit_pane_id);
                    this.save_draft_for_session_with_context(
                        session_id.as_deref(),
                        &text,
                        &mentions,
                        &[],
                        &attachments,
                        cx,
                    );
                }
                _ => {}
            },
        ));
        self._subscriptions.push(cx.subscribe(
            &model_search,
            |_this, _input, event: &ComposerEvent, cx| match event {
                ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        ));
        let question_pane_id = pane_id.to_string();
        let paste_pane_id = pane_id.to_string();
        self._subscriptions.push(cx.subscribe(
            &composer_input,
            move |this, _input, event: &ComposerAttachmentPaste, cx| {
                this.stage_clipboard_attachments(&paste_pane_id, event.0.clone(), cx);
            },
        ));
        self._subscriptions.push(cx.subscribe(
            &question_input,
            move |this, _input, event: &ComposerEvent, cx| match event {
                // Typing is repainted by the input entity itself.
                ComposerEvent::Edited => {}
                ComposerEvent::Focus => cx.notify(),
                ComposerEvent::Submit(answer, _) if !answer.trim().is_empty() => {
                    if let Some(session_id) = this.active_session_for_pane(&question_pane_id) {
                        this.answer_pending_question_for_session(
                            session_id,
                            serde_json::Value::String(answer.trim().to_owned()),
                            cx,
                        );
                    }
                }
                _ => {}
            },
        ));
        self.workspace_pane_states.insert(
            pane_id.to_string(),
            WorkspacePaneState {
                transcript_view,
                composer_input,
                question_input,
                selected_model: self.selected_model.clone(),
                active_picker_tab: self.active_picker_tab.clone(),
                approval_mode: self.approval_mode,
                approval_mode_history: Vec::new(),
                thinking_level: self.thinking_level,
                model_menu,
                approval_menu,
                usage_menu,
                selected_project_id: self.selected_project_id.clone(),
                branches: self.branches.clone(),
                branch_loaded: self.branch_loaded,
                branch_is_git_repository: self.branch_is_git_repository,
                branch_pending: self.branch_pending,
                project_menu,
                branch_menu,
                model_search,
                loaded_session_id: None,
                tab_strip_follow: console_ui::workspace::TabStripFollow::new(),
            },
        );
    }

    pub(crate) fn transcript_for_pane(&self, pane_id: &str) -> Entity<TranscriptView> {
        if pane_id == "pane-main" {
            self.transcript_view.clone()
        } else {
            self.workspace_pane_states
                .get(pane_id)
                .map(|state| state.transcript_view.clone())
                .unwrap_or_else(|| self.transcript_view.clone())
        }
    }

    pub(crate) fn composer_for_pane(&self, pane_id: &str) -> Entity<ComposerInput> {
        if pane_id == "pane-main" {
            self.composer_input.clone()
        } else {
            self.workspace_pane_states
                .get(pane_id)
                .map(|state| state.composer_input.clone())
                .unwrap_or_else(|| self.composer_input.clone())
        }
    }

    pub(crate) fn question_input_for_pane(&self, pane_id: &str) -> Entity<ComposerInput> {
        if pane_id == "pane-main" {
            self.question_input.clone()
        } else {
            self.workspace_pane_states
                .get(pane_id)
                .map(|state| state.question_input.clone())
                .unwrap_or_else(|| self.question_input.clone())
        }
    }

    /// Clear the answer fields of every pane currently showing `session_id`.
    /// Scoped so clearing one chat's answered question never wipes text being
    /// typed into another chat's question card.
    pub(crate) fn clear_question_inputs_for_session(
        &mut self,
        session_id: &str,
        cx: &mut Context<Self>,
    ) {
        let tab_id = format!("chat:{session_id}");
        let pane_ids: Vec<String> = self
            .workspace_root
            .leaves()
            .iter()
            .filter(|leaf| leaf.active_tab_id.as_deref() == Some(tab_id.as_str()))
            .map(|leaf| leaf.id.clone())
            .collect();
        for pane_id in &pane_ids {
            self.question_input_for_pane(pane_id)
                .update(cx, |input, cx| input.clear(cx));
        }
    }

    pub(crate) fn active_transcript_view(&self) -> Entity<TranscriptView> {
        self.active_pane_id
            .as_deref()
            .map(|pane_id| self.transcript_for_pane(pane_id))
            .unwrap_or_else(|| self.transcript_view.clone())
    }

    pub(crate) fn active_composer_input(&self) -> Entity<ComposerInput> {
        self.active_pane_id
            .as_deref()
            .map(|pane_id| self.composer_for_pane(pane_id))
            .unwrap_or_else(|| self.composer_input.clone())
    }

    pub(crate) fn pane_selected_model(&self, pane_id: &str) -> Option<SelectedModel> {
        self.workspace_pane_states
            .get(pane_id)
            .and_then(|state| state.selected_model.clone())
    }

    /// The model the composer picker shows for a pane: the chat's own
    /// model. Run payloads use the same value.
    pub(crate) fn effective_model_for_pane(&self, pane_id: &str) -> Option<SelectedModel> {
        self.pane_selected_model(pane_id)
    }

    pub(crate) fn pane_picker_tab(&self, pane_id: &str) -> PickerTab {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.active_picker_tab.clone())
            .unwrap_or_else(|| self.active_picker_tab.clone())
    }

    pub(crate) fn pane_approval_mode(&self, pane_id: &str) -> ApprovalMode {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.approval_mode)
            .unwrap_or(self.approval_mode)
    }

    pub(crate) fn pane_project_id(&self, pane_id: &str) -> Option<String> {
        self.workspace_pane_states
            .get(pane_id)
            .and_then(|state| state.selected_project_id.clone())
            .or_else(|| self.selected_project_id.clone())
    }

    pub(crate) fn pane_branches(&self, pane_id: &str) -> Rc<Vec<GitBranchInfo>> {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.branches.clone())
            .unwrap_or_else(|| self.branches.clone())
    }

    pub(crate) fn pane_branch_loaded(&self, pane_id: &str) -> bool {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.branch_loaded)
            .unwrap_or(self.branch_loaded)
    }

    pub(crate) fn pane_is_git_repository(&self, pane_id: &str) -> bool {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.branch_is_git_repository)
            .unwrap_or(self.branch_is_git_repository)
    }

    pub(crate) fn pane_branch_pending(&self, pane_id: &str) -> bool {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.branch_pending)
            .unwrap_or(self.branch_pending)
    }

    pub(crate) fn pane_model_menu(&self, pane_id: &str) -> ContextMenuHandle {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.model_menu.clone())
            .unwrap_or_else(|| self.model_menu.clone())
    }

    /// The model picker search field for a pane. Always present: the main pane
    /// and every additional pane each own a `ComposerInput` entity so the query
    /// and focus survive the dropdown's per-frame rebuild.
    pub(crate) fn pane_model_search(&self, pane_id: &str) -> Entity<ComposerInput> {
        self.workspace_pane_states
            .get(pane_id)
            .or_else(|| self.workspace_pane_states.get("pane-main"))
            .or_else(|| self.workspace_pane_states.values().next())
            .map(|state| state.model_search.clone())
            .expect("model search field is always present in workspace pane states")
    }

    pub(crate) fn pane_approval_menu(&self, pane_id: &str) -> ContextMenuHandle {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.approval_menu.clone())
            .unwrap_or_else(|| self.approval_menu.clone())
    }

    pub(crate) fn pane_project_menu(&self, pane_id: &str) -> ContextMenuHandle {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.project_menu.clone())
            .unwrap_or_else(|| self.project_menu.clone())
    }

    pub(crate) fn pane_branch_menu(&self, pane_id: &str) -> ContextMenuHandle {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.branch_menu.clone())
            .unwrap_or_else(|| self.branch_menu.clone())
    }

    pub(crate) fn pane_usage_menu(&self, pane_id: &str) -> ContextMenuHandle {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.usage_menu.clone())
            .unwrap_or_else(|| self.usage_menu.clone())
    }

    pub(crate) fn set_pane_model(&mut self, pane_id: &str, model: Option<SelectedModel>) {
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.selected_model = model;
        }
    }

    pub(crate) fn set_pane_picker_tab(&mut self, pane_id: &str, tab: PickerTab) {
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.active_picker_tab = tab;
        }
    }

    pub(crate) fn pane_thinking_level(&self, pane_id: &str) -> Option<ThinkingLevel> {
        let explicit = self
            .workspace_pane_states
            .get(pane_id)
            .and_then(|state| state.thinking_level)
            .or(self.thinking_level);

        if explicit.is_some() {
            return explicit;
        }

        // Check active session header
        if let Some(session_id) = self.active_session_for_pane(pane_id) {
            if let Some(session) = self.sessions.iter().find(|s| s.id == session_id) {
                if session.thinking_level.is_some() {
                    return session.thinking_level;
                }
            }
        }

        // Default based on active model
        let supported = self.supported_thinking_levels_for_pane(pane_id);
        if supported.is_empty() {
            None
        } else if supported.contains(&ThinkingLevel::Low) {
            Some(ThinkingLevel::Low)
        } else {
            supported.first().copied()
        }
    }

    pub(crate) fn set_pane_thinking_level(&mut self, pane_id: &str, level: Option<ThinkingLevel>) {
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.thinking_level = level;
        } else {
            self.thinking_level = level;
        }
    }

    pub(crate) fn supported_thinking_levels_for_pane(&self, pane_id: &str) -> Vec<ThinkingLevel> {
        let Some(selected) = self.pane_selected_model(pane_id) else {
            return Vec::new();
        };

        // Check live fetched models first
        if let Some(models) = self.models_by_provider.get(&selected.provider) {
            if let Some(m) = models.iter().find(|m| m.id == selected.model_id) {
                if let Some(ref levels) = m.supported_thinking_levels {
                    if !levels.is_empty() {
                        return levels.clone();
                    }
                }
            }
        }

        // Check static catalog models
        if let Some(entry) = self.providers.iter().find(|p| p.name == selected.provider) {
            if let Some(m) = entry.models.iter().find(|m| m.id == selected.model_id) {
                if let Some(ref levels) = m.supported_thinking_levels {
                    if !levels.is_empty() {
                        return levels.clone();
                    }
                }
            }
        }

        // Standard defaults per provider
        match selected.provider.to_ascii_lowercase().as_str() {
            "claude" => vec![
                ThinkingLevel::Low,
                ThinkingLevel::Medium,
                ThinkingLevel::High,
                ThinkingLevel::XHigh,
                ThinkingLevel::Max,
            ],
            "codex" | "openai" => vec![
                ThinkingLevel::None,
                ThinkingLevel::Minimal,
                ThinkingLevel::Low,
                ThinkingLevel::Medium,
                ThinkingLevel::High,
                ThinkingLevel::XHigh,
                ThinkingLevel::Max,
            ],
            "antigravity" | "google" => vec![
                ThinkingLevel::Minimal,
                ThinkingLevel::Low,
                ThinkingLevel::Medium,
                ThinkingLevel::High,
            ],
            _ => vec![
                ThinkingLevel::Low,
                ThinkingLevel::Medium,
                ThinkingLevel::High,
            ],
        }
    }

    /// Step to the next supported thinking level for the pane's active model.
    pub fn cycle_thinking_level_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let supported = self.supported_thinking_levels_for_pane(pane_id);
        if supported.is_empty() {
            return;
        }

        let current = self
            .pane_thinking_level(pane_id)
            .unwrap_or(supported[0]);

        let current_index = supported
            .iter()
            .position(|&lvl| lvl == current)
            .unwrap_or(0);

        let next_level = supported[(current_index + 1) % supported.len()];
        self.set_pane_thinking_level(pane_id, Some(next_level));

        self.update_session_settings_for_pane(
            pane_id.to_string(),
            console_core::UpdateSessionDto {
                title: None,
                cwd: None,
                project_id: None,
                model_id: None,
                provider: None,
                approval_mode: None,
                thinking_level: Some(next_level),
            },
            cx,
        );

        cx.notify();
    }

    /// Set a pane's approval mode, recording the outgoing mode in the MRU
    /// history first. No-ops on identical modes so history only reflects real
    /// changes. History is capped so it stays a navigation aid, not a log.
    /// Derived entirely from observed changes — never hardcoded.
    pub(crate) fn set_pane_approval_mode(&mut self, pane_id: &str, mode: ApprovalMode) {
        const HISTORY_CAP: usize = 8;
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            if state.approval_mode != mode {
                push_approval_history(&mut state.approval_mode_history, state.approval_mode);
                state.approval_mode = mode;
            }
        } else if self.approval_mode != mode {
            push_approval_history(&mut self.approval_mode_history, self.approval_mode);
            self.approval_mode = mode;
        }

        fn push_approval_history(history: &mut Vec<ApprovalMode>, outgoing: ApprovalMode) {
            history.retain(|m| *m != outgoing);
            history.push(outgoing);
            if history.len() > HISTORY_CAP {
                history.remove(0);
            }
        }
    }

    /// The pane's approval-mode MRU history, newest last. Empty until the
    /// first mode change; falls back to the global history for panes without
    /// a state entry.
    pub(crate) fn pane_approval_mode_history(&self, pane_id: &str) -> &[ApprovalMode] {
        self.workspace_pane_states
            .get(pane_id)
            .map(|state| state.approval_mode_history.as_slice())
            .unwrap_or(self.approval_mode_history.as_slice())
    }

    /// Shift+Tab from the composer: jump to the most recently used mode that
    /// isn't current (a toggle between the last two modes in practice). When
    /// history is empty or exhausted, advance one step in `ApprovalMode::ALL`
    /// order — no mode is ever hardcoded or skipped.
    pub fn cycle_approval_mode(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let current = self.pane_approval_mode(&pane_id);
        let next = self
            .pane_approval_mode_history(&pane_id)
            .iter()
            .rev()
            .copied()
            .find(|m| *m != current)
            .or_else(|| {
                let all = ApprovalMode::ALL;
                let pos = all.iter().position(|m| *m == current)?;
                Some(all[(pos + 1) % all.len()])
            });
        let Some(next) = next else { return };
        self.set_pane_approval_mode(&pane_id, next);
        self.update_session_settings_for_pane(
            pane_id,
            UpdateSessionDto {
                title: None,
                cwd: None,
                project_id: None,
                model_id: None,
                provider: None,
                approval_mode: Some(next.value().to_string()),
                thinking_level: None,
            },
            cx,
        );
        // Keep focus in the composer; the dropdown label re-renders with the
        // new mode on the next frame.
        let composer = self.active_composer_input();
        composer.update(cx, |input, cx| {
            window.focus(&input.focus_handle(cx), cx);
        });
        cx.notify();
    }

    pub(crate) fn todo_items_for_pane(&self, pane_id: &str) -> Vec<TodoItem> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.todo_items.get(&sid).cloned())
            .unwrap_or_default()
    }

    pub(crate) fn is_todos_collapsed_for_pane(&self, pane_id: &str) -> bool {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.todos_collapsed.get(&sid).copied())
            .unwrap_or(true)
    }

    pub(crate) fn toggle_todos_collapsed_for_pane(&mut self, pane_id: &str) {
        if let Some(sid) = self.active_session_for_pane(pane_id) {
            let current = self.todos_collapsed.get(&sid).copied().unwrap_or(true);
            self.todos_collapsed.insert(sid, !current);
        }
    }

    pub(crate) fn set_todo_items_for_session(&mut self, session_id: &str, items: Vec<TodoItem>) {
        if items.is_empty() {
            self.todo_items.remove(session_id);
        } else {
            self.todo_items.insert(session_id.to_string(), items);
        }
    }

    /// Whether the active session for a pane has any messages — used to lock
    /// the project/cwd selector once a chat has started, since each run reloads
    /// `header.cwd` for prompt-ref expansion and all tool paths.
    pub(crate) fn session_has_messages(&self, pane_id: &str) -> bool {
        let Some(session_id) = self.active_session_for_pane(pane_id) else {
            return false;
        };
        // Use the server-reported count from the session header. The backend
        // guard (session.service.ts) is the real safety net; this UI lock just
        // reflects reality so the user doesn't try and fail.
        self.sessions
            .iter()
            .find(|s| s.id == session_id)
            .and_then(|s| s.message_count)
            .is_some_and(|count| count > 0)
    }

    pub(crate) fn active_session_for_pane(&self, pane_id: &str) -> Option<String> {
        self.workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .and_then(|leaf| leaf.active_tab_id.as_deref())
            .and_then(|tab_id| tab_id.strip_prefix("chat:"))
            .map(str::to_owned)
    }

    /// The active leaf's tab id, for places that still want the flat view.
    pub fn active_tab_id(&self) -> Option<String> {
        let pane_id = self.active_pane_id.as_deref()?;
        self.workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .and_then(|leaf| leaf.active_tab_id.clone())
    }

    /// Open (or activate) a chat tab for a session in the active pane.
    /// If the chat is already open in any split, focuses that split instead of creating a duplicate.
    pub fn open_chat_tab(
        &mut self,
        session_id: impl Into<String>,
        title: impl Into<String>,
    ) -> String {
        let session_id = session_id.into();
        let tab_id = format!("chat:{session_id}");

        // If this chat is already open in ANY pane in the workspace tree,
        // switch to that pane and select its tab instead of duplicating it into another split.
        if let Some(existing_pane_id) = self.workspace_root.leaves().iter().find_map(|leaf| {
            if leaf.tabs.iter().any(|t| t.id() == tab_id) {
                Some(leaf.id.clone())
            } else {
                None
            }
        }) {
            workspace_ops::select_tab(&mut self.workspace_root, &existing_pane_id, &tab_id);
            self.active_pane_id = Some(existing_pane_id);
            self.persist_workspaces();
            return tab_id;
        }

        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        self.open_chat_tab_in_pane(&pane_id, session_id, title)
    }

    pub fn open_chat_tab_in_pane(
        &mut self,
        pane_id: &str,
        session_id: impl Into<String>,
        title: impl Into<String>,
    ) -> String {
        let session_id = session_id.into();
        let tab_id = format!("chat:{session_id}");

        if let Some(leaf) = self
            .workspace_root
            .leaves()
            .iter()
            .find(|l| l.id == pane_id)
        {
            if leaf.tabs.iter().any(|t| t.id() == tab_id) {
                workspace_ops::select_tab(&mut self.workspace_root, pane_id, &tab_id);
                self.active_pane_id = Some(pane_id.to_string());
                self.persist_workspaces();
                return tab_id;
            }
        }

        // A known session stamps its project and provider verbatim — including explicit
        // None (No project) — so reopening never inherits the pane's project.
        // Only truly unknown sessions fall back to the pane.
        let known_session = self
            .sessions
            .iter()
            .find(|s| s.id == session_id);
        let known_project_id = known_session.map(|s| s.project_id.clone());
        let project_id = known_project_id
            .clone()
            .unwrap_or_else(|| self.pane_project_id(pane_id));
        let provider = known_session.map(|s| s.provider.clone());
        if known_project_id.is_some() {
            if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
                state.selected_project_id = project_id.clone();
            }
        }

        let tab = WorkspaceTabConfig::Chat {
            session_id: session_id.clone(),
            title: title.into(),
            provider,
            project_id,
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        workspace_ops::open_tab(&mut self.workspace_root, pane_id, tab);
        self.active_pane_id = Some(pane_id.to_string());
        self.persist_workspaces();
        format!("chat:{session_id}")
    }
}
