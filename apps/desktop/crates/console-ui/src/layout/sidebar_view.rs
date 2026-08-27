use console_core::{ProjectInfo, SessionHeader};
use gpui::{
    App, AppContext, ElementId, Entity, FontWeight, InteractiveElement, IntoElement, KeyDownEvent,
    ListState, MouseButton, ParentElement, RenderOnce, SharedString, StatefulInteractiveElement,
    Styled, Window, actions, div, list, prelude::FluentBuilder, px,
};
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use crate::input::ComposerInput;
use crate::layout::sidebar_loading::{self as sidebar, SidebarLoadingState};
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, MenuItem, dropdown_menu};
use crate::primitives::{IconName, app_icon, session_context_menu};
use crate::settings::EnvironmentRow;
use crate::theme::Theme;
use crate::utils::{SessionDateGroup, format_time_ago, group_indices_by_date};
use crate::workspace::{WorkspaceDrag, WorkspaceDragPreview};

actions!(
    console_sidebar_rename,
    [CommitSessionRename, CancelSessionRename]
);

const SESSION_RENAME_PARENT_CONTEXT: &str = "ConsoleSessionRename";
const SESSION_RENAME_FIELD_CONTEXT: &str = "ConsoleSessionRename > ComposerInput";

/// Bind rename actions after the generic ComposerInput bindings so Enter here
/// commits the inline editor instead of submitting the composer action.
pub fn init_session_rename_keybindings(cx: &mut App) {
    use gpui::KeyBinding;
    cx.bind_keys([
        KeyBinding::new(
            "enter",
            CommitSessionRename,
            Some(SESSION_RENAME_FIELD_CONTEXT),
        ),
        KeyBinding::new(
            "escape",
            CancelSessionRename,
            Some(SESSION_RENAME_FIELD_CONTEXT),
        ),
    ]);
}

#[derive(IntoElement)]
pub struct SidebarSessionItem {
    session: SessionHeader,
    /// The project this session belongs to, when the app knows it. Falls back
    /// to the session's working-directory name in the UI.
    project_name: Option<String>,
    is_active: bool,
    loading_state: Option<SidebarLoadingState>,
    has_draft: bool,
    is_renaming: bool,
    rename_input: Option<Entity<ComposerInput>>,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_commit_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_delete: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl SidebarSessionItem {
    pub fn new(
        session: SessionHeader,
        project_name: Option<String>,
        is_active: bool,
        loading_state: Option<SidebarLoadingState>,
        has_draft: bool,
        is_renaming: bool,
        rename_input: Option<Entity<ComposerInput>>,
        on_click: impl Fn(&mut Window, &mut App) + 'static,
        on_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_commit_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_cancel_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_delete: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            session,
            project_name,
            is_active,
            loading_state,
            has_draft,
            is_renaming,
            rename_input,
            on_click: Rc::new(on_click),
            on_rename: Rc::new(on_rename),
            on_commit_rename: Rc::new(on_commit_rename),
            on_cancel_rename: Rc::new(on_cancel_rename),
            on_delete: Rc::new(on_delete),
        }
    }
}

impl RenderOnce for SidebarSessionItem {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let session = &self.session;
        let is_active = self.is_active;
        let on_click = self.on_click;
        let on_rename = self.on_rename;
        let on_commit_rename = self.on_commit_rename;
        let on_cancel_rename = self.on_cancel_rename;
        let on_delete = self.on_delete;

        let loading_state = self.loading_state;
        let is_renaming = self.is_renaming;
        let rename_input = self.rename_input;

        let mut folder_name = self.project_name.clone().unwrap_or_else(|| {
            session
                .cwd
                .split(['/', '\\'])
                .filter(|s| !s.is_empty())
                .last()
                .unwrap_or("workspace")
                .to_string()
        });
        // Directory names arrive lowercase from the filesystem; sentence-case
        // the label so it reads like a title ("console" → "Console").
        if let Some(first) = folder_name.get_mut(..1) {
            first.make_ascii_uppercase();
        }

