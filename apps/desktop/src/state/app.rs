//! `ConsoleDesktopApp` — the single gpui entity owning all application state.
//!
//! This file keeps the struct definition and the bootstrap constructor;
//! domain handlers live in sibling modules (`projects`, `attachments`,
//! `errors`, `sessions`, `layout`, `run`) as additional `impl` blocks.

use console_core::{
    AgentMessage, ApprovalMode, AskQuestionRequest, ConsoleClient, GitBranchInfo, ImageAttachment,
    Model, ModelFavorite, PermissionRequest, ProjectInfo, ProviderCatalogEntry, SelectedModel,
    SessionHeader, TodoItem, WorkspaceNode,
};
use console_ui::markdown::render::TranscriptSelection;
use console_ui::terminal::TerminalView;
use console_ui::utils::SessionDateGroup;
use console_ui::{
    CommandPalette, ComposerAttachmentPaste, ComposerEvent, ComposerInput, ContextMenuHandle,
    PickerTab, ProjectBrowsePalette, QuickOpenPalette, TranscriptView,
};
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
pub(crate) const RIGHT_SIDEBAR_DEFAULT_WIDTH: f32 = 280.0;
pub(crate) const RIGHT_SIDEBAR_MIN_WIDTH: f32 = 220.0;
pub(crate) const RIGHT_SIDEBAR_MAX_WIDTH: f32 = 550.0;

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
    pub(crate) transcript_pagination:
        std::collections::HashMap<String, crate::state::pagination::SessionPaginationState>,
    pub(crate) pagination_in_flight: std::collections::HashSet<String>,
    pub composer_input: Entity<ComposerInput>,
    pub question_input: Entity<ComposerInput>,
    /// Staged composer images keyed by pane. Values are `Rc` so per-frame
    /// renders clone a refcount instead of megabyte base64 payloads.
    pub attachments: std::collections::HashMap<String, Rc<Vec<ImageAttachment>>>,
    /// Run-derived interactive and display state. All of these are keyed by
    /// session id (not pane id) so a run's permission prompt, question, todos,
    /// and notices stay attached to the chat that owns the run. Switching a
    /// pane to another chat surfaces that chat's state instead of leaking the
    /// background run's state onto it.
    pub pending_permissions: std::collections::HashMap<String, PermissionRequest>,
    pub pending_questions: std::collections::HashMap<String, AskQuestionRequest>,
    pub question_selected: std::collections::HashMap<String, std::collections::HashSet<String>>,
    pub todo_items: std::collections::HashMap<String, Vec<TodoItem>>,
    pub todos_collapsed: std::collections::HashMap<String, bool>,
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
    /// Monotonic token per session incremented on each prompt submission.
    /// Ensures out-of-order or late settling calls cannot overwrite newer runs.
    pub session_run_tokens: std::collections::HashMap<String, u64>,
    /// A stream burst can contain many provider chunks. The render loop uses
    /// this latch to publish at most one transcript repaint per cadence window.
    pub(crate) stream_render_pending: std::collections::HashMap<String, bool>,
    pub sidebar_visible: bool,
    pub sidebar_width: f32,
    pub right_sidebar_visible: bool,
    pub right_sidebar_width: f32,
    pub(crate) right_sidebar_resize_start: Option<(f32, f32)>,
    pub inspector_active_tab: console_ui::InspectorTab,
    pub inspector_search_query: String,
    pub inspector_tree: Rc<Vec<console_ui::FileTreeNode>>,
    pub inspector_working_changes: Rc<Vec<console_core::types::GitFileEntry>>,
    pub inspector_session_changes: Rc<Vec<console_core::types::SessionFileChange>>,
    pub inspector_expanded_folders: Rc<std::collections::HashSet<String>>,
    pub inspector_selected_path: Option<String>,
    pub session_subagents:
        std::collections::HashMap<String, Rc<Vec<console_core::types::SubagentInfo>>>,
    pub expanded_subagents: std::collections::HashSet<String>,
    pub preview_tab: Option<(String, std::time::Instant)>,
    pub open_file_contents: std::collections::HashMap<String, String>,
    pub open_diff_contents: std::collections::HashMap<String, (console_core::DiffResult, String)>,
    pub viewer_list_states: std::collections::HashMap<String, ListState>,
    pub viewer_selection_states: std::collections::HashMap<
        String,
        std::rc::Rc<std::cell::RefCell<console_ui::SelectionState>>,
    >,
    pub viewer_scrollbar_states:
        std::collections::HashMap<String, std::rc::Rc<console_ui::ScrollbarState>>,
    pub viewer_cached_file_lines: std::collections::HashMap<
        String,
        (usize, u64, std::rc::Rc<Vec<console_ui::CodeViewerLine>>),
    >,
    pub viewer_cached_diff_lines: std::collections::HashMap<
        String,
        (usize, u64, std::rc::Rc<Vec<console_ui::CodeViewerLine>>),
    >,
    pub viewer_cached_markdown_views: std::collections::HashMap<
        String,
        (
            usize,
            u64,
            std::rc::Rc<std::cell::RefCell<console_ui::MarkdownView>>,
        ),
    >,
    pub viewer_markdown_selections:
        std::collections::HashMap<String, console_ui::markdown::render::TranscriptSelection>,
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
    /// Last render-loop window-bounds poll. Keeps the per-frame
    /// `maybe_persist_window_state` check to an `Instant` comparison so idle
    /// renders skip the `window.window_bounds()` OS call.
    pub(crate) last_window_poll: Option<std::time::Instant>,
    /// ⌘K-style command palette (New Chat, New Terminal, …).
    pub command_palette: Entity<CommandPalette>,
    /// ⌘P quick file open palette, scoped to the active pane's project root.
    pub quick_open_palette: Entity<QuickOpenPalette>,
    /// ⌘O remote directory browser / project picker.
    pub project_browse_palette: Entity<ProjectBrowsePalette>,
    /// Live terminal surfaces keyed by terminal id. Tabs reference these via
    /// `WorkspaceTabConfig::Terminal { terminal_id }`.
    pub terminals: std::collections::HashMap<String, Entity<TerminalView>>,
    pub auth_status: Option<console_core::types::AuthStatusResponse>,
    pub auth_logging_in: std::collections::HashSet<String>,
    pub usage_reports:
        Option<Rc<std::collections::HashMap<String, Option<console_core::types::UsageReport>>>>,
    pub usage_loading: bool,
    pub usage_last_fetched: Option<std::time::SystemTime>,
    pub environments: Vec<super::environments::Environment>,
    pub active_env_id: Option<String>,
    pub env_probes: std::collections::HashMap<String, console_ui::settings::ProbeState>,
    pub server_menu: console_ui::primitives::ContextMenuHandle,
    pub deleted_sessions: Vec<SessionHeader>,
    pub settings_window_handle: Option<gpui::AnyWindowHandle>,
    pub settings_window_view: Option<gpui::WeakEntity<crate::settings_window::SettingsWindow>>,
    pub main_window_handle: Option<gpui::AnyWindowHandle>,
    pub drafts: std::collections::HashMap<String, crate::persistence::store::PersistedDraft>,
    /// Session IDs whose draft is confirmed for sidebar display.
    /// Only updated when a tab closes (or submit). Typing never touches this —
    /// so the sidebar stays frozen while a tab is open.
    pub sidebar_draft_ids: std::collections::HashSet<String>,
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
        let right_sidebar_width = if layout.right_sidebar_width.is_finite() {
            layout
                .right_sidebar_width
                .clamp(RIGHT_SIDEBAR_MIN_WIDTH, RIGHT_SIDEBAR_MAX_WIDTH)
        } else {
            RIGHT_SIDEBAR_DEFAULT_WIDTH
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
                {
                    let entity = entity.clone();
                    transcript.set_on_preview_image(move |image, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.preview_image_data(image, cx);
                            });
                        }
                    });
                }
                {
                    let entity = entity.clone();
                    transcript.set_on_view_subagent(move |call_id, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.view_subagent_in_panel(&call_id, cx);
                            });
                        }
                    });
                }
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
                            if let PickerTab::Provider(name) = this.pane_picker_tab("pane-main") {
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
        // All persisted drafts (except new_chat) are already confirmed for sidebar display.
        let sidebar_draft_ids: std::collections::HashSet<String> = drafts
            .keys()
            .filter(|k| k.as_str() != "new_chat" && !drafts[*k].prompt.trim().is_empty())
            .cloned()
            .collect();
        if let Some(initial_draft) = drafts.get("new_chat") {
            if !initial_draft.prompt.trim().is_empty() {
                composer_input.update(cx, |input, cx| {
                    input.set_content(initial_draft.prompt.clone(), cx);
                });
            }
        }

        let subscriptions = vec![
            cx.subscribe(&composer_input, |this, input, event: &ComposerEvent, cx| {
                match event {
                    ComposerEvent::Submit(prompt) => {
                        // The main composer belongs to "pane-main" even when
                        // another split holds focus; pin the pane before
                        // submitting so attachments and run state resolve
                        // against the chat this input is mounted in.
                        this.active_pane_id = Some("pane-main".to_string());
                        this.selected_session_id = this.active_session_for_pane("pane-main");
                        // Deep-copy only at the submit boundary; the Rc
                        // keeps per-frame renders cheap.
                        let attachments = (*this.attachments_for_pane("pane-main")).clone();
                        this.submit_prompt(prompt.clone(), attachments, cx);
                    }
                    ComposerEvent::Edited => {
                        // Save raw text for crash safety; does NOT update sidebar_draft_ids.
                        let text = input.read(cx).content().to_string();
                        let session_id = this.active_session_for_pane("pane-main");
                        this.save_draft_for_session(session_id.as_deref(), &text);
                    }
                    ComposerEvent::Focus => cx.notify(),
                    // Backspace on an empty composer removes the last staged
                    // attachment, the chat idiom for discarding a chip.
                    ComposerEvent::BackspaceOnEmpty
                        if !this
                            .attachments_for_pane(
                                this.active_pane_id.as_deref().unwrap_or("pane-main"),
                            )
                            .is_empty() =>
                    {
                        let pane_id = this
                            .active_pane_id
                            .clone()
                            .unwrap_or_else(|| "pane-main".to_string());
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
            }),
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

        let client_for_palettes = client.clone();
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
            active_picker_tab: PickerTab::Provider("antigravity".to_string()),
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
            transcript_pagination: std::collections::HashMap::new(),
            pagination_in_flight: std::collections::HashSet::new(),
            composer_input,
            question_input,
            attachments: std::collections::HashMap::new(),
            pending_permissions: std::collections::HashMap::new(),
            pending_questions: std::collections::HashMap::new(),
            question_selected: std::collections::HashMap::new(),
            todo_items: std::collections::HashMap::new(),
            todos_collapsed: std::collections::HashMap::new(),
            agent_notices: std::collections::HashMap::new(),
            error_message: None,
            session_errors: std::collections::HashMap::new(),
            error_selection: TranscriptSelection::default(),
            error_generation: 0,
            zoomed_image: None,
            running_sessions: std::collections::HashMap::new(),
            session_run_tokens: std::collections::HashMap::new(),
            stream_render_pending: std::collections::HashMap::new(),
            sidebar_visible: layout.sidebar_visible,
            sidebar_width,
            right_sidebar_visible: layout.right_sidebar_visible,
            right_sidebar_width,
            right_sidebar_resize_start: None,
            inspector_active_tab: console_ui::InspectorTab::AllFiles,
            inspector_search_query: String::new(),
            inspector_tree: Rc::new(Vec::new()),
            inspector_working_changes: Rc::new(Vec::new()),
            inspector_session_changes: Rc::new(Vec::new()),
            inspector_expanded_folders: Rc::new(std::collections::HashSet::new()),
            inspector_selected_path: None,
            session_subagents: std::collections::HashMap::new(),
            expanded_subagents: std::collections::HashSet::new(),
            preview_tab: None,
            open_file_contents: std::collections::HashMap::new(),
            open_diff_contents: std::collections::HashMap::new(),
            viewer_list_states: std::collections::HashMap::new(),
            viewer_selection_states: std::collections::HashMap::new(),
            viewer_scrollbar_states: std::collections::HashMap::new(),
            viewer_cached_file_lines: std::collections::HashMap::new(),
            viewer_cached_diff_lines: std::collections::HashMap::new(),
            viewer_cached_markdown_views: std::collections::HashMap::new(),
            viewer_markdown_selections: std::collections::HashMap::new(),
            sidebar_list_state: ListState::new(0, ListAlignment::Top, px(55.0)),
            sidebar_resize_start: None,
            split_resize: None,
            saved_window_state: persistence::store::load_window(),
            pending_window_state: None,
            last_window_poll: None,
            command_palette: cx.new(|cx| CommandPalette::new(window, cx)),
            quick_open_palette: cx
                .new(|cx| QuickOpenPalette::new(client_for_palettes.clone(), window, cx)),
            project_browse_palette: cx
                .new(|cx| ProjectBrowsePalette::new(client_for_palettes.clone(), window, cx)),
            terminals: std::collections::HashMap::new(),
            auth_status: None,
            auth_logging_in: std::collections::HashSet::new(),
            usage_reports: None,
            usage_loading: false,
            usage_last_fetched: None,
            environments: Vec::new(),
            active_env_id: None,
            env_probes: std::collections::HashMap::new(),
            server_menu: console_ui::primitives::ContextMenuHandle::new(cx),
            deleted_sessions: Vec::new(),
            settings_window_handle: None,
            settings_window_view: None,
            main_window_handle: Some(window.window_handle().into()),
            drafts,
            sidebar_draft_ids,
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

        // Wire the palette wrappers back into the app: ⌘P opens a confirmed
        // file as a workspace tab, ⌘O registers the browsed folder as a
        // project. The wrappers hide their own modals.
        {
            let entity = cx.entity().downgrade();
            app.quick_open_palette.update(cx, |palette, cx| {
                palette.set_on_open_file(
                    move |path, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let pane_id = this
                                    .active_pane_id
                                    .clone()
                                    .unwrap_or_else(|| "pane-main".to_string());
                                this.open_file_tab_in_pane(&pane_id, path, cx);
                            });
                        }
                    },
                    cx,
                );
            });
        }
        {
            let entity = cx.entity().downgrade();
            app.project_browse_palette.update(cx, |palette, cx| {
                palette.set_on_select_project(
                    move |path, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.add_project_from_path(path, cx));
                        }
                    },
                    cx,
                );
            });
        }

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
                loaded_session_id: None,
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
                                        PickerTab::Provider(name) => {
                                            !this.providers.iter().any(|p| &p.name == name)
                                        }
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
                                            this.branches = Rc::new(branches.branches.clone());
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
        cx.spawn(async move |entity, cx| {
            loop {
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
            }
        })
        .detach();

        app
    }

    pub fn viewer_list_state(
        &mut self,
        id: &str,
        count: usize,
        row_height: f32,
    ) -> gpui::ListState {
        let state = self
            .viewer_list_states
            .entry(id.to_string())
            .or_insert_with(|| {
                gpui::ListState::new(count, gpui::ListAlignment::Top, gpui::px(120.0))
                    .with_uniform_item_height(gpui::px(row_height))
            });
        if state.item_count() != count {
            state.reset_with_uniform_height(count, gpui::px(row_height));
        }
        state.clone()
    }

    pub fn viewer_selection_state(
        &mut self,
        id: &str,
    ) -> std::rc::Rc<std::cell::RefCell<console_ui::SelectionState>> {
        self.viewer_selection_states
            .entry(id.to_string())
            .or_insert_with(|| {
                std::rc::Rc::new(std::cell::RefCell::new(
                    console_ui::SelectionState::default(),
                ))
            })
            .clone()
    }

    pub fn viewer_scrollbar_state(&mut self, id: &str) -> std::rc::Rc<console_ui::ScrollbarState> {
        self.viewer_scrollbar_states
            .entry(id.to_string())
            .or_insert_with(console_ui::ScrollbarState::new)
            .clone()
    }

    pub fn get_or_build_file_lines(
        &mut self,
        path: &str,
        content: &str,
    ) -> std::rc::Rc<Vec<console_ui::CodeViewerLine>> {
        use std::hash::{Hash, Hasher};
        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        content.hash(&mut hasher);
        let hash = hasher.finish();
        let len = content.len();

        if let Some((cached_len, cached_hash, cached_lines)) =
            self.viewer_cached_file_lines.get(path)
        {
            if *cached_len == len && *cached_hash == hash {
                return cached_lines.clone();
            }
        }

        let lines = std::rc::Rc::new(console_ui::build_file_lines(path, content));
        self.viewer_cached_file_lines
            .insert(path.to_string(), (len, hash, lines.clone()));
        lines
    }

    pub fn get_or_build_diff_lines(
        &mut self,
        path: &str,
        diff: &console_core::DiffResult,
        theme: &console_ui::Theme,
    ) -> std::rc::Rc<Vec<console_ui::CodeViewerLine>> {
        use std::hash::{Hash, Hasher};
        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        diff.lines.len().hash(&mut hasher);
        for line in &diff.lines {
            line.text.hash(&mut hasher);
        }
        let hash = hasher.finish();
        let len = diff.lines.len();

        if let Some((cached_len, cached_hash, cached_lines)) =
            self.viewer_cached_diff_lines.get(path)
        {
            if *cached_len == len && *cached_hash == hash {
                return cached_lines.clone();
            }
        }

        let lines = std::rc::Rc::new(console_ui::build_diff_lines(path, diff, theme));
        self.viewer_cached_diff_lines
            .insert(path.to_string(), (len, hash, lines.clone()));
        lines
    }

    pub fn get_or_build_markdown_view(
        &mut self,
        path: &str,
        content: &str,
    ) -> std::rc::Rc<std::cell::RefCell<console_ui::MarkdownView>> {
        use std::hash::{Hash, Hasher};
        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        content.hash(&mut hasher);
        let hash = hasher.finish();
        let len = content.len();

        if let Some((cached_len, cached_hash, view)) = self.viewer_cached_markdown_views.get(path) {
            if *cached_len == len && *cached_hash == hash {
                return view.clone();
            }
        }

        let mut view = console_ui::MarkdownView::new();
        view.set_text(content, false);
        let rc = std::rc::Rc::new(std::cell::RefCell::new(view));
        self.viewer_cached_markdown_views
            .insert(path.to_string(), (len, hash, rc.clone()));
        rc
    }

    pub fn viewer_markdown_selection(
        &mut self,
        path: &str,
    ) -> console_ui::markdown::render::TranscriptSelection {
        self.viewer_markdown_selections
            .entry(path.to_string())
            .or_default()
            .clone()
    }
}
