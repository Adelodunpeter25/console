//! `ConsoleDesktopApp` — the single gpui entity owning all application state.
//!
//! This file keeps the struct definition and the bootstrap constructor;
//! domain handlers live in sibling modules (`projects`, `attachments`,
//! `errors`, `sessions`, `layout`, `run`) as additional `impl` blocks.

use console_core::{
    AgentMessage, ApprovalMode, AskQuestionRequest, ConsoleClient, GitBranchInfo,
    ImageAttachment, Model, ModelFavorite, PermissionRequest, ProjectInfo,
    ProviderCatalogEntry, SelectedModel, SessionHeader, TodoItem, WorkspaceNode,
    WorkspaceTabConfig,
};
use console_ui::markdown::render::TranscriptSelection;
use console_ui::utils::SessionDateGroup;
use console_ui::workspace::{WorkspaceDrag, ops as workspace_ops};
use console_ui::{
    CommandPalette, ComposerAttachmentPaste, ComposerEvent, ComposerInput, ContextMenuHandle,
    PickerTab, TranscriptView,
};
use console_ui::terminal::TerminalView;
use gpui::{AppContext, Context, Entity, ListAlignment, ListState, Subscription, Window, px};
use std::rc::Rc;

use crate::persistence;
use crate::types::WorkspacePaneState;

