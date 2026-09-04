use console_core::types::git::GitBranchInfo;
use console_core::{ApprovalMode, SelectedModel, TodoItem, WorkspaceTabConfig};
use console_ui::chat::TranscriptView;
use console_ui::input::{ComposerEvent, ComposerInput};
use console_ui::model_picker::PickerTab;
use console_ui::primitives::menu::ContextMenuHandle;
use console_ui::terminal::TerminalView;
use console_ui::workspace::{WorkspaceDrag, ops as workspace_ops};
use gpui::{AppContext, Context, Entity, Window};
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
                        app.update(cx, |this, cx| {
                            this.approval_menu.close(window, cx);
                            if let PickerTab::Provider(name) = this.pane_picker_tab(&pane_id_owned)
                            {
                                this.load_models_for_provider(&name, cx);
                            }
                        });
                    }
                    search.update(cx, |input, cx| input.clear(cx));
                    let focus = search.read(cx).focus();
                    let weak = entity.clone();
                    window.on_next_frame(move |window, _| {
                        window.on_next_frame(move |window, cx| {
                            let still_open = weak
                                .upgrade()
                                .is_some_and(|app| app.read(cx).model_menu.is_open());
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
        transcript_view.update(cx, |transcript, _| {
            transcript.set_on_preview_image({
                let entity = entity.clone();
                move |image, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.preview_image_data(image, cx);
                        });
                    }
                }
            });
            transcript.set_on_open_file({
                let entity = entity.clone();
                let pane_id = pane_id_owned.clone();
                move |link, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.open_file_link(link, &pane_id, cx);
                        });
                    }
                }
            });
        });
        let submit_pane_id = pane_id.to_string();
        let edit_pane_id = pane_id.to_string();
        self._subscriptions.push(cx.subscribe(
            &composer_input,
            move |this, input, event: &ComposerEvent, cx| match event {
                ComposerEvent::Submit(prompt) => {
                    this.active_pane_id = Some(submit_pane_id.clone());
                    this.selected_session_id = this.active_session_for_pane(&submit_pane_id);
                    let attachments = (*this.attachments_for_pane(&submit_pane_id)).clone();
                    this.submit_prompt(prompt.clone(), attachments, cx);
                }
                ComposerEvent::Edited => {
                    // Save raw text for crash safety; does NOT update sidebar_draft_ids.
                    let text = input.read(cx).content().to_string();
                    let session_id = this.active_session_for_pane(&edit_pane_id);
                    this.save_draft_for_session(session_id.as_deref(), &text);
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
        self._subscriptions.push(cx.subscribe(
            &question_input,
            move |this, _input, event: &ComposerEvent, cx| match event {
                // Typing is repainted by the input entity itself.
                ComposerEvent::Edited => {}
                ComposerEvent::Focus => cx.notify(),
                ComposerEvent::Submit(answer) if !answer.trim().is_empty() => {
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
                model_menu,
                approval_menu,
                selected_project_id: self.selected_project_id.clone(),
                branches: self.branches.clone(),
                branch_loaded: self.branch_loaded,
                branch_is_git_repository: self.branch_is_git_repository,
                branch_pending: self.branch_pending,
                project_menu,
                branch_menu,
                model_search,
                loaded_session_id: None,
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

    pub(crate) fn set_pane_approval_mode(&mut self, pane_id: &str, mode: ApprovalMode) {
        if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
            state.approval_mode = mode;
        }
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
    pub fn open_chat_tab(
        &mut self,
        session_id: impl Into<String>,
        title: impl Into<String>,
    ) -> String {
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
        let tab = WorkspaceTabConfig::Chat {
            session_id: session_id.clone(),
            title: title.into(),
            project_id: self.pane_project_id(pane_id),
        };
        workspace_ops::open_tab(&mut self.workspace_root, pane_id, tab);
        self.active_pane_id = Some(pane_id.to_string());
        format!("chat:{session_id}")
    }

    /// Open a new Terminal tab in the active pane. The PTY cwd is the pane's
    /// selected project path, falling back to the process working directory.
    pub fn open_terminal_tab(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        let cwd = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
            .or_else(|| {
                std::env::current_dir()
                    .map(|p| p.to_string_lossy().to_string())
                    .ok()
            })
            .unwrap_or_else(|| ".".to_string());
        self.open_terminal_tab_in_pane(&pane_id, cwd, window, cx);
    }

    /// Open a Terminal tab running at `cwd` in a specific pane.
    pub fn open_terminal_tab_in_pane(
        &mut self,
        pane_id: &str,
        cwd: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let terminal_id = format!(
            "term-{}-{}",
            chrono::Utc::now().timestamp_millis(),
            self.terminals.len(),
        );
        let view = cx.new(|cx| TerminalView::with_cwd(cwd, self.client.clone(), window, cx));
        self.terminals.insert(terminal_id.clone(), view);
        let tab = WorkspaceTabConfig::Terminal {
            terminal_id: terminal_id.clone(),
            title: "Terminal".into(),
            project_id: self.pane_project_id(pane_id),
        };
        workspace_ops::open_tab(&mut self.workspace_root, pane_id, tab);
        self.active_pane_id = Some(pane_id.to_string());
        cx.notify();
    }

    /// Resolve a clicked transcript file link against the session cwd with a
    /// pane project-path fallback, then open it as a workspace tab. Phase 1
    /// opens the file only; `:line:col` suffixes are stripped by the resolver.
    pub fn open_file_link(&mut self, link: String, pane_id: &str, cx: &mut Context<Self>) {
        let cwd = self.transcript_for_pane(pane_id).read(cx).session_cwd();
        let project_path = self
            .selected_project_for_pane(pane_id)
            .map(|project| project.path.clone());
        let resolved = console_ui::markdown::file_links::resolve_file_link(
            &link,
            cwd.as_deref(),
            project_path.as_deref(),
        );
        self.open_file_tab_in_pane(pane_id, resolved, cx);
    }

    pub fn open_file_tab(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        self.open_file_tab_in_pane(&pane_id, path, cx);
    }

    pub fn open_file_tab_in_pane(&mut self, pane_id: &str, path: String, cx: &mut Context<Self>) {
        let title = std::path::Path::new(&path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&path)
            .to_string();

        let preview_target = self.preview_tab.as_ref().and_then(|(tab_id, opened_at)| {
            if opened_at.elapsed() < std::time::Duration::from_secs(600) {
                Some(tab_id.clone())
            } else {
                None
            }
        });

        let tab = WorkspaceTabConfig::File {
            path: path.clone(),
            title,
            project_id: self.pane_project_id(pane_id),
        };

        let new_tab_id = workspace_ops::replace_or_open_tab(
            &mut self.workspace_root,
            pane_id,
            preview_target.as_deref(),
            tab,
        );

        let created_at = self
            .preview_tab
            .as_ref()
            .filter(|(tid, _)| preview_target.as_deref() == Some(tid.as_str()))
            .map(|(_, instant)| *instant)
            .unwrap_or_else(std::time::Instant::now);

        self.preview_tab = Some((new_tab_id, created_at));
        self.active_pane_id = Some(pane_id.to_string());
        self.inspector_selected_path = Some(path.clone());

        // Fetch file content if not cached
        let client = self.client.clone();
        let file_path = path.clone();
        cx.spawn(
            async move |entity, cx| match client.fs.read_file(&file_path).await {
                Ok(resp) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.open_file_contents.insert(file_path, resp.content);
                                cx.notify();
                            });
                        }
                    });
                }
                Err(err) => {
                    log::warn!("Failed to read file for tab {}: {}", file_path, err);
                }
            },
        )
        .detach();

        cx.notify();
    }

    pub fn open_diff_tab(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        self.open_diff_tab_in_pane(&pane_id, path, cx);
    }

    pub fn open_diff_tab_in_pane(&mut self, pane_id: &str, path: String, cx: &mut Context<Self>) {
        let title = format!(
            "Diff: {}",
            std::path::Path::new(&path)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or(&path)
        );

        let preview_target = self.preview_tab.as_ref().and_then(|(tab_id, opened_at)| {
            if opened_at.elapsed() < std::time::Duration::from_secs(600) {
                Some(tab_id.clone())
            } else {
                None
            }
        });

        let tab = WorkspaceTabConfig::Diff {
            path: path.clone(),
            title,
            project_id: self.pane_project_id(pane_id),
        };

        let new_tab_id = workspace_ops::replace_or_open_tab(
            &mut self.workspace_root,
            pane_id,
            preview_target.as_deref(),
            tab,
        );

        let created_at = self
            .preview_tab
            .as_ref()
            .filter(|(tid, _)| preview_target.as_deref() == Some(tid.as_str()))
            .map(|(_, instant)| *instant)
            .unwrap_or_else(std::time::Instant::now);

        self.preview_tab = Some((new_tab_id, created_at));
        self.active_pane_id = Some(pane_id.to_string());
        self.inspector_selected_path = Some(path.clone());

        let client = self.client.clone();
        let file_path = path.clone();
        let cwd = self
            .selected_session_id
            .as_deref()
            .and_then(|id| self.sessions.iter().find(|s| s.id == id))
            .map(|s| s.cwd.clone())
            .or_else(|| {
                self.selected_project_id
                    .as_deref()
                    .and_then(|id| self.projects.iter().find(|p| p.id == id))
                    .map(|p| p.path.clone())
            });

        cx.spawn(async move |entity, cx| {
            let mut diff_raw = String::new();
            if let Ok(resp) = client.git.get_diff(cwd.as_deref(), Some(&file_path)).await {
                diff_raw = resp.diff;
            }

            let diff_result = if !diff_raw.trim().is_empty() {
                console_core::utils::diff::parse_unified_diff(&diff_raw)
            } else {
                console_core::DiffResult::default()
            };

            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.open_diff_contents
                            .insert(file_path, (diff_result, diff_raw));
                        cx.notify();
                    });
                }
            });
        })
        .detach();

        cx.notify();
    }

    /// Close a tab in a pane. Returns the newly active tab id, if any.
    pub fn close_workspace_tab(&mut self, pane_id: &str, tab_id: &str) -> Option<String> {
        workspace_ops::close_tab(&mut self.workspace_root, pane_id, tab_id)
    }

    /// Close every tab matching `predicate` across all panes.
    pub fn close_matching_workspace_tabs(
        &mut self,
        predicate: impl Fn(&WorkspaceTabConfig) -> bool,
    ) {
        workspace_ops::close_matching_tabs(&mut self.workspace_root, predicate);
    }

    /// Activate a tab in a pane.
    pub fn select_workspace_tab(&mut self, pane_id: &str, tab_id: &str) {
        workspace_ops::select_tab(&mut self.workspace_root, pane_id, tab_id);
        self.active_pane_id = Some(pane_id.to_string());
    }

    pub fn focus_workspace_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if self
            .workspace_root
            .leaves()
            .into_iter()
            .all(|leaf| leaf.id != pane_id)
        {
            return;
        }
        self.active_pane_id = Some(pane_id.to_string());
        self.selected_session_id = self.active_session_for_pane(pane_id);
        cx.notify();
    }

    /// Close a split pane and collapse its parent into the remaining sibling.
    pub fn close_workspace_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if !workspace_ops::close_pane(&mut self.workspace_root, pane_id) {
            return;
        }
        self.workspace_pane_states.remove(pane_id);
        self.todo_items.remove(pane_id);
        if self.active_pane_id.as_deref() == Some(pane_id) {
            let next_pane_id = self.workspace_root.first_leaf().map(|leaf| leaf.id.clone());
            self.active_pane_id = next_pane_id.clone();
            self.selected_session_id = next_pane_id
                .as_deref()
                .and_then(|id| self.active_session_for_pane(id));
            if let Some(next_pane_id) = next_pane_id {
                let transcript = self.transcript_for_pane(&next_pane_id);
                let composer = self.composer_for_pane(&next_pane_id);
                composer.update(cx, |input, cx| input.set_content("", cx));
                if let Some(session_id) = self.selected_session_id.clone() {
                    self.load_session_messages_for_pane(next_pane_id, session_id, cx);
                } else {
                    transcript.update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
                }
            }
        }
        cx.notify();
    }

    /// Move a dragged tab into the existing target pane as a new tab.
    pub fn move_workspace_tab_to_pane(
        &mut self,
        target_pane_id: String,
        drag: WorkspaceDrag,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.save_transcript_scroll_position(cx);
        let source_pane_id = drag.source_pane_id.clone();
        let tab = drag.tab.clone();
        let Some(target_pane_id) = workspace_ops::move_tab_to_pane(
            &mut self.workspace_root,
            source_pane_id.clone().as_deref(),
            &target_pane_id,
            drag.tab,
        ) else {
            return;
        };

        if source_pane_id.as_deref() != Some(target_pane_id.as_str()) {
            if let Some(source_pane_id) = source_pane_id {
                if let Some(source_session_id) = self.active_session_for_pane(&source_pane_id) {
                    let source_transcript = self.transcript_for_pane(&source_pane_id);
                    source_transcript.update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
                    self.load_session_messages_for_pane(source_pane_id, source_session_id, cx);
                } else if self
                    .workspace_root
                    .leaves()
                    .iter()
                    .any(|leaf| leaf.id == source_pane_id)
                {
                    self.transcript_for_pane(&source_pane_id)
                        .update(cx, |transcript, cx| {
                            transcript.set_messages(Vec::new(), cx);
                        });
                }
            }
        }

        self.active_pane_id = Some(target_pane_id.clone());
        self.ensure_workspace_pane_state(&target_pane_id, window, cx);
        if let WorkspaceTabConfig::Chat { session_id, .. } = tab {
            self.selected_session_id = Some(session_id.clone());
            self.composer_for_pane(&target_pane_id)
                .update(cx, |input, cx| {
                    input.set_prompt_history(Vec::new(), cx);
                });
            self.transcript_for_pane(&target_pane_id)
                .update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
            self.load_session_messages_for_pane(target_pane_id, session_id, cx);
        }
        cx.notify();
    }

    /// Move a dragged tab into a new horizontal split beside the target pane.
    pub fn move_workspace_tab_to_split(
        &mut self,
        target_pane_id: String,
        drag: WorkspaceDrag,
        insert_left: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.save_transcript_scroll_position(cx);
        let source_pane_id = drag.source_pane_id.clone();
        let tab = drag.tab.clone();
        let Some(new_pane_id) = workspace_ops::move_tab_to_split(
            &mut self.workspace_root,
            source_pane_id.as_deref(),
            &target_pane_id,
            drag.tab,
            insert_left,
        ) else {
            return;
        };

        if let Some(source_pane_id) = source_pane_id {
            if let Some(source_session_id) = self.active_session_for_pane(&source_pane_id) {
                let source_transcript = self.transcript_for_pane(&source_pane_id);
                source_transcript.update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
                self.load_session_messages_for_pane(source_pane_id, source_session_id, cx);
            } else if self
                .workspace_root
                .leaves()
                .iter()
                .any(|leaf| leaf.id == source_pane_id)
            {
                self.transcript_for_pane(&source_pane_id)
                    .update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
            }
        }

        self.ensure_workspace_pane_state(&new_pane_id, window, cx);
        self.active_pane_id = Some(new_pane_id.clone());
        if let WorkspaceTabConfig::Chat { session_id, .. } = tab {
            self.selected_session_id = Some(session_id.clone());
            let draft = self
                .get_draft_for_session(Some(&session_id))
                .map(|s| s.to_string());
            self.composer_for_pane(&new_pane_id)
                .update(cx, |input, cx| {
                    input.set_prompt_history(Vec::new(), cx);
                    if let Some(draft_text) = draft {
                        input.set_content(draft_text, cx);
                    } else {
                        input.clear(cx);
                    }
                });
            self.transcript_for_pane(&new_pane_id)
                .update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
            self.load_session_messages_for_pane(new_pane_id, session_id, cx);
        }
        cx.notify();
    }
}
