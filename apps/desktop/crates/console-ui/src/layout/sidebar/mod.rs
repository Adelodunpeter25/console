pub mod draft_item;
pub mod group_header;
mod item_row;
pub mod session_card;
pub mod session_item;

pub use draft_item::{DraftSummary, render_sidebar_draft_item};
pub use group_header::{
    drafts_group_header, group_header, pinned_group_header, project_header,
    search_results_group_header, settled_group_header, sort_mode_toggle,
};
pub use session_card::{
    SidebarHoverManager, dismiss_card, on_card_hover, on_row_hover, render_session_card_content,
};
pub use session_item::{
    CancelSessionRename, CommitSessionRename, SidebarSessionItem, init_session_rename_keybindings,
    render_sidebar_session_item,
};

gpui::actions!(console_sidebar_search, [CancelSidebarSearch]);

pub const SIDEBAR_SEARCH_PARENT_CONTEXT: &str = "ConsoleSidebarSearch";
pub const SIDEBAR_SEARCH_FIELD_CONTEXT: &str = "ConsoleSidebarSearch > ComposerInput";

pub fn init_sidebar_search_keybindings(cx: &mut App) {
    use gpui::KeyBinding;
    cx.bind_keys([KeyBinding::new(
        "escape",
        CancelSidebarSearch,
        Some(SIDEBAR_SEARCH_FIELD_CONTEXT),
    )]);
}

use console_core::{ProjectInfo, ProjectInfoExt, SessionHeader, SessionHeaderExt};
use gpui::{
    App, Entity, FontWeight, InteractiveElement, IntoElement, ListState, MouseButton,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div, list,
    prelude::FluentBuilder, px,
};
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use crate::input::ComposerInput;
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, MenuItem, dropdown_menu};
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::settings::EnvironmentRow;
use crate::theme::Theme;
use crate::utils::{
    ProjectSectionKey, SessionDateGroup, SidebarSortMode, group_indices_by_date,
    group_indices_by_project, partition_settled,
};

/// Collapsed-project key for sessions without a known project.
pub const NO_PROJECT_KEY: &str = "none";

/// Display project names as sentence case without changing the persisted name.
fn sentence_case_project_name(name: &str) -> String {
    let mut chars = name.chars();
    let Some(first) = chars.next() else {
        return String::new();
    };
    first.to_uppercase().chain(chars).collect()
}

#[derive(Clone)]
enum SidebarRow {
    Draft(usize),
    /// Position into the sidebar's shared session list. Carrying the index
    /// (not the header) keeps row construction allocation-free; only rows
    /// actually rendered by the virtualized list clone their header.
    Session(usize),
    /// Drafts section header inside the scroll list — only when the sticky
    /// top row is occupied by Pinned instead.
    DraftsHeader,
    /// Settled section header — always the last section of the list.
    SettledHeader,
    EmptySearch,
    GroupHeader {
        group: SessionDateGroup,
        collapsed: bool,
    },
    /// Project section header: display name plus the collapse key (project id
    /// or [`NO_PROJECT_KEY`]).
    ProjectHeader {
        name: String,
        key: String,
        collapsed: bool,
    },
}

