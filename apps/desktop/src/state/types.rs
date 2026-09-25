//! `ConsoleDesktopApp` state schema and supporting types.
//!
//! All application state struct definitions live in this single file so any
//! new state field or sub-state can be added and reviewed in one place.

use console_core::{
    ApprovalMode, AskQuestionRequest, ConsoleClient, GitBranchInfo, ImageAttachment, Model,
    PermissionRequest, ProjectInfo, ProviderCatalogEntry, QueuedPrompt, SelectedModel,
    SessionHeader, ThinkingLevel, TodoItem, WorkspaceNode,
};
use console_ui::markdown::render::TranscriptSelection;
use console_ui::terminal::TerminalView;
use console_ui::utils::SessionDateGroup;
use console_ui::{
    CommandPalette, ComposerInput, ContextMenuHandle, GlobalSearchPanel, PickerTab,
    ProjectBrowsePalette, QuickOpenPalette, TranscriptView,
};
use gpui::{Entity, ListState, Subscription};
use std::cell::RefCell;
use std::rc::Rc;

use crate::persistence;
use crate::types::WorkspacePaneState;

#[derive(Clone)]
pub struct WorkspaceTerminalState {
    pub terminals: Vec<(usize, Entity<TerminalView>)>,
    pub active_idx: usize,
    pub next_id: usize,
    /// Open script log tabs, keyed by script id.
    pub script_logs: Vec<String>,
    /// None selects a terminal; Some selects a script log id.
    pub active_script_log: Option<String>,
}