        let display_title = if session.title.trim().is_empty() {
            "New Chat".to_string()
        } else {
            session.title.clone()
        };
        let drag = WorkspaceDrag::new(
            console_core::WorkspaceTabConfig::Chat {
                session_id: session.id.clone(),
                title: display_title.clone(),
                project_id: session.project_id.clone(),
            },
            None,
        );

        let group_name = format!("sidebar-session-{}", session.id);
        let delete_button_action = on_delete.clone();
        let title = if let Some(rename_input) = rename_input {
            let on_commit = on_commit_rename.clone();
            let on_cancel = on_cancel_rename.clone();
            div()
                .id(ElementId::Name(
                    format!("session-rename-{}", session.id).into(),
                ))
                .key_context(SESSION_RENAME_PARENT_CONTEXT)
                .h(px(22.0))
                .flex_1()
                .min_w_0()
                .px(px(4.0))
                .rounded(px(5.0))
                .border_1()
                .border_color(theme.accent)
                .bg(theme.inset)
                .flex()
                .items_center()
                .on_action({
                    let on_commit = on_commit.clone();
                    move |_: &CommitSessionRename, window, cx| {
                        (on_commit)(window, cx);
                        cx.stop_propagation();
                    }
                })
                .on_action(move |_: &CancelSessionRename, window, cx| {
                    (on_cancel)(window, cx);
                    cx.stop_propagation();
                })
                .child(rename_input)
                .into_any_element()
        } else {
            div()
                .flex_1()
                .min_w_0()
                .truncate()
                .text_size(px(13.5))
                .font_weight(if is_active {
                    FontWeight::SEMIBOLD
                } else {
                    FontWeight::MEDIUM
                })
                .text_color(if is_active {
                    theme.text
                } else {
                    theme.text_secondary
                })
                .child(display_title.clone())
                .into_any_element()
        };

        let row = div()
            .id(ElementId::Name(session.id.clone().into()))
            .relative()
            .w_full()
            .min_w_0()
            .h(px(53.0))
            .px(px(12.0))
            .py(px(7.0))
            .rounded(px(7.0))
            .cursor_default()
            .on_drag(drag, |drag, _, _, cx| {
                cx.new(|_| WorkspaceDragPreview::new(drag.tab.title()))
            })
            .flex()
            .flex_col()
            .gap_y(px(6.0))
            .justify_between()
            .group(group_name.clone())
            .when(is_active, |s| s.bg(theme.sidebar_item_background))
            .when(!is_active, |s| {
                s.hover(|h| h.bg(theme.sidebar_item_background))
            })
            .when(!is_renaming, |element| {
                element.on_click(move |_, w, cx| {
                    (on_click)(w, cx);
                })
            })
            // Top Row: Title + Status Icon + Hover Delete
            .child(
                div()
                    .w_full()
                    .flex()
                    .items_center()
                    .justify_between()
                    .gap_x(px(6.0))
                    .overflow_hidden()
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .flex()
                            .items_center()
                            .gap_x(px(6.0))
                            .child(title)
                            .when(self.has_draft, |el| {
                                el.child(
                                    div()
                                        .flex_none()
                                        .px(px(4.0))
                                        .py(px(0.5))
                                        .rounded(px(3.0))
                                        .bg(theme.accent.opacity(0.15))
                                        .text_color(theme.accent)
                                        .text_size(px(10.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .child("Draft"),
                                )
                            })
                            .when_some(
                                loading_state
                                    .and_then(|state| sidebar::status_indicator(state, theme)),
                                |el, indicator| el.child(indicator),
                            ),
                    )
                    // Delete Button: hidden until hover, then reveals on
                    // the right side of the card.
                    .child(
                        div()
                            .id(ElementId::Name(format!("del-{}", session.id).into()))
                            .p(px(3.0))
                            .rounded(px(4.0))
                            .cursor_default()
                            .invisible()
                            .group_hover(group_name.clone(), |el| el.visible())
                            .hover(|s| s.bg(theme.danger_soft))
                            .on_click(move |_, w, cx| {
                                cx.stop_propagation();
                                (delete_button_action)(w, cx);
                            })
                            .child(app_icon(
                                IconName::TrashBinMinimalistic,
                                11.0,
                                theme.text_ghost,
                            )),
                    ),
            )
            // Bottom Row: Folder icon + Working Dir Name + Time
            .child(
                div()
                    .w_full()
                    .flex()
                    .items_center()
                    .justify_between()
                    .gap_x(px(5.0))
                    .text_size(px(11.5))
                    .line_height(px(15.0))
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .flex()
                            .items_center()
                            .gap_x(px(4.0))
                            .child(app_icon(IconName::Folder, 11.0, theme.text_tertiary))
                            .child(
                                div()
                                    .flex_1()
                                    .truncate()
                                    .text_color(theme.text_tertiary)
                                    .child(folder_name),
                            ),
                    )
                    .child(
                        div()
                            .flex_none()
                            .text_color(if loading_state.is_some() {
                                theme.text_tertiary
                            } else {
                                theme.text_ghost
                            })
                            .child(
                                loading_state
                                    .and_then(|state| {
                                        sidebar::working_label(
                                            state,
                                            chrono::Utc::now().timestamp(),
                                        )
                                    })
                                    .unwrap_or_else(|| {
                                        SharedString::from(format_time_ago(
                                            session.updated_at.max(session.created_at),
                                        ))
                                    }),
                            ),
                    ),
            );