#[derive(IntoElement)]
pub struct SidebarView {
    pub visible: bool,
    pub width: f32,
    /// Shared with the app state; cloned per frame as a refcount bump.
    pub sessions: Rc<Vec<SessionHeader>>,
    /// Known projects, used to resolve each session's project name. Shared
    /// with the app state; cloned per frame as a refcount bump.
    pub projects: Rc<Vec<ProjectInfo>>,
    pub selected_session_id: Option<String>,
    /// Date groups the user collapsed; their sessions are hidden.
    pub collapsed_groups: Rc<HashSet<SessionDateGroup>>,
    /// Which axis the session list is sectioned by.
    pub sort_mode: SidebarSortMode,
    /// Collapsed project sections (project id or [`NO_PROJECT_KEY`]).
    pub collapsed_projects: Rc<HashSet<String>>,
    /// Sessions with a locally active run, keyed by session id with the run's
    /// Unix-second start time. Each running chat row shows its own Working
    /// indicator — one per pane — instead of a single derived id that could
    /// attach the indicator to the wrong chat after a pane switch.
    pub running_sessions: HashMap<String, i64>,
    /// Sessions waiting for a permission or question response, keyed by session
    /// id. Each shows its own Waiting indicator.
    pub waiting_sessions: HashSet<String>,
    /// Unsent prompt drafts, each with preview and target session if any.
    pub draft_summaries: Vec<DraftSummary>,
    pub drafts_collapsed: bool,
    /// Whether the Pinned section is collapsed (rows hidden, sticky header
    /// chevron flipped).
    pub pinned_collapsed: bool,
    /// Whether the Settled section at the bottom of the list is collapsed.
    /// Settled chats hide behind it; the section is omitted when empty.
    pub settled_collapsed: bool,
    /// Retained list state is owned by the desktop app so it survives root
    /// renders caused by scrolling, status ticks, and transcript updates.
    pub sidebar_list_state: ListState,
    on_select_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_open_in_new_window: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_search: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_group: Rc<dyn Fn(SessionDateGroup, &mut Window, &mut App) + 'static>,
    on_toggle_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle_sort_mode: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_drafts: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_pinned: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_settled: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_rename_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_commit_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_pin_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_settle_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_unsettle_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle_read_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_delete_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_delete_draft: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    renaming_session_id: Option<String>,
    rename_input: Option<Entity<ComposerInput>>,
    on_resize_start: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    on_open_settings: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_open_server_settings: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub environments: Vec<EnvironmentRow>,
    pub server_menu: ContextMenuHandle,
    on_switch_environment: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub search_active: bool,
    pub search_query: String,
    pub search_input: Option<Entity<ComposerInput>>,
    pub on_start_search: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    pub on_clear_search: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    pub on_close_search: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl SidebarView {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        visible: bool,
        width: f32,
        sessions: Rc<Vec<SessionHeader>>,
        projects: Rc<Vec<ProjectInfo>>,
        selected_session_id: Option<String>,
        collapsed_groups: Rc<HashSet<SessionDateGroup>>,
        sort_mode: SidebarSortMode,
        collapsed_projects: Rc<HashSet<String>>,
        running_sessions: HashMap<String, i64>,
        waiting_sessions: HashSet<String>,
        draft_summaries: Vec<DraftSummary>,
        drafts_collapsed: bool,
        pinned_collapsed: bool,
        settled_collapsed: bool,
        sidebar_list_state: ListState,
        environments: Vec<EnvironmentRow>,
        server_menu: ContextMenuHandle,
        on_select_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_open_in_new_window: impl Fn(String, &mut Window, &mut App) + 'static,
        on_new_chat: impl Fn(&mut Window, &mut App) + 'static,
        on_search: impl Fn(&mut Window, &mut App) + 'static,
        on_add_project: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_group: impl Fn(SessionDateGroup, &mut Window, &mut App) + 'static,
        on_toggle_project: impl Fn(String, &mut Window, &mut App) + 'static,
        on_toggle_sort_mode: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_drafts: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_pinned: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_settled: impl Fn(&mut Window, &mut App) + 'static,
        on_rename_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_commit_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_cancel_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_pin_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_settle_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_unsettle_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_toggle_read_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_delete_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_delete_draft: impl Fn(String, &mut Window, &mut App) + 'static,
        renaming_session_id: Option<String>,
        rename_input: Option<Entity<ComposerInput>>,
        on_resize_start: impl Fn(f32, &mut Window, &mut App) + 'static,
        on_open_settings: impl Fn(&mut Window, &mut App) + 'static,
        on_open_server_settings: impl Fn(&mut Window, &mut App) + 'static,
        on_switch_environment: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            visible,
            width,
            sessions,
            projects,
            selected_session_id,
            collapsed_groups,
            sort_mode,
            collapsed_projects,
            running_sessions,
            waiting_sessions,
            draft_summaries,
            drafts_collapsed,
            pinned_collapsed,
            settled_collapsed,
            sidebar_list_state,
            on_select_session: Rc::new(on_select_session),
            on_open_in_new_window: Rc::new(on_open_in_new_window),
            on_new_chat: Rc::new(on_new_chat),
            on_search: Rc::new(on_search),
            on_add_project: Rc::new(on_add_project),
            on_toggle_group: Rc::new(on_toggle_group),
            on_toggle_project: Rc::new(on_toggle_project),
            on_toggle_sort_mode: Rc::new(on_toggle_sort_mode),
            on_toggle_drafts: Rc::new(on_toggle_drafts),
            on_toggle_pinned: Rc::new(on_toggle_pinned),
            on_toggle_settled: Rc::new(on_toggle_settled),
            on_rename_session: Rc::new(on_rename_session),
            on_commit_rename: Rc::new(on_commit_rename),
            on_cancel_rename: Rc::new(on_cancel_rename),
            on_toggle_pin_session: Rc::new(on_toggle_pin_session),
            on_settle_session: Rc::new(on_settle_session),
            on_unsettle_session: Rc::new(on_unsettle_session),
            on_toggle_read_session: Rc::new(on_toggle_read_session),
            on_delete_session: Rc::new(on_delete_session),
            on_delete_draft: Rc::new(on_delete_draft),
            renaming_session_id,
            rename_input,
            on_resize_start: Rc::new(on_resize_start),
            on_open_settings: Rc::new(on_open_settings),
            on_open_server_settings: Rc::new(on_open_server_settings),
            environments,
            server_menu,
            on_switch_environment: Rc::new(on_switch_environment),
            search_active: false,
            search_query: String::new(),
            search_input: None,
            on_start_search: None,
            on_clear_search: None,
            on_close_search: None,
        }
    }

    pub fn search(
        mut self,
        active: bool,
        query: String,
        input: Option<Entity<ComposerInput>>,
        on_start: impl Fn(&mut Window, &mut App) + 'static,
        on_clear: impl Fn(&mut Window, &mut App) + 'static,
        on_close: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.search_active = active;
        self.search_query = query;
        self.search_input = input;
        self.on_start_search = Some(Rc::new(on_start));
        self.on_clear_search = Some(Rc::new(on_clear));
        self.on_close_search = Some(Rc::new(on_close));
        self
    }
}