#[derive(Clone, Debug)]
pub enum ImageFileState {
    Loading,
    Loaded {
        image: std::sync::Arc<gpui::Image>,
        w: Option<u32>,
        h: Option<u32>,
        size_bytes: u64,
        mime: String,
    },
    Failed {
        message: String,
    },
    Blocked {
        title: String,
        message: String,
    },
}

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
    /// Cached workspace pane trees per project (keyed by Some(project_id) or None for no-project chats).
    pub(crate) project_workspace_roots: std::collections::HashMap<Option<String>, WorkspaceNode>,
    /// Focused pane per workspace (keyed like `project_workspace_roots`), so
    /// splits keep their focus across workspace switches and restarts.
    pub(crate) project_active_panes: std::collections::HashMap<Option<String>, String>,
    /// Throttle state for workspace.json writes (see `persist_workspaces`).
    /// Interior mutability keeps the `&self` call sites unchanged.
    pub(crate) workspaces_dirty: std::cell::Cell<bool>,
    pub(crate) last_workspaces_persist: std::cell::Cell<Option<std::time::Instant>>,
    pub(crate) persisted_workspaces_bytes: std::cell::RefCell<Option<Vec<u8>>>,
    /// Trailing timer latch for draft file saves (see `schedule_drafts_save`).
    pub(crate) drafts_save_pending: bool,
    /// User-picked folder per session awaiting server confirmation
    /// (`Some(pid)` = moved, `None` = cleared to No project). Reopens honor
    /// this over a possibly stale server header (see `sync_project_...`).
    pub(crate) pending_project_override: std::collections::HashMap<String, Option<String>>,
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
    pub thinking_level: Option<ThinkingLevel>,
    /// Global fallback for the per-pane approval-mode MRU history, used when
    /// a pane has no state entry yet (same pattern as `approval_mode` above).
    pub(crate) approval_mode_history: Vec<ApprovalMode>,
    pub model_menu: ContextMenuHandle,
    pub approval_menu: ContextMenuHandle,
    pub usage_menu: ContextMenuHandle,
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
    /// staged composer images keyed by pane. Values are `Rc` so per-frame
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
    /// Staged next-turn prompt per session, persisted on the server and
    /// broadcast as `queueUpdated` via SSE.
    pub queued_prompts: std::collections::HashMap<String, Vec<QueuedPrompt>>,
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
    pub right_sidebar_bottom_height: f32,
    pub(crate) right_sidebar_bottom_resize_start: Option<(f32, f32)>,
    pub right_sidebar_bottom_collapsed: bool,
    pub right_sidebar_terminals_by_cwd: std::collections::HashMap<String, WorkspaceTerminalState>,
    pub(crate) persisted_bottom_terminals: std::collections::HashMap<String, (usize, usize)>,
    pub(crate) persisted_script_tabs:
        std::collections::HashMap<String, crate::persistence::PersistedScriptTabs>,
    pub right_sidebar_bottom_run_selected: bool,
    pub project_scripts_by_project:
        std::collections::HashMap<String, super::project_scripts::ProjectScriptsPanelState>,
    pub(crate) project_script_streams:
        std::collections::HashMap<(String, String), (u64, gpui::Task<()>)>,
    pub(crate) project_script_stream_seq: u64,
    /// Active project's shortcut map for window-wide keyboard dispatch. Rebuilt
    /// whenever scripts load or the active project changes. Conflicts (two
    /// scripts in the same project sharing a shortcut) are omitted and surfaced
    /// via `shortcut_conflict` on the row's view model instead.
    pub active_project_shortcuts: std::collections::HashMap<String, String>,
    pub inspector_active_tab: console_ui::InspectorTab,
    pub inspector_open_auxiliary_tabs: Vec<console_ui::AuxiliaryTab>,
    pub browser_view: Option<gpui::Entity<console_ui::BrowserView>>,
    pub device_view: Option<gpui::Entity<console_ui::DeviceViewer>>,
    pub forwarded_ports_by_project:
        std::collections::HashMap<String, Rc<Vec<console_core::ForwardedPort>>>,
    pub ports_menu_handle: console_ui::ContextMenuHandle,
    pub inspector_add_tab_menu: console_ui::ContextMenuHandle,
    pub inspector_search_query: String,
    pub inspector_tree: Rc<Vec<console_ui::FileTreeNode>>,
    pub inspector_working_changes: Rc<Vec<console_core::types::GitFileEntry>>,
    /// One live fs-watch SSE stream per inspector target: the cwd it watches
    /// plus its task. Dropping the task cancels the stream, so replacing this
    /// on target change is what prevents duplicate stream accumulation.
    pub(crate) inspector_fs_watch: Option<(String, gpui::Task<()>)>,
    /// Same as `inspector_fs_watch` for the git-status watch stream.
    pub(crate) inspector_git_watch: Option<(String, gpui::Task<()>)>,
    /// One live forwarded-port SSE stream for the active backend environment.
    pub(crate) port_stream: Option<gpui::Task<()>>,
    /// Trailing-debounce latch so bursts of fs events fetch the tree once.
    pub(crate) fs_tree_fetch_pending: bool,
    pub inspector_session_changes: Rc<Vec<console_core::types::SessionFileChange>>,
    pub inspector_changes_scope: console_core::types::ChangesScope,
    pub inspector_changes_scope_menu: console_ui::ContextMenuHandle,
    /// Fetched change list for each open "View all" review tab, keyed by the
    /// tab id (`changesReview:<sessionId>:<Scope>`). Populated once on open;
    /// `diffText` per row is already included by the `GET /changes` response
    /// with a resolved scope, so no per-file follow-up fetch is needed.
    pub(crate) changes_review_data:
        std::collections::HashMap<String, Rc<Vec<console_core::types::SessionFileChange>>>,
    /// Paths collapsed in a given review tab (local UI state, not persisted).
    /// A file starts collapsed iff it was already reviewed when the tab's
    /// data was fetched; toggling reviewed flips membership here too.
    pub(crate) changes_review_collapsed:
        std::collections::HashMap<String, std::collections::HashSet<String>>,
    pub inspector_expanded_folders: Rc<std::collections::HashSet<String>>,
    pub inspector_selected_path: Option<String>,
    pub session_subagents:
        std::collections::HashMap<String, Rc<Vec<console_core::types::SubagentInfo>>>,
    pub expanded_subagents: std::collections::HashSet<String>,
    pub subagent_markdown_views: Rc<
        RefCell<
            std::collections::HashMap<
                String,
                Rc<RefCell<console_ui::markdown::render::MarkdownView>>,
            >,
        >,
    >,
    pub preview_tab: Option<(String, std::time::Instant)>,
    pub open_file_contents: std::collections::HashMap<String, String>,
    pub open_image_contents: std::collections::HashMap<String, ImageFileState>,
    pub svg_preview_mode: std::collections::HashMap<String, console_ui::SvgViewMode>,
    pub open_diff_contents: std::collections::HashMap<String, (console_core::DiffResult, String)>,
    pub viewer_list_states: std::collections::HashMap<String, ListState>,
    pub viewer_scrollbar_states:
        std::collections::HashMap<String, std::rc::Rc<console_ui::ScrollbarState>>,
    pub viewer_editor_views: std::collections::HashMap<
        String,
        (
            usize,
            u64,
            gpui::Entity<editor_ui::EditorState>,
            gpui::Entity<editor_ui::EditorView>,
        ),
    >,
    pub viewer_diff_views: std::collections::HashMap<
        String,
        (
            usize,
            u64,
            gpui::Entity<editor_ui::DiffState>,
            gpui::Entity<editor_ui::DiffView>,
        ),
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
    /// Which axis the sidebar session list is sectioned by (date buckets or
    /// project sections). Persisted; defaults to date.
    pub sidebar_sort_mode: console_ui::utils::SidebarSortMode,
    /// Collapsed project sections (project id, or "none") in project sort
    /// mode. Shared with the sidebar; cloned per frame as a refcount bump.
    pub collapsed_projects: Rc<std::collections::HashSet<String>>,
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
    /// ⌘⇧P palette for switching between open chat and terminal tabs.
    pub tab_palette: Entity<CommandPalette>,
    /// ⌘P quick file open palette, scoped to the active pane's project root.
    pub quick_open_palette: Entity<QuickOpenPalette>,
    /// ⌘⇧F global content search panel, scoped to the active pane's project root.
    pub global_search_panel: Entity<GlobalSearchPanel>,
    /// ⌘O remote directory browser / project picker.
    pub project_browse_palette: Entity<ProjectBrowsePalette>,
    /// Live terminal surfaces keyed by terminal id. Tabs reference these via
    /// `WorkspaceTabConfig::Terminal { terminal_id }`.
    pub terminals: std::collections::HashMap<String, Entity<TerminalView>>,
    /// Live browser surfaces keyed by browser id. Tabs reference these via
    /// `WorkspaceTabConfig::Browser { browser_id }`. Separate from the
    /// inspector's singleton `browser_view`.
    pub browser_views: std::collections::HashMap<String, Entity<console_ui::BrowserView>>,
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
    /// True only for windows launched as the persisted main window.
    /// Secondary "New Window" windows never persist layout or bounds.
    pub is_main_window: bool,
    pub drafts: std::collections::HashMap<String, crate::persistence::store::PersistedDraft>,
    /// Session IDs whose draft is confirmed for sidebar display.
    /// Only updated when a tab closes (or submit). Typing never touches this —
    /// so the sidebar stays frozen while a tab is open.
    pub sidebar_draft_ids: std::collections::HashSet<String>,
    pub drafts_collapsed: bool,
    pub is_window_active: bool,
    pub _subscriptions: Vec<Subscription>,
}