/// User prompts in a session, used to populate the composer's up-arrow
/// history. Shared by `sessions` and `run`.
pub(crate) fn user_prompt_history(messages: &[AgentMessage]) -> Vec<String> {
    messages
        .iter()
        .filter_map(|message| match message {
            AgentMessage::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect()
}

const SIDEBAR_DEFAULT_WIDTH: f32 = 260.0;

/// Shared with `layout` for clamping the sidebar's drag-resize width.
pub(crate) const SIDEBAR_MIN_WIDTH: f32 = 180.0;
/// Shared with `layout` for clamping the sidebar's drag-resize width.
pub(crate) const SIDEBAR_MAX_WIDTH: f32 = 520.0;

pub struct ConsoleDesktopApp {
    pub client: ConsoleClient,
    /// Shared session history for the sidebar/titlebar. `Rc` so per-frame
    /// renders clone a refcount, not the vector; mutations go through
    /// `Rc::make_mut`.
    pub sessions: Rc<Vec<SessionHeader>>,
    pub selected_session_id: Option<String>,
    /// Session currently being renamed inline in the sidebar.
    pub(crate) session_rename_id: Option<String>,
    pub(crate) session_rename_input: Entity<ComposerInput>,
    /// The workspace pane tree (tabs, splits). Starts as a single leaf.
    pub workspace_root: WorkspaceNode,
    /// The pane currently holding focus.
    pub active_pane_id: Option<String>,
    /// Shared with every pane's model picker; cloned per frame as a refcount
    /// bump.
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    /// Live-fetched models per provider, keyed by provider name. Populated
    /// lazily by `load_models_for_provider`; until a provider's entry exists,
    /// the picker falls back to the static `models` embedded in
    /// [`providers`], so the list is never blank. Shared across all panes.
    pub(crate) models_by_provider: Rc<std::collections::HashMap<String, Vec<Model>>>,
    /// Providers whose dynamic model list is mid-flight. Guards against
    /// re-firing on every render/tab-switch.
    pub(crate) loading_models: std::collections::HashSet<String>,
    pub selected_model: Option<SelectedModel>,
    pub active_picker_tab: PickerTab,
    pub favorites: Rc<std::collections::HashSet<String>>,
    pub approval_mode: ApprovalMode,
    pub model_menu: ContextMenuHandle,
    pub approval_menu: ContextMenuHandle,
    /// Shared with the sidebar and footer; cloned per frame as a refcount
    /// bump.
    pub projects: Rc<Vec<ProjectInfo>>,
    pub selected_project_id: Option<String>,
    pub branches: Rc<Vec<GitBranchInfo>>,
    pub branch_loaded: bool,
    pub branch_is_git_repository: bool,
    pub branch_pending: bool,
    pub project_menu: ContextMenuHandle,
    pub branch_menu: ContextMenuHandle,
    /// Main-pane compatibility handles; additional panes keep independent
    /// entities in `workspace_pane_states`.
    pub transcript_view: Entity<TranscriptView>,
    pub(crate) workspace_pane_states: std::collections::HashMap<String, WorkspacePaneState>,
    /// Pane-scoped slash-command and file-reference autocomplete state.
    pub(crate) autocomplete_states:
        std::collections::HashMap<String, super::autocomplete::PaneAutocompleteState>,
    /// Per-session logical transcript positions, retained while switching tabs.
    pub(crate) transcript_scroll_positions: std::collections::HashMap<
        String,
        crate::state::transcript_scroll::TranscriptScrollPosition,
    >,
    pub composer_input: Entity<ComposerInput>,
    pub question_input: Entity<ComposerInput>,
    /// Staged composer images keyed by pane. Values are `Rc` so per-frame
    /// renders clone a refcount instead of megabyte base64 payloads.
    pub attachments:
        std::collections::HashMap<String, Rc<Vec<ImageAttachment>>>,
    /// Run-derived interactive and display state. All of these are keyed by
    /// session id (not pane id) so a run's permission prompt, question, todos,
    /// and notices stay attached to the chat that owns the run. Switching a
    /// pane to another chat surfaces that chat's state instead of leaking the
    /// background run's state onto it.
    pub pending_permissions: std::collections::HashMap<String, PermissionRequest>,
    pub pending_questions: std::collections::HashMap<String, AskQuestionRequest>,
    pub question_selected: std::collections::HashMap<String, std::collections::HashSet<String>>,
    pub todo_items: std::collections::HashMap<String, Vec<TodoItem>>,
    pub agent_notices: std::collections::HashMap<String, String>,
    /// App-level error banner (not tied to any chat); shown in every pane.
    pub error_message: Option<super::errors::BannerError>,
    /// Chat-owned error banners keyed by session id (mirrors `agent_notices`),
    /// rendered only in panes whose active tab shows that session.
    pub session_errors: std::collections::HashMap<String, super::errors::BannerError>,
    pub error_selection: TranscriptSelection,
    /// Monotonic token so a stale auto-dismiss timer never clears a newer error.
    pub(crate) error_generation: u64,
    /// Decoded image shown in the image preview modal.
    pub zoomed_image: Option<std::sync::Arc<gpui::Image>>,
    /// Sessions with a locally active run, keyed by session id. The value is
    /// the Unix seconds when the run began, used by the sidebar's live
    /// Working-for label until the next canonical session refresh. A session is
    /// running iff it is present in this map. Keying by session (not pane)
    /// keeps a run's status attached to the chat that owns it, so switching a
    /// pane to a different chat never transfers the working indicator or
    /// streamed text.
    pub running_sessions: std::collections::HashMap<String, i64>,
    /// A stream burst can contain many provider chunks. The render loop uses
    /// this latch to publish at most one transcript repaint per cadence window.
    pub(crate) stream_render_pending: std::collections::HashMap<String, bool>,
    pub sidebar_visible: bool,
    pub sidebar_width: f32,
    /// Retained virtualization state for the sidebar session history.
    pub sidebar_list_state: ListState,
    /// Live drag-resize anchor, owned by `layout`.
    pub(crate) sidebar_resize_start: Option<(f32, f32)>,
    /// Active split divider drag: (split_id, direction, start_pos, start_sizes, viewport_size)
    pub(crate) split_resize: Option<(
        String,
        console_core::SplitDirection,
        gpui::Point<gpui::Pixels>,
        [f32; 2],
        gpui::Size<gpui::Pixels>,
    )>,
    /// Calendar-period groups the user collapsed in the sidebar. Shared with
    /// the sidebar; cloned per frame as a refcount bump.
    pub collapsed_groups: Rc<std::collections::HashSet<SessionDateGroup>>,
    /// Last window frame known to be on disk. Seeded from storage at startup
    /// and compared in memory each render, so unchanged frames perform no I/O.
    pub(crate) saved_window_state: Option<persistence::window::PersistedWindowState>,
    /// Window frame captured since the last flush, waiting out the persist
    /// debounce. While a drag continuously changes bounds this holds the
    /// newest frame and a single trailing timer writes it.
    pub(crate) pending_window_state: Option<persistence::window::PersistedWindowState>,
    /// ⌘K-style palette (gpui-component `Command`), opened by the sidebar
    /// search button. Entries are rebuilt each frame in `view.rs`.
    pub command_palette: Entity<CommandPalette>,
    /// Live terminal surfaces keyed by terminal id. Tabs reference these via
    /// `WorkspaceTabConfig::Terminal { terminal_id }`.
    pub terminals: std::collections::HashMap<String, Entity<TerminalView>>,
    pub auth_status: Option<console_core::types::AuthStatusResponse>,
    pub auth_logging_in: std::collections::HashSet<String>,
    pub environments: Vec<super::environments::Environment>,
    pub active_env_id: Option<String>,
    pub env_probes: std::collections::HashMap<String, console_ui::settings::ProbeState>,
    pub server_menu: console_ui::primitives::ContextMenuHandle,
    pub deleted_sessions: Vec<SessionHeader>,
    pub settings_window_handle: Option<gpui::AnyWindowHandle>,
    pub settings_window_view: Option<gpui::WeakEntity<crate::settings_window::SettingsWindow>>,
    pub drafts: std::collections::HashMap<String, crate::persistence::store::PersistedDraft>,
    pub drafts_collapsed: bool,
    pub _subscriptions: Vec<Subscription>,
}

impl ConsoleDesktopApp {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let client = ConsoleClient::new(None);
        let layout = persistence::layout::load();
        let sidebar_width = if layout.sidebar_width.is_finite() {
            layout
                .sidebar_width
                .clamp(SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH)
        } else {
            SIDEBAR_DEFAULT_WIDTH
        };
        let transcript_view = cx.new(|cx| TranscriptView::new(cx));
        let composer_input = cx.new(|cx| ComposerInput::new(window, cx));
        let question_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Type your answer...")
        });
        let session_rename_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Rename session")
        });
        // Search field inside the model picker popover. Cleared each time the
        // popover opens and focused a couple of frames later, once the deferred
        // popover subtree has joined the dispatch tree.
        let model_search = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Search models...")
        });
        let entity = cx.entity().downgrade();
        {
            let entity = entity.clone();
            transcript_view.update(cx, |transcript, _| {
                transcript.set_on_preview_image(move |image, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.preview_image_data(image, cx);
                        });
                    }
                });
            });
        }
        let model_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            let search = model_search.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.approval_menu.close(window, cx);
                            // Lazy: fetch only the active provider's live models.
                            // Static catalog stays visible as fallback, so we
                            // avoid N network calls on every open. Favorites
                            // shows static until its tab is visited.
                            if let PickerTab::Provider(name) =
                                this.pane_picker_tab("pane-main")
                            {
                                this.load_models_for_provider(&name, cx);
                            }
                        });
                    }
                    // Start each open from an empty query and put focus in the
                    // filter box. The popover content is deferred, so the input
                    // is not in the dispatch tree until two frames after the
                    // toggle — the same cadence the command palette uses.
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
        let approval_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.model_menu.close(window, cx));
                    }
                }
            }
        });
        let project_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.branch_menu.close(window, cx));
                    }
                }
            }
        });
        let branch_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.project_menu.close(window, cx));
                    }
                }
            }
        });

        let drafts = persistence::store::load_drafts();
        if let Some(initial_draft) = drafts.get("new_chat") {
            if !initial_draft.prompt.trim().is_empty() {
                composer_input.update(cx, |input, cx| {
                    input.set_content(initial_draft.prompt.clone(), cx);
                });
            }
        }

        let subscriptions = vec![
            cx.subscribe(
                &composer_input,
                |this, input, event: &ComposerEvent, cx| {
                    match event {
                        ComposerEvent::Submit(prompt) => {
                            // The main composer belongs to "pane-main" even when
                            // another split holds focus; pin the pane before
                            // submitting so attachments and run state resolve
                            // against the chat this input is mounted in.
                            this.active_pane_id = Some("pane-main".to_string());
                            this.selected_session_id =
                                this.active_session_for_pane("pane-main");
                            // Deep-copy only at the submit boundary; the Rc
                            // keeps per-frame renders cheap.
                            let attachments =
                                (*this.attachments_for_pane("pane-main")).clone();
                            this.submit_prompt(prompt.clone(), attachments, cx);
                        }
                        ComposerEvent::Edited => {
                            let text = input.read(cx).content().to_string();
                            let session_id = this.active_session_for_pane("pane-main");
                            this.save_draft_for_session(session_id.as_deref(), &text);
                        }
                        ComposerEvent::Focus => cx.notify(),
                        // Backspace on an empty composer removes the last staged
                        // attachment, the chat idiom for discarding a chip.
                        ComposerEvent::BackspaceOnEmpty if !this.attachments_for_pane(this.active_pane_id.as_deref().unwrap_or("pane-main")).is_empty() => {
                            let pane_id = this.active_pane_id.clone().unwrap_or_else(|| "pane-main".to_string());
                            if let Some(staged) = this.attachments.get_mut(&pane_id) {
                                Rc::make_mut(staged).pop();
                                if staged.is_empty() {
                                    this.attachments.remove(&pane_id);
                                }
                            }
                            cx.notify();
                        }
                        _ => {}
                    }
                },
            ),
            // Pasting an image (or image files) stages them as attachment
            // chips instead of inserting text.
            cx.subscribe(
                &composer_input,
                |this, _input, event: &ComposerAttachmentPaste, cx| {
                    this.stage_clipboard_attachments(event.0.clone(), cx);
                },
            ),
            cx.subscribe(
                &question_input,
                |this, _input, event: &ComposerEvent, cx| match event {
                    // Typing is repainted by the input entity itself; a full
                    // app render per keystroke only adds lag.
                    ComposerEvent::Edited => {}
                    ComposerEvent::Focus => cx.notify(),
                    ComposerEvent::Submit(answer) if !answer.trim().is_empty() => {
                        if let Some(session_id) = this.active_session_for_pane("pane-main") {
                            this.answer_pending_question_for_session(
                                session_id,
                                serde_json::Value::String(answer.trim().to_owned()),
                                cx,
                            );
                        }
                    }
                    _ => {}
                },
            ),
            // Re-render on every keystroke in the model picker search so the
            // filtered list follows the query. The field entity's own notify
            // only repaints the input; the list lives in the app's render.
            cx.subscribe(
                &model_search,
                |_this, _input, event: &ComposerEvent, cx| match event {
                    ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                    _ => {}
                },
            ),
        ];

        let mut app = Self {
            client,
            sessions: Rc::new(Vec::new()),
            selected_session_id: None,
            session_rename_id: None,
            session_rename_input,
            workspace_root: WorkspaceNode::leaf("pane-main"),
            active_pane_id: Some("pane-main".into()),
            providers: Rc::new(Vec::new()),
            models_by_provider: Rc::new(std::collections::HashMap::new()),
            loading_models: std::collections::HashSet::new(),
            selected_model: None,
            active_picker_tab: PickerTab::Provider("gemini".to_string()),
            favorites: Rc::new(std::collections::HashSet::new()),
            approval_mode: ApprovalMode::AlwaysAsk,
            model_menu,
            approval_menu,
            projects: Rc::new(Vec::new()),
            selected_project_id: None,
            branches: Rc::new(Vec::new()),
            branch_loaded: false,
            branch_is_git_repository: false,
            branch_pending: false,
            project_menu,
            branch_menu,
            transcript_view,
            workspace_pane_states: std::collections::HashMap::new(),
            autocomplete_states: std::collections::HashMap::new(),
            transcript_scroll_positions: std::collections::HashMap::new(),
            composer_input,
            question_input,
            attachments: std::collections::HashMap::new(),
            pending_permissions: std::collections::HashMap::new(),
            pending_questions: std::collections::HashMap::new(),
            question_selected: std::collections::HashMap::new(),
            todo_items: std::collections::HashMap::new(),
            agent_notices: std::collections::HashMap::new(),
            error_message: None,
            session_errors: std::collections::HashMap::new(),
            error_selection: TranscriptSelection::default(),
            error_generation: 0,
            zoomed_image: None,
            running_sessions: std::collections::HashMap::new(),
            stream_render_pending: std::collections::HashMap::new(),
            sidebar_visible: layout.sidebar_visible,
            sidebar_width,
            sidebar_list_state: ListState::new(0, ListAlignment::Top, px(55.0)),
            sidebar_resize_start: None,
            split_resize: None,
            saved_window_state: persistence::store::load_window(),
            pending_window_state: None,
            command_palette: cx.new(|cx| CommandPalette::new(window, cx)),
            terminals: std::collections::HashMap::new(),
            auth_status: None,
            auth_logging_in: std::collections::HashSet::new(),
            environments: Vec::new(),
            active_env_id: None,
            env_probes: std::collections::HashMap::new(),
            server_menu: console_ui::primitives::ContextMenuHandle::new(cx),
            deleted_sessions: Vec::new(),
            settings_window_handle: None,
            settings_window_view: None,
            drafts,
            drafts_collapsed: false,
            collapsed_groups: Rc::new(
                layout
                    .collapsed_groups
                    .iter()
                    .filter_map(|index| SessionDateGroup::ALL.get(*index).copied())
                    .collect(),
            ),
            _subscriptions: subscriptions,
        };

        app.init_environments(cx);
        app.refresh_auth_status(cx);
        app.init_notifications(cx);

        app.workspace_pane_states.insert(
            "pane-main".to_string(),
            WorkspacePaneState {
                transcript_view: app.transcript_view.clone(),
                composer_input: app.composer_input.clone(),
                question_input: app.question_input.clone(),
                selected_model: app.selected_model.clone(),
                active_picker_tab: app.active_picker_tab.clone(),
                approval_mode: app.approval_mode,
                model_menu: app.model_menu.clone(),
                approval_menu: app.approval_menu.clone(),
                selected_project_id: app.selected_project_id.clone(),
                branches: app.branches.clone(),
                branch_loaded: app.branch_loaded,
                branch_is_git_repository: app.branch_is_git_repository,
                branch_pending: app.branch_pending,
                project_menu: app.project_menu.clone(),
                branch_menu: app.branch_menu.clone(),
                model_search,
            },
        );

        // Bootstrap data from backend on startup. No chat is auto-selected:
        // the workspace pane starts tab-less and shows its empty state until
        // the user picks a session from the sidebar or starts a new chat.
        let client_clone = app.client.clone();
        cx.spawn(async move |entity, cx| {
            // 1. Fetch the session list for the sidebar.
            match client_clone.sessions.list(None, None).await {
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
            }

            // 2. Fetch providers and models.
            match client_clone.providers.list().await {
                Ok(providers) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let first_model = providers.first().and_then(|p| {
                                    p.models.first().map(|m| SelectedModel {
                                        provider: p.name.clone(),
                                        model_id: m.id.clone(),
                                    })
                                });
                                this.providers = Rc::new(providers);
                                // Default to first provider instead of Favorites so the
                                // popover only needs one live fetch. Favoriting
                                // stays available via the star tab, but it no
                                // longer forces N fetches on first open.
                                if let Some(first) = this.providers.first() {
                                    let first_name = first.name.clone();
                                    let needs_init = match &this.active_picker_tab {
                                        PickerTab::Favorites => true,
                                        PickerTab::Provider(name) => !this
                                            .providers
                                            .iter()
                                            .any(|p| &p.name == name),
                                    };
                                    if needs_init {
                                        this.active_picker_tab =
                                            PickerTab::Provider(first_name.clone());
                                        if let Some(state) =
                                            this.workspace_pane_states.get_mut("pane-main")
                                        {
                                            state.active_picker_tab =
                                                PickerTab::Provider(first_name);
                                        }
                                    }
                                }
                                if this.selected_model.is_none() {
                                    this.selected_model = first_model.clone();
                                }
                                if let Some(state) = this.workspace_pane_states.get_mut("pane-main")
                                {
                                    if state.selected_model.is_none() {
                                        state.selected_model = first_model;
                                    }
                                }
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load providers and models: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }

            // 3. Load model favorites persisted by the backend.
            match client_clone.model_favorites.list().await {
                Ok(model_favorites) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.favorites = Rc::new(
                                    model_favorites
                                        .into_iter()
                                        .map(|favorite: ModelFavorite| {
                                            format!("{}:{}", favorite.provider, favorite.model_id)
                                        })
                                        .collect(),
                                );
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load model favorites: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }

            // 4. Load projects; derive the selected project from the active
            // session, then fetch its Git branches for the branch chip.
            match client_clone.projects.list().await {
                Ok(projects) => {
                    let project_path = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.projects = Rc::new(projects);
                                if this.selected_project_id.is_none() {
                                    this.selected_project_id = this
                                        .sessions
                                        .iter()
                                        .find(|session| {
                                            Some(&session.id) == this.selected_session_id.as_ref()
                                        })
                                        .and_then(|session| session.project_id.clone());
                                }
                                if let Some(state) = this.workspace_pane_states.get_mut("pane-main")
                                {
                                    if state.selected_project_id.is_none() {
                                        state.selected_project_id =
                                            this.selected_project_id.clone();
                                    }
                                }
                                let path = this
                                    .selected_project_for_pane("pane-main")
                                    .map(|project| project.path.clone());
                                cx.notify();
                                path
                            })
                        } else {
                            None
                        }
                    });
                    if let Some(path) = project_path {
                        let client = client_clone.clone();
                        let entity = entity.clone();
                        cx.spawn(async move |cx| {
                            match client.git.list_branches(Some(&path)).await {
                                Ok(branches) => cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.branches =
                                                Rc::new(branches.branches.clone());
                                            this.branch_loaded = true;
                                            this.branch_is_git_repository =
                                                branches.is_git_repository;
                                            if let Some(state) =
                                                this.workspace_pane_states.get_mut("pane-main")
                                            {
                                                state.branches = Rc::new(branches.branches);
                                                state.branch_loaded = true;
                                                state.branch_is_git_repository =
                                                    branches.is_git_repository;
                                            }
                                            cx.notify();
                                        });
                                    }
                                }),
                                Err(_) => {}
                            }
                        })
                        .detach();
                    }
                }
                Err(error) => {
                    let message = format!("Unable to load projects: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }
        })
        .detach();

        // 5. Periodic polling for session list / working status sync across surfaces.
        let poll_client = app.client.clone();
        cx.spawn(async move |entity, cx| loop {
            cx.background_executor()
                .timer(std::time::Duration::from_secs(30))
                .await;
            if let Ok(sessions) = poll_client.sessions.list(None, None).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.sessions = Rc::new(sessions);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();

        app
    }

    pub(crate) fn ensure_workspace_pane_state(
        &mut self,
        pane_id: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if pane_id == "pane-main" || self.workspace_pane_states.contains_key(pane_id) {
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
                            if let PickerTab::Provider(name) =
                                this.pane_picker_tab(&pane_id_owned)
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
            let entity = entity.clone();
            transcript.set_on_preview_image(move |image, _window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.preview_image_data(image, cx);
                    });
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
            .map(|state| state.model_search.clone())
            .expect("model search field is always present for a workspace pane")
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
            }
            None => {
                self.running_sessions.remove(session_id);
            }
        }
    }
    /// Snapshot of all currently running sessions and their start times, for
    /// the sidebar to highlight every running chat (one per pane).
    pub(crate) fn running_sessions_snapshot(&self) -> std::collections::HashMap<String, i64> {
        self.running_sessions.clone()
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
    pub(crate) fn question_selected_for_pane(&self, pane_id: &str) -> std::collections::HashSet<String> {
        self.active_session_for_pane(pane_id)
            .and_then(|sid| self.question_selected.get(&sid).cloned())
            .unwrap_or_default()
    }
    pub(crate) fn set_question_selected_for_session(
        &mut self,
        session_id: &str,
        selected: std::collections::HashSet<String>,
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
    ) -> std::collections::HashSet<String> {
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
    pub(crate) fn waiting_sessions_snapshot(&self) -> std::collections::HashSet<String> {
        let mut out: std::collections::HashSet<String> = self.pending_permissions.keys().cloned().collect();
        out.extend(self.pending_questions.keys().cloned());
        out
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
            let draft = self.get_draft_for_session(Some(&session_id)).map(|s| s.to_string());
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

    #[allow(dead_code)]
    pub fn draft_session_ids(&self) -> std::collections::HashSet<String> {
        self.drafts
            .iter()
            .filter(|(k, v)| k.as_str() != "new_chat" && !v.prompt.trim().is_empty())
            .map(|(k, _)| k.clone())
            .collect()
    }

    pub fn draft_summaries(&self) -> Vec<console_ui::DraftSummary> {
        let mut summaries = Vec::new();
        for (key, draft) in &self.drafts {
            let prompt_trimmed = draft.prompt.trim();
            if prompt_trimmed.is_empty() {
                continue;
            }
            let first_line = prompt_trimmed.lines().next().unwrap_or("").trim().to_string();
            let preview = if first_line.chars().count() > 42 {
                let truncated: String = first_line.chars().take(40).collect();
                format!("{truncated}…")
            } else {
                first_line
            };
            let project_name = if key == "new_chat" {
                self.active_pane_id
                    .as_deref()
                    .and_then(|pane_id| self.selected_project_for_pane(pane_id))
                    .map(|p| p.name.clone())
            } else if let Some(session) = self.sessions.iter().find(|s| &s.id == key) {
                self.projects
                    .iter()
                    .find(|project| {
                        session.project_id.as_deref() == Some(project.id.as_str())
                            || (!session.cwd.is_empty() && session.cwd == project.path)
                    })
                    .map(|project| project.name.clone())
            } else {
                None
            };

            if key == "new_chat" {
                summaries.push(console_ui::DraftSummary {
                    session_id: None,
                    title: "New Chat".to_string(),
                    preview,
                    project_name,
                    updated_at: draft.updated_at,
                });
            } else if let Some(session) = self.sessions.iter().find(|s| &s.id == key) {
                summaries.push(console_ui::DraftSummary {
                    session_id: Some(session.id.clone()),
                    title: if session.title.trim().is_empty() {
                        "New Chat".to_string()
                    } else {
                        session.title.clone()
                    },
                    preview,
                    project_name,
                    updated_at: draft.updated_at,
                });
            }
        }
        summaries.sort_by(|a, b| b.updated_at.cmp(&a.updated_at));
        summaries
    }

    pub fn get_draft_for_session(&self, session_id: Option<&str>) -> Option<&str> {
        let key = session_id.unwrap_or("new_chat");
        self.drafts.get(key).map(|d| d.prompt.as_str())
    }

    pub fn save_draft_for_session(&mut self, session_id: Option<&str>, text: &str) {
        let key = session_id.unwrap_or("new_chat").to_string();
        if text.trim().is_empty() {
            if self.drafts.remove(&key).is_some() {
                persistence::store::save_drafts(self.drafts.clone());
            }
        } else {
            let changed = match self.drafts.get(&key) {
                Some(existing) => existing.prompt != text,
                None => true,
            };
            if changed {
                self.drafts.insert(
                    key,
                    crate::persistence::store::PersistedDraft {
                        prompt: text.to_string(),
                        updated_at: chrono::Utc::now().timestamp(),
                    },
                );
                persistence::store::save_drafts(self.drafts.clone());
            }
        }
    }

    pub fn clear_draft_for_session(&mut self, session_id: Option<&str>) {
        let key = session_id.unwrap_or("new_chat");
        if self.drafts.remove(key).is_some() {
            persistence::store::save_drafts(self.drafts.clone());
        }
    }
}