impl RenderOnce for SidebarView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        if !self.visible {
            return div().id("console-sidebar-hidden").w(px(0.0)).h_full();
        }

        let theme = Theme::current(cx);
        let projects = self.projects;
        let selected_id = self.selected_session_id;
        let has_sessions = !self.sessions.is_empty();
        let collapsed_groups = self.collapsed_groups;
        let sort_mode = self.sort_mode;
        let collapsed_projects = self.collapsed_projects;
        let running_sessions = self.running_sessions;
        let waiting_sessions = self.waiting_sessions;
        let draft_summaries = self.draft_summaries;
        let draft_sessions: HashSet<String> = draft_summaries
            .iter()
            .filter_map(|d| d.session_id.clone())
            .collect();
        let on_new = self.on_new_chat;
        let on_search = self.on_search;
        let on_add = self.on_add_project;
        let on_toggle_group = self.on_toggle_group;
        let on_toggle_project = self.on_toggle_project;
        let on_toggle_sort_mode = self.on_toggle_sort_mode;
        let on_sel = self.on_select_session;
        let on_open_in_new_window = self.on_open_in_new_window;
        let on_rename = self.on_rename_session;
        let on_commit_rename = self.on_commit_rename;
        let on_cancel_rename = self.on_cancel_rename;
        let on_toggle_pin = self.on_toggle_pin_session;
        let on_del = self.on_delete_session;
        let on_delete_draft = self.on_delete_draft;
        let renaming_session_id = self.renaming_session_id;
        let rename_input = self.rename_input;
        let on_resize_start = self.on_resize_start;
        let on_open_settings = self.on_open_settings;
        let on_open_server_settings = self.on_open_server_settings;
        let environments = self.environments;
        let server_menu = self.server_menu;
        let on_switch_env = self.on_switch_environment;
        let width = self.width;

        // Filter out sessions that have drafts so they appear exclusively in the
        // Drafts section without duplicating in date groups below. Session
        // timestamps resolve once here so both grouping modes share them.
        let sessions = self.sessions;
        // Settled chats leave every active section (pinned, drafts, date,
        // project) and gather in their own Settled section at the bottom.
        let is_settled = |index: usize| sessions[index].settled_at.is_some();
        let settled_at = |index: usize| sessions[index].settled_at.unwrap_or(0);
        let (active_positions, settled_positions) =
            partition_settled(sessions.len(), is_settled, settled_at);
        let active: HashSet<usize> = active_positions.iter().copied().collect();
        let has_settled = !settled_positions.is_empty();
        let session_timestamp = |index: usize| {
            let session = &sessions[index];
            if draft_sessions.contains(&session.id) {
                0
            } else {
                session.sort_timestamp()
            }
        };
        let is_pinned = |index: usize| sessions[index].pinned_at.is_some();
        let pinned_positions: Vec<usize> = (0..sessions.len())
            .filter(|&idx| {
                is_pinned(idx)
                    && !draft_sessions.contains(&sessions[idx].id)
                    && active.contains(&idx)
            })
            .collect();
        let grouped: Vec<(SessionDateGroup, Vec<usize>)> =
            group_indices_by_date(sessions.len(), session_timestamp)
                .into_iter()
                .map(|(group, positions)| {
                    let filtered: Vec<usize> = positions
                        .into_iter()
                        .filter(|&idx| {
                            !draft_sessions.contains(&sessions[idx].id)
                                && !is_pinned(idx)
                                && active.contains(&idx)
                        })
                        .collect();
                    (group, filtered)
                })
                .filter(|(_, positions)| !positions.is_empty())
                .collect();

        // Project sections for project sort mode: the collapse key is the
        // project id (or "none" for sessions without one); the display name
        // resolves against the project list. Ordered by most recent activity.
        let visible_positions: HashSet<usize> = grouped
            .iter()
            .flat_map(|(_, positions)| positions.iter().copied())
            .collect();
        let project_key = |index: usize| -> ProjectSectionKey {
            let session = &sessions[index];
            projects
                .iter()
                .find(|project| project.matches_session(session))
                .map(|project| project.id.clone())
        };
        let project_name_for = |key: &ProjectSectionKey| -> String {
            match key {
                Some(id) => projects
                    .iter()
                    .find(|project| &project.id == id)
                    .map(|project| sentence_case_project_name(&project.name))
                    .unwrap_or_else(|| "Unknown Project".to_string()),
                None => "No Project".to_string(),
            }
        };
        let project_sections: Vec<(ProjectSectionKey, String, Vec<usize>)> =
            group_indices_by_project(sessions.len(), project_key, session_timestamp)
                .into_iter()
                .map(|(key, positions)| {
                    let filtered: Vec<usize> = positions
                        .into_iter()
                        .filter(|idx| visible_positions.contains(idx))
                        .collect();
                    let name = project_name_for(&key);
                    (key, name, filtered)
                })
                .filter(|(_, _, positions)| !positions.is_empty())
                .collect();
        let collapse_key = |key: &ProjectSectionKey| -> String {
            key.clone().unwrap_or_else(|| NO_PROJECT_KEY.to_string())
        };

        let has_drafts = !draft_summaries.is_empty();
        let has_pinned = !pinned_positions.is_empty();
        let pinned_collapsed = self.pinned_collapsed;

        // The sticky header row above the list. When active search query is
        // non-empty, the single Search Results header replaces all section headers.
        let is_searching = self.search_active && !self.search_query.trim().is_empty();
        let sort_toggle = sort_mode_toggle(theme, sort_mode, on_toggle_sort_mode.clone());
        let pinned_header = if is_searching {
            search_results_group_header(theme, "Search Results").into_any_element()
        } else if has_pinned {
            div()
                .flex()
                .items_center()
                .justify_between()
                .child(
                    pinned_group_header(
                        theme,
                        pinned_collapsed,
                        false,
                        on_add.clone(),
                        self.on_toggle_pinned.clone(),
                    )
                    .into_any_element(),
                )
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(4.0))
                        .child(sort_toggle.into_any_element())
                        .child(
                            div()
                                .id("btn-add-project")
                                .w(px(20.0))
                                .h(px(20.0))
                                .rounded(px(6.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_default()
                                .hover(|s| s.bg(theme.overlay))
                                .active(|s| s.bg(theme.overlay_strong))
                                .on_click({
                                    let on_add = on_add.clone();
                                    move |_, window, cx| (on_add)(window, cx)
                                })
                                .tooltip(Tooltip::text("Add project"))
                                .child(app_icon(IconName::FolderNew, 15.0, theme.text_ghost)),
                        ),
                )
                .into_any_element()
        } else if has_drafts {
            div()
                .flex()
                .items_center()
                .justify_between()
                .child(
                    drafts_group_header(
                        theme,
                        self.drafts_collapsed,
                        false,
                        on_add.clone(),
                        self.on_toggle_drafts.clone(),
                    )
                    .into_any_element(),
                )
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(4.0))
                        .child(sort_toggle.into_any_element())
                        .child(
                            div()
                                .id("btn-add-project")
                                .w(px(20.0))
                                .h(px(20.0))
                                .rounded(px(6.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_default()
                                .hover(|s| s.bg(theme.overlay))
                                .active(|s| s.bg(theme.overlay_strong))
                                .on_click({
                                    let on_add = on_add.clone();
                                    move |_, window, cx| (on_add)(window, cx)
                                })
                                .tooltip(Tooltip::text("Add project"))
                                .child(app_icon(IconName::FolderNew, 15.0, theme.text_ghost)),
                        ),
                )
                .into_any_element()
        } else {
            match sort_mode {
                SidebarSortMode::Date => {
                    let first_group = grouped
                        .first()
                        .map(|(g, _)| *g)
                        .unwrap_or(SessionDateGroup::Today);
                    div()
                        .flex()
                        .items_center()
                        .justify_between()
                        .child(
                            group_header(
                                theme,
                                first_group.label(),
                                collapsed_groups.contains(&first_group),
                                false,
                                on_add.clone(),
                                on_toggle_group.clone(),
                                first_group,
                            )
                            .into_any_element(),
                        )
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .gap(px(4.0))
                                .child(sort_toggle.into_any_element())
                                .child(
                                    div()
                                        .id("btn-add-project")
                                        .w(px(20.0))
                                        .h(px(20.0))
                                        .rounded(px(6.0))
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .cursor_default()
                                        .hover(|s| s.bg(theme.overlay))
                                        .active(|s| s.bg(theme.overlay_strong))
                                        .on_click({
                                            let on_add = on_add.clone();
                                            move |_, window, cx| (on_add)(window, cx)
                                        })
                                        .child(app_icon(
                                            IconName::FolderNew,
                                            15.0,
                                            theme.text_ghost,
                                        )),
                                ),
                        )
                        .into_any_element()
                }
                SidebarSortMode::Project => {
                    let (first_key, first_name) = project_sections
                        .first()
                        .map(|(key, name, _)| (collapse_key(key), name.clone()))
                        .unwrap_or_else(|| (NO_PROJECT_KEY.to_string(), "No Project".to_string()));
                    let first_collapsed = collapsed_projects.contains(&first_key);
                    div()
                        .flex()
                        .items_center()
                        .justify_between()
                        .child(
                            project_header(
                                theme,
                                first_name,
                                first_key,
                                first_collapsed,
                                false,
                                on_add.clone(),
                                on_toggle_project.clone(),
                            )
                            .into_any_element(),
                        )
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .gap(px(4.0))
                                .child(sort_toggle.into_any_element())
                                .child(
                                    div()
                                        .id("btn-add-project")
                                        .w(px(20.0))
                                        .h(px(20.0))
                                        .rounded(px(6.0))
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .cursor_default()
                                        .hover(|s| s.bg(theme.overlay))
                                        .active(|s| s.bg(theme.overlay_strong))
                                        .on_click({
                                            let on_add = on_add.clone();
                                            move |_, window, cx| (on_add)(window, cx)
                                        })
                                        .child(app_icon(
                                            IconName::FolderNew,
                                            15.0,
                                            theme.text_ghost,
                                        )),
                                ),
                        )
                        .into_any_element()
                }
            }
        };

        // Scrollable rows: when search is active with a non-empty query, shows
        // matching sessions directly under the Search Results header, preserving
        // the current sort mode's relative ordering. When query is empty (or search
        // is inactive), shows the normal pinned, drafts, and date/project sections.
        let mut list_rows = Vec::new();
        if is_searching {
            let mut ordered_positions: Vec<usize> = Vec::new();
            ordered_positions.extend(pinned_positions.iter().copied());
            match sort_mode {
                SidebarSortMode::Date => {
                    for (_, positions) in &grouped {
                        ordered_positions.extend(positions.iter().copied());
                    }
                }
                SidebarSortMode::Project => {
                    for (_, _, positions) in &project_sections {
                        ordered_positions.extend(positions.iter().copied());
                    }
                }
            }
            ordered_positions.extend(settled_positions.iter().copied());
            let in_ordered: HashSet<usize> = ordered_positions.iter().copied().collect();
            for idx in 0..sessions.len() {
                if !in_ordered.contains(&idx) {
                    ordered_positions.push(idx);
                }
            }

            let query_lower = self.search_query.trim().to_lowercase();
            let matching_positions: Vec<usize> = ordered_positions
                .into_iter()
                .filter(|&idx| {
                    let session = &sessions[idx];
                    session
                        .display_title()
                        .to_lowercase()
                        .contains(&query_lower)
                        || session.title.to_lowercase().contains(&query_lower)
                })
                .collect();

            if matching_positions.is_empty() {
                list_rows.push(SidebarRow::EmptySearch);
            } else {
                list_rows.extend(matching_positions.into_iter().map(SidebarRow::Session));
            }
        } else {
            let render_all_headers = has_drafts || has_pinned;
            let push_section_rows = |list_rows: &mut Vec<SidebarRow>,
                                     sections: Vec<(SessionDateGroup, Vec<usize>)>,
                                     render_all: bool| {
                if render_all {
                    for (group, positions) in sections {
                        let collapsed = collapsed_groups.contains(&group);
                        list_rows.push(SidebarRow::GroupHeader { group, collapsed });
                        if !collapsed {
                            list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                        }
                    }
                } else {
                    let mut sections_iter = sections.into_iter();
                    if let Some((group, positions)) = sections_iter.next()
                        && !collapsed_groups.contains(&group)
                    {
                        list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                    }
                    for (group, positions) in sections_iter {
                        let collapsed = collapsed_groups.contains(&group);
                        list_rows.push(SidebarRow::GroupHeader { group, collapsed });
                        if !collapsed {
                            list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                        }
                    }
                }
            };
            if has_pinned {
                if !pinned_collapsed {
                    list_rows.extend(pinned_positions.iter().copied().map(SidebarRow::Session));
                }
                if has_drafts {
                    list_rows.push(SidebarRow::DraftsHeader);
                    if !self.drafts_collapsed {
                        for i in 0..draft_summaries.len() {
                            list_rows.push(SidebarRow::Draft(i));
                        }
                    }
                }
            } else if has_drafts && !self.drafts_collapsed {
                for i in 0..draft_summaries.len() {
                    list_rows.push(SidebarRow::Draft(i));
                }
            }
            match sort_mode {
                SidebarSortMode::Date => {
                    push_section_rows(&mut list_rows, grouped, render_all_headers);
                }
                SidebarSortMode::Project => {
                    let sections: Vec<(String, String, Vec<usize>)> = project_sections
                        .into_iter()
                        .map(|(key, name, positions)| (collapse_key(&key), name, positions))
                        .collect();
                    if render_all_headers {
                        for (key, name, positions) in sections {
                            let collapsed = collapsed_projects.contains(&key);
                            list_rows.push(SidebarRow::ProjectHeader {
                                name,
                                key,
                                collapsed,
                            });
                            if !collapsed {
                                list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                            }
                        }
                    } else {
                        let mut sections_iter = sections.into_iter();
                        if let Some((key, _name, positions)) = sections_iter.next()
                            && !collapsed_projects.contains(&key)
                        {
                            list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                        }
                        for (key, name, positions) in sections_iter {
                            let collapsed = collapsed_projects.contains(&key);
                            list_rows.push(SidebarRow::ProjectHeader {
                                name,
                                key,
                                collapsed,
                            });
                            if !collapsed {
                                list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                            }
                        }
                    }
                }
            }
            // Settled chats always come last, in their own collapsed-by-default
            // section, most recently settled first.
            if has_settled {
                list_rows.push(SidebarRow::SettledHeader);
                if !self.settled_collapsed {
                    list_rows.extend(settled_positions.iter().copied().map(SidebarRow::Session));
                }
            }
        }
        let list_rows = Rc::new(list_rows);
        let sidebar_list_state = self.sidebar_list_state;
        if sidebar_list_state.item_count() != list_rows.len() {
            sidebar_list_state.reset_with_uniform_height(list_rows.len(), px(55.0));
        }

        let list_sessions = sessions;
        let list_projects = projects;
        let list_selected_id = selected_id;
        let list_running_sessions = running_sessions;
        let list_waiting_sessions = waiting_sessions;
        let list_draft_sessions = draft_sessions;
        let list_draft_summaries = draft_summaries;
        let list_on_new = on_new.clone();
        let list_on_sel = on_sel;
        let list_on_open_in_new_window = on_open_in_new_window;
        let list_on_rename = on_rename;
        let list_on_commit_rename = on_commit_rename;
        let list_on_cancel_rename = on_cancel_rename;
        let list_on_toggle_pin = on_toggle_pin;
        let list_on_del = on_del;
        let list_on_delete_draft = on_delete_draft;
        let list_renaming_session_id = renaming_session_id;
        let list_rename_input = rename_input;
        let list_on_add = on_add.clone();
        let list_on_toggle_group = on_toggle_group.clone();
        let list_on_toggle_project = on_toggle_project.clone();
        let list_drafts_collapsed = self.drafts_collapsed;
        let list_on_toggle_drafts = self.on_toggle_drafts;
        let list_settled_collapsed = self.settled_collapsed;
        let list_on_toggle_settled = self.on_toggle_settled;
        let list_on_settle = self.on_settle_session;
        let list_on_unsettle = self.on_unsettle_session;
        let list_on_toggle_read = self.on_toggle_read_session;
        let grouped_rows = list(sidebar_list_state, move |index, window, cx| {
            let Some(row) = list_rows.get(index).cloned() else {
                return div().into_any_element();
            };
            let row = match row {
                SidebarRow::Draft(draft_index) => {
                    let Some(draft) = list_draft_summaries.get(draft_index) else {
                        return div().into_any_element();
                    };
                    render_sidebar_draft_item(
                        draft,
                        list_selected_id.as_deref(),
                        &list_on_sel,
                        &list_on_new,
                        &list_on_delete_draft,
                        theme,
                        window,
                        cx,
                    )
                }
                SidebarRow::Session(session_index) => {
                    // Clone only for rows the virtualized list actually
                    // paints; positions were grouped without touching data.
                    let Some(session) = list_sessions.get(session_index).cloned() else {
                        return div().into_any_element();
                    };
                    // Resolve this row's own running/waiting/draft state from the
                    // per-session maps so each chat shows its own indicator.
                    let running_started_at = list_running_sessions.get(&session.id).copied();
                    let is_waiting = list_waiting_sessions.contains(&session.id);
                    let has_draft = list_draft_sessions.contains(&session.id);
                    render_sidebar_session_item(
                        session,
                        &list_projects,
                        list_selected_id.as_deref(),
                        running_started_at,
                        is_waiting,
                        has_draft,
                        &list_on_sel,
                        &list_on_open_in_new_window,
                        &list_on_rename,
                        &list_on_commit_rename,
                        &list_on_cancel_rename,
                        &list_on_toggle_pin,
                        &list_on_settle,
                        &list_on_unsettle,
                        &list_on_toggle_read,
                        &list_on_del,
                        list_renaming_session_id.as_deref(),
                        list_rename_input.clone(),
                    )
                }
                SidebarRow::DraftsHeader => drafts_group_header(
                    theme,
                    list_drafts_collapsed,
                    false,
                    list_on_add.clone(),
                    list_on_toggle_drafts.clone(),
                )
                .into_any_element(),
                SidebarRow::SettledHeader => settled_group_header(
                    theme,
                    list_settled_collapsed,
                    list_on_toggle_settled.clone(),
                )
                .into_any_element(),
                SidebarRow::GroupHeader { group, collapsed } => group_header(
                    theme,
                    group.label(),
                    collapsed,
                    false,
                    list_on_add.clone(),
                    list_on_toggle_group.clone(),
                    group,
                )
                .into_any_element(),
                SidebarRow::ProjectHeader {
                    name,
                    key,
                    collapsed,
                } => project_header(
                    theme,
                    name,
                    key,
                    collapsed,
                    false,
                    list_on_add.clone(),
                    list_on_toggle_project.clone(),
                )
                .into_any_element(),
                SidebarRow::EmptySearch => div()
                    .px(px(10.0))
                    .py(px(12.0))
                    .text_size(px(12.5))
                    .text_color(theme.text_tertiary)
                    .child("No matching sessions")
                    .into_any_element(),
            };
            div().w_full().pb(px(2.0)).child(row).into_any_element()
        })
        .size_full();

        let on_outside = self
            .on_close_search
            .clone()
            .or_else(|| self.on_clear_search.clone());

        div()
            .id("console-sidebar")
            .w(px(width))
            .h_full()
            .relative()
            .px(px(12.0))
            .bg(theme.sidebar)
            .border_r_1()
            .border_color(theme.sidebar_border)
            .flex()
            .flex_col()
            .when(self.search_active, |el| {
                el.when_some(on_outside, |el, on_outside| {
                    el.on_mouse_down_out(move |_, window, cx| {
                        (on_outside)(window, cx);
                    })
                })
            })
            // Action Rows (New Chat & Search) — the unified window title bar
            // owns the sidebar toggle now, so the sidebar starts here.
            .child(
                div()
                    .pt(px(4.0))
                    .pb(px(8.0))
                    .flex()
                    .flex_col()
                    .gap_y(px(4.0))
                    // New Chat Row
                    .child(
                        div()
                            .id("btn-new-chat")
                            .tab_index(0)
                            .w_full()
                            .h(px(32.0))
                            .px(px(6.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .gap(px(10.0))
                            .cursor_default()
                            .hover(|s| s.bg(theme.sidebar_item_background))
                            .active(|s| s.bg(theme.overlay_strong))
                            .on_click(move |_, window, cx| {
                                (on_new)(window, cx);
                            })
                            .child(
                                div()
                                    .size(px(20.0))
                                    .flex_none()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .child(app_icon(IconName::Compose, 16.0, theme.text_secondary)),
                            )
                            .child(
                                div()
                                    .min_w_0()
                                    .truncate()
                                    .text_size(px(13.0))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(theme.text_secondary)
                                    .child("New Task"),
                            ),
                    )
                    // Search Row
                    .child(if self.search_active {
                        let on_cancel = self.on_clear_search.clone();
                        let on_clear = self.on_clear_search.clone();
                        let has_query = !self.search_query.trim().is_empty();
                        let clear_tooltip = if has_query {
                            "Clear search"
                        } else {
                            "Close search (Esc)"
                        };
                        div()
                            .id("sidebar-search-active")
                            .key_context(SIDEBAR_SEARCH_PARENT_CONTEXT)
                            .w_full()
                            .h(px(32.0))
                            .px(px(6.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .gap(px(10.0))
                            .text_size(px(13.0))
                            .when_some(on_cancel, |el, on_cancel| {
                                el.on_action(move |_: &CancelSidebarSearch, window, cx| {
                                    (on_cancel)(window, cx);
                                    cx.stop_propagation();
                                })
                            })
                            .child(
                                div()
                                    .size(px(20.0))
                                    .flex_none()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .child(app_icon(IconName::Search, 16.0, theme.text_secondary)),
                            )
                            .child(
                                div()
                                    .flex_1()
                                    .min_w_0()
                                    .text_size(px(13.0))
                                    .children(self.search_input.clone()),
                            )
                            .child(
                                div()
                                    .id("btn-clear-search")
                                    .size(px(18.0))
                                    .flex_none()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .rounded(px(4.0))
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .active(|s| s.bg(theme.overlay_strong))
                                    .when_some(on_clear, |el, on_clear| {
                                        el.on_click(move |_, window, cx| {
                                            (on_clear)(window, cx);
                                        })
                                    })
                                    .tooltip(Tooltip::text(clear_tooltip))
                                    .child(app_icon(IconName::X, 12.0, theme.text_tertiary)),
                            )
                    } else {
                        let on_start = self.on_start_search.clone().unwrap_or(on_search);
                        div()
                            .id("btn-search")
                            .tab_index(0)
                            .w_full()
                            .h(px(32.0))
                            .px(px(6.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .cursor_default()
                            .hover(|s| s.bg(theme.sidebar_item_background))
                            .active(|s| s.bg(theme.overlay_strong))
                            .on_click(move |_, window, cx| {
                                (on_start)(window, cx);
                            })
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(10.0))
                                    .child(
                                        div()
                                            .size(px(20.0))
                                            .flex_none()
                                            .flex()
                                            .items_center()
                                            .justify_center()
                                            .child(app_icon(
                                                IconName::Search,
                                                16.0,
                                                theme.text_secondary,
                                            )),
                                    )
                                    .child(
                                        div()
                                            .min_w_0()
                                            .truncate()
                                            .text_size(px(13.0))
                                            .text_color(theme.text_secondary)
                                            .child("Search"),
                                    ),
                            )
                    }),
            )
            // Pinned header row: the first group's header (Today) with the
            // add-project button, kept OUTSIDE the scroll container so it
            // never moves when groups collapse or expand.
            .child(pinned_header)
            // Session List Container: the remaining groups' headers and all
            // sessions live here and scroll; toggling a group only changes
            // this container's content, never the pinned row above. The
            // sidebar's `px(12)` inset applies left and right to every row.
            .child(
                div()
                    .id("sidebar-session-list")
                    .flex_1()
                    .min_h(px(0.0))
                    .relative()
                    .child(grouped_rows)
                    .when(!has_sessions, |el| {
                        el.child(
                            div()
                                .absolute()
                                .inset_0()
                                .flex()
                                .flex_col()
                                .items_center()
                                .justify_center()
                                .gap_y(px(6.0))
                                .child(app_icon(IconName::Folder, 20.0, theme.text_ghost))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .text_color(theme.text_tertiary)
                                        .child("No tasks yet"),
                                ),
                        )
                    }),
            )
            // Footer: Settings + Server Dropdown
            .child(
                div()
                    .flex_none()
                    .h(px(40.0))
                    .px(px(2.0))
                    .flex()
                    .items_center()
                    .justify_between()
                    .child({
                        let on_open_settings = on_open_settings.clone();
                        div()
                            .id("open-settings")
                            .tab_index(0)
                            .w(px(26.0))
                            .h(px(26.0))
                            .rounded(px(6.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .active(|s| s.bg(theme.overlay_strong))
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_open_settings)(window, cx);
                            })
                            .tooltip(Tooltip::text("Settings"))
                            .child(app_icon(IconName::Settings, 15.0, theme.text_tertiary))
                    })
                    .child({
                        let server_chip = div()
                            .id("open-server-picker")
                            .tab_index(0)
                            .w(px(26.0))
                            .h(px(26.0))
                            .rounded(px(6.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .active(|s| s.bg(theme.overlay_strong))
                            .tooltip(Tooltip::text("Servers"))
                            .child(app_icon(IconName::Server, 15.0, theme.text_tertiary));

                        let on_open_server_settings = on_open_server_settings.clone();
                        dropdown_menu(
                            server_chip,
                            "sidebar-server-menu",
                            &server_menu,
                            MenuAlign::AboveRight,
                            move |_| {
                                let mut items = environments
                                    .iter()
                                    .map(|env| {
                                        let env_id = env.id.clone();
                                        let on_switch = on_switch_env.clone();
                                        let is_active = env.is_active;
                                        let label = env.name.clone();
                                        MenuItem::new(label, move |window, cx| {
                                            (on_switch)(env_id.clone(), window, cx);
                                        })
                                        .icon(IconName::Server.path())
                                        .selected(is_active)
                                    })
                                    .collect::<Vec<_>>();

                                if !items.is_empty() {
                                    items.push(MenuItem::Separator);
                                }
                                let on_open_server_settings = on_open_server_settings.clone();
                                items.push(
                                    MenuItem::new("Server Settings…", move |window, cx| {
                                        (on_open_server_settings)(window, cx);
                                    })
                                    .icon(IconName::Settings.path()),
                                );
                                items
                            },
                        )
                    }),
            )
            // The divider is also the resize target. Keep the hit area a few
            // pixels wide without changing the visible border position.
            .child(
                div()
                    .id("sidebar-resize-handle")
                    .absolute()
                    .top_0()
                    .bottom_0()
                    .right(px(-3.0))
                    .w(px(6.0))
                    .cursor_e_resize()
                    .on_mouse_down(MouseButton::Left, move |event, window, cx| {
                        cx.stop_propagation();
                        (on_resize_start)(f32::from(event.position.x), window, cx);
                    }),
            )
    }
}
