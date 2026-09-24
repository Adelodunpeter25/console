//! `ConsoleDesktopApp` — the single gpui entity owning all application state.
//!
//! This file keeps the struct definition and the bootstrap constructor;
//! domain handlers live in sibling modules (`projects`, `attachments`,
//! `errors`, `sessions`, `layout`, `run`) as additional `impl` blocks.

use console_core::{AgentMessage, ApprovalMode, ConsoleClient, WorkspaceNode};
use console_ui::markdown::render::TranscriptSelection;
use console_ui::utils::SessionDateGroup;
use console_ui::{
    CommandPalette, ComposerAttachmentPaste, ComposerEvent, ComposerInput, ContextMenuHandle,
    PickerTab, ProjectBrowsePalette, QuickOpenPalette, TranscriptView,
};
use gpui::{AppContext, Context, ListAlignment, ListState, Window, px};
use std::cell::RefCell;
use std::rc::Rc;

use crate::persistence;
use crate::types::WorkspacePaneState;

/// User prompts in a session, used to populate the composer's up-arrow
/// history. Shared by `sessions` and `run`. Each entry pairs the prompt text
/// with the full paths of any file mentions it carried, so recalling an
/// older prompt from history can rebuild its mention chips.
pub(crate) fn user_prompt_history(messages: &[AgentMessage]) -> Vec<(String, Vec<String>)> {
    messages
        .iter()
        .filter_map(|message| match message {
            AgentMessage::User {
                content,
                context_files,
                ..
            } => Some((content.clone(), context_files.clone().unwrap_or_default())),
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
pub(crate) const RIGHT_SIDEBAR_BOTTOM_DEFAULT_HEIGHT: f32 = 180.0;
pub(crate) const RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT: f32 = 100.0;
pub(crate) const RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT: f32 = 450.0;

pub use super::types::{ConsoleDesktopApp, ImageFileState};

impl ConsoleDesktopApp {
    pub fn new(
        window: &mut Window,
        target: crate::window::WindowLaunchTarget,
        cx: &mut Context<Self>,
    ) -> Self {
        let client = ConsoleClient::new(None);
        let ws_doc = persistence::load_workspaces();
        let mut project_workspace_roots = std::collections::HashMap::new();
        let mut project_active_panes = std::collections::HashMap::new();
        let mut persisted_bottom_terminals = std::collections::HashMap::new();
        let mut persisted_script_tabs = std::collections::HashMap::new();
        for ws in ws_doc.workspaces {
            let mut root = ws.root;
            // Never restore a workspace containing another folder's tabs
            // (heals saves written before folder-change partitioning).
            console_ui::workspace::ops::retain_project_tabs(&mut root, &ws.project_id);
            if let Some(pane_id) = ws.active_pane_id {
                if root.leaves().iter().any(|leaf| leaf.id == pane_id) {
                    project_active_panes.insert(ws.project_id.clone(), pane_id);
                }
            }
            project_workspace_roots.insert(ws.project_id, root);
            if let Some(cwd) = ws.cwd {
                if let (Some(count), Some(active_idx)) =
                    (ws.bottom_terminal_tab_count, ws.bottom_terminal_active_idx)
                {
                    persisted_bottom_terminals.insert(cwd.clone(), (count, active_idx));
                }
                if let Some(script_tabs) = ws.script_tabs {
                    persisted_script_tabs.insert(cwd, script_tabs);
                }
            }
        }
        let ws_state = persistence::load_workspace_state();
        // One read of state.json split in memory: window, layout, drafts and
        // environments previously re-read + re-parsed the same file each.
        let store_doc = persistence::read_document();
        let layout = store_doc.layout.clone().unwrap_or_default();

        let (
            sidebar_visible,
            sidebar_width,
            right_sidebar_visible,
            right_sidebar_width,
            right_sidebar_bottom_height,
            right_sidebar_bottom_collapsed,
            initial_selected_project_id,
            initial_root,
            initial_saved_window_state,
        ) = match &target {
            crate::window::WindowLaunchTarget::RestorePersisted => {
                let sb_visible = ws_state.sidebar_visible;
                let sb_width = if ws_state.sidebar_width.is_finite() && ws_state.sidebar_width > 0.0
                {
                    ws_state
                        .sidebar_width
                        .clamp(SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH)
                } else if layout.sidebar_width.is_finite() {
                    layout
                        .sidebar_width
                        .clamp(SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH)
                } else {
                    SIDEBAR_DEFAULT_WIDTH
                };
                let rsb_visible = ws_state.right_sidebar_visible;
                let rsb_width = if ws_state.right_sidebar_width.is_finite()
                    && ws_state.right_sidebar_width > 0.0
                {
                    ws_state
                        .right_sidebar_width
                        .clamp(RIGHT_SIDEBAR_MIN_WIDTH, RIGHT_SIDEBAR_MAX_WIDTH)
                } else if layout.right_sidebar_width.is_finite() {
                    layout
                        .right_sidebar_width
                        .clamp(RIGHT_SIDEBAR_MIN_WIDTH, RIGHT_SIDEBAR_MAX_WIDTH)
                } else {
                    RIGHT_SIDEBAR_DEFAULT_WIDTH
                };
                let rsb_bottom_height = if ws_state.right_sidebar_bottom_height.is_finite()
                    && ws_state.right_sidebar_bottom_height > 0.0
                {
                    ws_state.right_sidebar_bottom_height.clamp(
                        RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT,
                        RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT,
                    )
                } else if layout.right_sidebar_bottom_height.is_finite()
                    && layout.right_sidebar_bottom_height > 0.0
                {
                    layout.right_sidebar_bottom_height.clamp(
                        RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT,
                        RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT,
                    )
                } else {
                    RIGHT_SIDEBAR_BOTTOM_DEFAULT_HEIGHT
                };
                let rsb_bottom_collapsed = ws_state.right_sidebar_bottom_collapsed;
                let proj_id = ws_state.active_workspace_id.as_ref().and_then(|wid| {
                    if wid == "__default__" {
                        None
                    } else {
                        Some(wid.clone())
                    }
                });
                let root = proj_id
                    .as_ref()
                    .and_then(|pid| project_workspace_roots.get(&Some(pid.clone())).cloned())
                    .or_else(|| project_workspace_roots.get(&None).cloned())
                    .unwrap_or_else(|| WorkspaceNode::leaf("pane-main"));
                (
                    sb_visible,
                    sb_width,
                    rsb_visible,
                    rsb_width,
                    rsb_bottom_height,
                    rsb_bottom_collapsed,
                    proj_id,
                    root,
                    store_doc.window.clone(),
                )
            }
            crate::window::WindowLaunchTarget::Fresh { .. }
            | crate::window::WindowLaunchTarget::Session(_) => {
                let sb_visible = ws_state.sidebar_visible;
                let sb_width = if ws_state.sidebar_width.is_finite() && ws_state.sidebar_width > 0.0
                {
                    ws_state
                        .sidebar_width
                        .clamp(SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH)
                } else {
                    SIDEBAR_DEFAULT_WIDTH
                };
                let rsb_visible = ws_state.right_sidebar_visible;
                let rsb_width = if ws_state.right_sidebar_width.is_finite()
                    && ws_state.right_sidebar_width > 0.0
                {
                    ws_state
                        .right_sidebar_width
                        .clamp(RIGHT_SIDEBAR_MIN_WIDTH, RIGHT_SIDEBAR_MAX_WIDTH)
                } else {
                    RIGHT_SIDEBAR_DEFAULT_WIDTH
                };
                let rsb_bottom_height = if ws_state.right_sidebar_bottom_height.is_finite()
                    && ws_state.right_sidebar_bottom_height > 0.0
                {
                    ws_state.right_sidebar_bottom_height.clamp(
                        RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT,
                        RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT,
                    )
                } else {
                    RIGHT_SIDEBAR_BOTTOM_DEFAULT_HEIGHT
                };
                let rsb_bottom_collapsed = ws_state.right_sidebar_bottom_collapsed;
                (
                    sb_visible,
                    sb_width,
                    rsb_visible,
                    rsb_width,
                    rsb_bottom_height,
                    rsb_bottom_collapsed,
                    None,
                    WorkspaceNode::leaf("pane-main"),
                    None,
                )
            }
        };
        // Restore this workspace's remembered split focus when still present,
        // else the first leaf.
        let initial_active_pane_id = project_active_panes
            .get(&initial_selected_project_id)
            .cloned()
            .filter(|id| initial_root.leaves().iter().any(|leaf| &leaf.id == id))
            .or_else(|| initial_root.first_leaf().map(|l| l.id.clone()))
            .or_else(|| Some("pane-main".into()));
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
                super::transcript_wiring::wire_preview_image(transcript, entity.clone());
                super::transcript_wiring::wire_view_subagent(transcript, entity.clone());
                super::transcript_wiring::wire_open_file_for_active_pane(
                    transcript,
                    entity.clone(),
                );
            });
        }
        let model_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            let search = model_search.clone();
            move |open, window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        cx.defer(move |cx| {
                            if let Some(window) = cx.active_window() {
                                let _ = window.update(cx, |_, window, cx| {
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
                                });
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
            move |open, _window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        cx.defer(move |cx| {
                            if let Some(window) = cx.active_window() {
                                let _ = window.update(cx, |_, window, cx| {
                                    app.update(cx, |this, cx| this.model_menu.close(window, cx));
                                });
                            }
                        });
                    }
                }
            }
        });
        let project_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, _window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        cx.defer(move |cx| {
                            if let Some(window) = cx.active_window() {
                                let _ = window.update(cx, |_, window, cx| {
                                    app.update(cx, |this, cx| this.branch_menu.close(window, cx));
                                });
                            }
                        });
                    }
                }
            }
        });
        let branch_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, _window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        cx.defer(move |cx| {
                            if let Some(window) = cx.active_window() {
                                let _ = window.update(cx, |_, window, cx| {
                                    app.update(cx, |this, cx| this.project_menu.close(window, cx));
                                });
                            }
                        });
                    }
                }
            }
        });
        let usage_menu = ContextMenuHandle::new(cx).on_toggle({
            let entity = entity.clone();
            move |open, _window, cx| {
                if open {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let session_id = this.active_session_for_pane("pane-main").unwrap_or_default();
                            this.maybe_fetch_usage(&session_id, cx);
                        });
                    }
                }
            }
        });

        let drafts = store_doc.drafts.map(|s| s.drafts).unwrap_or_default();
        // All persisted drafts (except new_chat) are already confirmed for sidebar display.
        let sidebar_draft_ids: std::collections::HashSet<String> = drafts
            .keys()
            .filter(|k| k.as_str() != "new_chat" && !drafts[*k].prompt.trim().is_empty())
            .cloned()
            .collect();
        if let Some(initial_draft) = drafts.get("new_chat") {
            if !initial_draft.prompt.trim().is_empty() || !initial_draft.context_files.is_empty() {
                let mentions = initial_draft
                    .mentions
                    .iter()
                    .map(|mention| console_ui::ComposerMention {
                        range: mention.start..mention.end,
                        path: mention.path.clone(),
                        label: mention.label.clone().unwrap_or_else(|| {
                            std::path::Path::new(&mention.path)
                                .file_name()
                                .and_then(|n| n.to_str())
                                .unwrap_or(&mention.path)
                                .to_string()
                        }),
                    })
                    .collect();
                let context_files = initial_draft.context_files.clone();
                composer_input.update(cx, |input, cx| {
                    input.set_content_with_mentions(initial_draft.prompt.clone(), mentions, cx);
                    input.set_context_files(context_files);
                    cx.notify();
                });
            }
        }

        let subscriptions = vec![
            cx.observe_window_activation(window, |this, window, cx| {
                this.is_window_active = window.is_window_active();
                if this.is_window_active {
                    crate::state::clear_all_notifications();
                }
                cx.notify();
            }),
            cx.subscribe(&composer_input, |this, input, event: &ComposerEvent, cx| {
                match event {
                    ComposerEvent::Submit(prompt, context_files) => {
                        // The main composer belongs to "pane-main" even when
                        // another split holds focus; pin the pane before
                        // submitting so attachments and run state resolve
                        // against the chat this input is mounted in.
                        let context_files = context_files.clone();
                        this.active_pane_id = Some("pane-main".to_string());
                        this.selected_session_id = this.active_session_for_pane("pane-main");
                        let pane_id = "pane-main".to_string();
                        if this.is_active_session_running_for_pane(&pane_id) {
                            // Turn is running: queue behind it instead of starting a parallel run.
                            let attachments = (*this.attachments_for_pane(&pane_id)).clone();
                            this.queue_prompt_for_pane(
                                pane_id,
                                prompt.clone(),
                                attachments,
                                context_files.clone(),
                                cx,
                            );
                            return;
                        }
                        // Deep-copy only at the submit boundary; the Rc
                        // keeps per-frame renders cheap.
                        let attachments = (*this.attachments_for_pane("pane-main")).clone();
                        this.submit_prompt_with_context(prompt.clone(), attachments, context_files, cx);
                    }
                    ComposerEvent::SubmitSteer(prompt, context_files) => {
                        this.active_pane_id = Some("pane-main".to_string());
                        this.selected_session_id = this.active_session_for_pane("pane-main");
                        this.submit_steer_for_pane(
                            "pane-main".to_string(),
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
                        let session_id = this.active_session_for_pane("pane-main");
                        let attachments = this.attachments_for_pane("pane-main");
                        this.save_draft_for_session_with_context(
                            session_id.as_deref(),
                            &text,
                            &mentions,
                            &[],
                            &attachments,
                            cx,
                        );
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
            // chips instead of inserting text. pane-main is the only
            // composer owned here; splits subscribe their own in
            // `ensure_workspace_pane_state` with their pane id captured.
            cx.subscribe(
                &composer_input,
                |this, _input, event: &ComposerAttachmentPaste, cx| {
                    this.stage_clipboard_attachments("pane-main", event.0.clone(), cx);
                },
            ),
            cx.subscribe(
                &question_input,
                |this, _input, event: &ComposerEvent, cx| match event {
                    // Typing is repainted by the input entity itself; a full
                    // app render per keystroke only adds lag.
                    ComposerEvent::Edited => {}
                    ComposerEvent::Focus => cx.notify(),
                    ComposerEvent::Submit(answer, _) if !answer.trim().is_empty() => {
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
            workspace_root: initial_root,
            project_workspace_roots,
            project_active_panes,
            workspaces_dirty: std::cell::Cell::new(false),
            last_workspaces_persist: std::cell::Cell::new(None),
            persisted_workspaces_bytes: std::cell::RefCell::new(None),
            drafts_save_pending: false,
            pending_project_override: std::collections::HashMap::new(),
            active_pane_id: initial_active_pane_id,
            providers: Rc::new(Vec::new()),
            models_by_provider: Rc::new(std::collections::HashMap::new()),
            loading_models: std::collections::HashSet::new(),
            selected_model: None,
            active_picker_tab: PickerTab::Provider("antigravity".to_string()),
            favorites: Rc::new(std::collections::HashSet::new()),
            approval_mode: ApprovalMode::AlwaysAsk,
            approval_mode_history: Vec::new(),
            thinking_level: None,
            model_menu,
            approval_menu,
            usage_menu,
            projects: Rc::new(Vec::new()),
            selected_project_id: initial_selected_project_id,
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
            queued_prompts: std::collections::HashMap::new(),
            agent_notices: std::collections::HashMap::new(),
            error_message: None,
            session_errors: std::collections::HashMap::new(),
            error_selection: TranscriptSelection::default(),
            error_generation: 0,
            zoomed_image: None,
            running_sessions: std::collections::HashMap::new(),
            session_run_tokens: std::collections::HashMap::new(),
            stream_render_pending: std::collections::HashMap::new(),
            sidebar_visible,
            sidebar_width,
            right_sidebar_visible,
            right_sidebar_width,
            right_sidebar_resize_start: None,
            right_sidebar_bottom_height,
            right_sidebar_bottom_resize_start: None,
            right_sidebar_bottom_collapsed,
            right_sidebar_terminals_by_cwd: std::collections::HashMap::new(),
            persisted_bottom_terminals,
            persisted_script_tabs,
            right_sidebar_bottom_run_selected: false,
            project_scripts_by_project: std::collections::HashMap::new(),
            project_script_streams: std::collections::HashMap::new(),
            project_script_stream_seq: 0,
            active_project_shortcuts: std::collections::HashMap::new(),
            inspector_active_tab: console_ui::InspectorTab::default(),
            inspector_open_auxiliary_tabs: layout.open_auxiliary_tabs(),
            browser_view: None,
            device_view: None,
            forwarded_ports_by_project: std::collections::HashMap::new(),
            ports_menu_handle: console_ui::ContextMenuHandle::new(cx),
            inspector_add_tab_menu: console_ui::ContextMenuHandle::new(cx),
            inspector_search_query: String::new(),
            inspector_tree: Rc::new(Vec::new()),
            inspector_working_changes: Rc::new(Vec::new()),
            inspector_fs_watch: None,
            inspector_git_watch: None,
            port_stream: None,
            fs_tree_fetch_pending: false,
            inspector_session_changes: Rc::new(Vec::new()),
            inspector_changes_scope: console_core::types::ChangesScope::default(),
            inspector_changes_scope_menu: console_ui::ContextMenuHandle::new(cx),
            changes_review_data: std::collections::HashMap::new(),
            changes_review_collapsed: std::collections::HashMap::new(),
            inspector_expanded_folders: Rc::new(std::collections::HashSet::new()),
            inspector_selected_path: None,
            session_subagents: std::collections::HashMap::new(),
            expanded_subagents: std::collections::HashSet::new(),
            subagent_markdown_views: Rc::new(RefCell::new(std::collections::HashMap::new())),
            preview_tab: None,
            open_file_contents: std::collections::HashMap::new(),
            open_image_contents: std::collections::HashMap::new(),
            svg_preview_mode: std::collections::HashMap::new(),
            open_diff_contents: std::collections::HashMap::new(),
            viewer_list_states: std::collections::HashMap::new(),
            viewer_scrollbar_states: std::collections::HashMap::new(),
            viewer_editor_views: std::collections::HashMap::new(),
            viewer_diff_views: std::collections::HashMap::new(),
            viewer_cached_markdown_views: std::collections::HashMap::new(),
            viewer_markdown_selections: std::collections::HashMap::new(),
            sidebar_list_state: ListState::new(0, ListAlignment::Top, px(55.0)),
            sidebar_resize_start: None,
            split_resize: None,
            saved_window_state: initial_saved_window_state,
            pending_window_state: None,
            last_window_poll: None,
            command_palette: cx.new(|cx| CommandPalette::new(window, cx)),
            tab_palette: cx.new(|cx| CommandPalette::new(window, cx)),
            quick_open_palette: cx
                .new(|cx| QuickOpenPalette::new(client_for_palettes.clone(), window, cx)),
            project_browse_palette: cx
                .new(|cx| ProjectBrowsePalette::new(client_for_palettes.clone(), window, cx)),
            terminals: std::collections::HashMap::new(),
            browser_views: std::collections::HashMap::new(),
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
            is_main_window: matches!(&target, crate::window::WindowLaunchTarget::RestorePersisted),
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
            sidebar_sort_mode: layout.sidebar_sort_mode(),
            collapsed_projects: Rc::new(layout.collapsed_projects.iter().cloned().collect()),
            is_window_active: window.is_window_active(),
            _subscriptions: subscriptions,
        };

        // Remaining font weights register after the first frame is presented,
        // keeping ~1.7MB of TTF parsing off the startup path (once per
        // process; secondary windows share the text system).
        window.on_next_frame(move |_, cx| {
            crate::assets::register_remaining_fonts_once(cx);
        });

        let inherited_environment_id = match target {
            crate::window::WindowLaunchTarget::Fresh { ref environment_id } => {
                environment_id.clone()
            }
            _ => None,
        };
        app.init_environments(store_doc.environments.clone(), inherited_environment_id, cx);
        app.refresh_auth_status(cx);
        app.init_notifications(cx);

        app.wire_palette_callbacks(cx);

        app.workspace_pane_states.insert(
            "pane-main".to_string(),
            WorkspacePaneState {
                transcript_view: app.transcript_view.clone(),
                composer_input: app.composer_input.clone(),
                question_input: app.question_input.clone(),
                selected_model: app.selected_model.clone(),
                active_picker_tab: app.active_picker_tab.clone(),
                approval_mode: app.approval_mode,
                approval_mode_history: Vec::new(),
                thinking_level: app.thinking_level,
                model_menu: app.model_menu.clone(),
                approval_menu: app.approval_menu.clone(),
                usage_menu: app.usage_menu.clone(),
                selected_project_id: app.selected_project_id.clone(),
                branches: app.branches.clone(),
                branch_loaded: app.branch_loaded,
                branch_is_git_repository: app.branch_is_git_repository,
                branch_pending: app.branch_pending,
                project_menu: app.project_menu.clone(),
                branch_menu: app.branch_menu.clone(),
                model_search,
                loaded_session_id: None,
                tab_strip_follow: console_ui::workspace::TabStripFollow::new(),
            },
        );

        app.bootstrap_backend(target, cx);

        // Port forwards can be created by the agent or another client. Keep the
        // title-bar popover synchronized without repeatedly probing the server.
        app.init_port_stream(cx);

        app
    }
}