        if is_renaming {
            let on_commit = on_commit_rename;
            div()
                .w_full()
                .child(row)
                .on_mouse_down_out(move |_, window, cx| {
                    (on_commit)(window, cx);
                })
                .into_any_element()
        } else {
            session_context_menu(
                row,
                move |window, cx| (on_rename)(window, cx),
                move |window, cx| (on_delete)(window, cx),
            )
            .into_any_element()
        }
    }
}

#[derive(Clone, Debug)]
pub struct DraftSummary {
    pub session_id: Option<String>,
    pub title: String,
    pub preview: String,
    pub updated_at: i64,
}

#[derive(Clone)]
enum SidebarRow {
    DraftHeader,
    Draft(usize),
    /// Position into the sidebar's shared session list. Carrying the index
    /// (not the header) keeps row construction allocation-free; only rows
    /// actually rendered by the virtualized list clone their header.
    Session(usize),
    GroupHeader {
        group: SessionDateGroup,
        collapsed: bool,
    },
}

fn render_sidebar_session_item(
    session: SessionHeader,
    projects: &[ProjectInfo],
    selected_id: Option<&str>,
    running_started_at: Option<i64>,
    is_waiting: bool,
    has_draft: bool,
    on_select: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_rename: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_commit_rename: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_delete: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    renaming_session_id: Option<&str>,
    rename_input: Option<Entity<ComposerInput>>,
) -> gpui::AnyElement {
    let is_active = selected_id == Some(session.id.as_str());
    let loading_state = sidebar::session_loading_state(
        &session,
        running_started_at,
        is_waiting,
    );
    let project_name = projects
        .iter()
        .find(|project| {
            session.project_id.as_deref() == Some(project.id.as_str())
                || (!session.cwd.is_empty() && session.cwd == project.path)
        })
        .map(|project| project.name.clone());
    let session_id = session.id.clone();
    let is_renaming = renaming_session_id == Some(session_id.as_str());
    SidebarSessionItem::new(
        session,
        project_name,
        is_active,
        loading_state,
        has_draft,
        is_renaming,
        is_renaming.then_some(rename_input).flatten(),
        {
            let session_id = session_id.clone();
            let on_select = on_select.clone();
            move |window, cx| (on_select)(session_id.clone(), window, cx)
        },
        {
            let session_id = session_id.clone();
            let on_rename = on_rename.clone();
            move |window, cx| (on_rename)(session_id.clone(), window, cx)
        },
        {
            let on_commit_rename = on_commit_rename.clone();
            move |window, cx| (on_commit_rename)(window, cx)
        },
        {
            let on_cancel_rename = on_cancel_rename.clone();
            move |window, cx| (on_cancel_rename)(window, cx)
        },
        {
            let on_delete = on_delete.clone();
            move |window, cx| (on_delete)(session_id.clone(), window, cx)
        },
    )
    .into_any_element()
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
    /// Retained list state is owned by the desktop app so it survives root
    /// renders caused by scrolling, status ticks, and transcript updates.
    pub sidebar_list_state: ListState,
    on_select_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_search: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_group: Rc<dyn Fn(SessionDateGroup, &mut Window, &mut App) + 'static>,
    on_rename_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_commit_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_delete_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    renaming_session_id: Option<String>,
    rename_input: Option<Entity<ComposerInput>>,
    on_resize_start: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    on_open_settings: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_open_server_settings: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub environments: Vec<EnvironmentRow>,
    pub server_menu: ContextMenuHandle,
    on_switch_environment: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl SidebarView {
    pub fn new(
        visible: bool,
        width: f32,
        sessions: Rc<Vec<SessionHeader>>,
        projects: Rc<Vec<ProjectInfo>>,
        selected_session_id: Option<String>,
        collapsed_groups: Rc<HashSet<SessionDateGroup>>,
        running_sessions: HashMap<String, i64>,
        waiting_sessions: HashSet<String>,
        draft_summaries: Vec<DraftSummary>,
        sidebar_list_state: ListState,
        environments: Vec<EnvironmentRow>,
        server_menu: ContextMenuHandle,
        on_select_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_new_chat: impl Fn(&mut Window, &mut App) + 'static,
        on_search: impl Fn(&mut Window, &mut App) + 'static,
        on_add_project: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_group: impl Fn(SessionDateGroup, &mut Window, &mut App) + 'static,
        on_rename_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_commit_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_cancel_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_delete_session: impl Fn(String, &mut Window, &mut App) + 'static,
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
            running_sessions,
            waiting_sessions,
            draft_summaries,
            sidebar_list_state,
            on_select_session: Rc::new(on_select_session),
            on_new_chat: Rc::new(on_new_chat),
            on_search: Rc::new(on_search),
            on_add_project: Rc::new(on_add_project),
            on_toggle_group: Rc::new(on_toggle_group),
            on_rename_session: Rc::new(on_rename_session),
            on_commit_rename: Rc::new(on_commit_rename),
            on_cancel_rename: Rc::new(on_cancel_rename),
            on_delete_session: Rc::new(on_delete_session),
            renaming_session_id,
            rename_input,
            on_resize_start: Rc::new(on_resize_start),
            on_open_settings: Rc::new(on_open_settings),
            on_open_server_settings: Rc::new(on_open_server_settings),
            environments,
            server_menu,
            on_switch_environment: Rc::new(on_switch_environment),
        }
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
        let on_sel = self.on_select_session;
        let on_rename = self.on_rename_session;
        let on_commit_rename = self.on_commit_rename;
        let on_cancel_rename = self.on_cancel_rename;
        let on_del = self.on_delete_session;
        let renaming_session_id = self.renaming_session_id;
        let rename_input = self.rename_input;
        let on_resize_start = self.on_resize_start;
        let on_open_settings = self.on_open_settings;
        let on_open_server_settings = self.on_open_server_settings;
        let environments = self.environments;
        let server_menu = self.server_menu;
        let on_switch_env = self.on_switch_environment;
        let width = self.width;

        // Bucket sessions by calendar period (Today / Yesterday / This Week /
        // This Month / Older), each group preceded by its section header —
        // which doubles as a collapse toggle. The *first* group's header is
        // pinned above the virtualized list (so "Today" and the folder+ icon
        // never move when toggling); its sessions plus the remaining groups'
        // headers and sessions become lightweight row descriptors. Grouping
        // moves only positions; headers are cloned per rendered row below.
        let sessions = self.sessions;
        let mut grouped = group_indices_by_date(sessions.len(), |index| {
            let session = &sessions[index];
            session.updated_at.max(session.created_at)
        })
        .into_iter();
        let pinned = grouped.next();

        // The pinned header row above the list: always the first group's
        // header, sharing its line with the add-project button.
        let pinned_header = match &pinned {
            Some((group, _)) => group_header(
                theme,
                group.label(),
                collapsed_groups.contains(group),
                true,
                on_add.clone(),
                on_toggle_group.clone(),
                *group,
            )
            .into_any_element(),
            None => group_header(
                theme,
                "Today",
                false,
                true,
                on_add.clone(),
                on_toggle_group.clone(),
                SessionDateGroup::Today,
            )
            .into_any_element(),
        };

        // Scrollable rows: any active drafts are pinned right at the top under
        // a dedicated Drafts section, followed by date groups.
        let mut list_rows = Vec::new();
        if !draft_summaries.is_empty() {
            list_rows.push(SidebarRow::DraftHeader);
            for i in 0..draft_summaries.len() {
                list_rows.push(SidebarRow::Draft(i));
            }
        }
        if let Some((group, positions)) = pinned {
            if !collapsed_groups.contains(&group) {
                list_rows.extend(positions.into_iter().map(SidebarRow::Session));
            }
        }
        for (group, positions) in grouped {
            let collapsed = collapsed_groups.contains(&group);
            list_rows.push(SidebarRow::GroupHeader { group, collapsed });
            if !collapsed {
                list_rows.extend(positions.into_iter().map(SidebarRow::Session));
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
        let list_on_rename = on_rename;
        let list_on_commit_rename = on_commit_rename;
        let list_on_cancel_rename = on_cancel_rename;
        let list_on_del = on_del;
        let list_renaming_session_id = renaming_session_id;
        let list_rename_input = rename_input;
        let list_on_add = on_add.clone();
        let list_on_toggle_group = on_toggle_group.clone();
        let grouped_rows = list(sidebar_list_state, move |index, _window, _cx| {
            let Some(row) = list_rows.get(index).cloned() else {
                return div().into_any_element();
            };
            let row = match row {
                SidebarRow::DraftHeader => div()
                    .w_full()
                    .px(px(14.0))
                    .py(px(4.0))
                    .flex()
                    .items_center()
                    .justify_between()
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap_x(px(6.0))
                            .child(app_icon(IconName::Pen, 10.5, theme.accent))
                            .child(
                                div()
                                    .text_size(px(11.0))
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .text_color(theme.text_secondary)
                                    .child("Drafts"),
                            ),
                    )
                    .child(
                        div()
                            .px(px(5.0))
                            .py(px(0.5))
                            .rounded(px(3.0))
                            .bg(theme.accent.opacity(0.12))
                            .text_color(theme.accent)
                            .text_size(px(10.0))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .child(format!("{}", list_draft_summaries.len())),
                    )
                    .into_any_element(),
                SidebarRow::Draft(draft_index) => {
                    let Some(draft) = list_draft_summaries.get(draft_index) else {
                        return div().into_any_element();
                    };
                    let is_active = match &draft.session_id {
                        Some(sid) => list_selected_id.as_deref() == Some(sid),
                        None => list_selected_id.is_none(),
                    };
                    let sid = draft.session_id.clone();
                    let on_sel = list_on_sel.clone();
                    let on_new = list_on_new.clone();
                    div()
                        .id(ElementId::Name(format!("draft-row-{}", draft_index).into()))
                        .w_full()
                        .px(px(10.0))
                        .py(px(6.0))
                        .rounded(px(6.0))
                        .cursor_pointer()
                        .when(is_active, |s| s.bg(theme.sidebar_item_background))
                        .when(!is_active, |s| s.hover(|h| h.bg(theme.sidebar_item_background)))
                        .on_click(move |_, window, cx| {
                            if let Some(sid) = &sid {
                                (on_sel)(sid.clone(), window, cx);
                            } else {
                                (on_new)(window, cx);
                            }
                        })
                        .flex()
                        .flex_col()
                        .gap_y(px(2.0))
                        .child(
                            div()
                                .w_full()
                                .flex()
                                .items_center()
                                .justify_between()
                                .gap_x(px(6.0))
                                .child(
                                    div()
                                        .flex_1()
                                        .min_w_0()
                                        .truncate()
                                        .text_size(px(12.5))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child(draft.title.clone()),
                                )
                                .child(
                                    div()
                                        .flex_none()
                                        .px(px(4.0))
                                        .py(px(0.5))
                                        .rounded(px(3.0))
                                        .bg(theme.accent.opacity(0.15))
                                        .text_color(theme.accent)
                                        .text_size(px(10.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .child("Draft"),
                                ),
                        )
                        .child(
                            div()
                                .w_full()
                                .truncate()
                                .text_size(px(11.0))
                                .text_color(theme.text_tertiary)
                                .child(draft.preview.clone()),
                        )
                        .into_any_element()
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
                        &list_on_rename,
                        &list_on_commit_rename,
                        &list_on_cancel_rename,
                        &list_on_del,
                        list_renaming_session_id.as_deref(),
                        list_rename_input.clone(),
                    )
                }
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
            };
            div().w_full().pb(px(2.0)).child(row).into_any_element()
        })
        .size_full();

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
                    .child(
                        div()
                            .id("btn-search")
                            .tab_index(0)
                            .w_full()
                            .h(px(32.0))
                            .px(px(6.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .justify_between()
                            .cursor_default()
                            .hover(|s| s.bg(theme.sidebar_item_background))
                            .active(|s| s.bg(theme.overlay_strong))
                            .on_click(move |_, window, cx| {
                                (on_search)(window, cx);
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
                            .child(
                                div()
                                    .px(px(4.0))
                                    .py(px(1.0))
                                    .rounded(px(3.0))
                                    .bg(theme.raised)
                                    .text_size(px(9.5))
                                    .text_color(theme.text_ghost)
                                    .child("⌘K"),
                            ),
                    ),
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
                            .child(app_icon(IconName::Settings, 14.0, theme.text_tertiary))
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
                            .child(app_icon(IconName::Server, 14.0, theme.text_tertiary));

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

/// A calendar-period section header for the grouped session list. Clicking the
/// label collapses or expands the group (a chevron reveals on hover). The
/// first group's header also hosts the add-project button on its right edge,
/// so the label and the folder+ icon sit on the same line.
#[allow(clippy::too_many_arguments)]
fn group_header(
    theme: Theme,
    label: &'static str,
    collapsed: bool,
    with_add_button: bool,
    on_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(SessionDateGroup, &mut Window, &mut App) + 'static>,
    group: SessionDateGroup,
) -> impl IntoElement {
    let group_name = format!("sidebar-group-header-{}", group.index());
    let chevron = app_icon(IconName::ChevronDown, 11.0, theme.text_ghost)
        .when(collapsed, |icon| {
            icon.with_transformation(gpui::Transformation::rotate(gpui::percentage(0.75)))
        })
        .invisible()
        .group_hover(group_name.clone(), |icon| icon.visible());
    let toggle_on_click = on_toggle.clone();
    let toggle_on_key = on_toggle;

    div()
        .h(px(28.0))
        .flex()
        .items_center()
        .justify_between()
        .group(group_name)
        .child(
            div()
                .id(ElementId::Name(
                    format!("sidebar-group-toggle-{}", group.index()).into(),
                ))
                .tab_index(0)
                .h(px(22.0))
                .flex()
                .items_center()
                .gap(px(4.0))
                .cursor_default()
                .hover(|s| s.bg(theme.overlay))
                .text_size(px(14.5))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(theme.text)
                .child(label)
                .child(chevron)
                .on_click(move |_, window, cx| (toggle_on_click)(group, window, cx))
                .on_key_down(move |event: &KeyDownEvent, window, cx| {
                    if matches!(event.keystroke.key.as_str(), "enter" | "space") {
                        (toggle_on_key)(group, window, cx);
                        cx.stop_propagation();
                    }
                }),
        )
        .when(with_add_button, |header| {
            header.child(
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
                    .on_click(move |_, window, cx| {
                        (on_add)(window, cx);
                    })
                    .child(app_icon(IconName::FolderNew, 15.0, theme.text_ghost)),
            )
        })
}
