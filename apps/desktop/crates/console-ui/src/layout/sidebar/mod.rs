pub mod draft_item;
pub mod group_header;
pub mod session_item;

pub use draft_item::{DraftSummary, render_sidebar_draft_item};
pub use group_header::{drafts_group_header, group_header};
pub use session_item::{
    CancelSessionRename, CommitSessionRename, SidebarSessionItem, init_session_rename_keybindings,
    render_sidebar_session_item,
};

use console_core::{ProjectInfo, SessionHeader};
use gpui::{
    App, Entity, FontWeight, InteractiveElement, IntoElement, ListState, MouseButton,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div, list,
    prelude::FluentBuilder, px,
};
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use crate::input::ComposerInput;
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, MenuItem, dropdown_menu};
use crate::primitives::{IconName, app_icon};
use crate::settings::EnvironmentRow;
use crate::theme::Theme;
use crate::utils::{SessionDateGroup, group_indices_by_date};

#[derive(Clone)]
enum SidebarRow {
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
    pub drafts_collapsed: bool,
    /// Retained list state is owned by the desktop app so it survives root
    /// renders caused by scrolling, status ticks, and transcript updates.
    pub sidebar_list_state: ListState,
    on_select_session: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_search: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_group: Rc<dyn Fn(SessionDateGroup, &mut Window, &mut App) + 'static>,
    on_toggle_drafts: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
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
    #[allow(clippy::too_many_arguments)]
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
        drafts_collapsed: bool,
        sidebar_list_state: ListState,
        environments: Vec<EnvironmentRow>,
        server_menu: ContextMenuHandle,
        on_select_session: impl Fn(String, &mut Window, &mut App) + 'static,
        on_new_chat: impl Fn(&mut Window, &mut App) + 'static,
        on_search: impl Fn(&mut Window, &mut App) + 'static,
        on_add_project: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_group: impl Fn(SessionDateGroup, &mut Window, &mut App) + 'static,
        on_toggle_drafts: impl Fn(&mut Window, &mut App) + 'static,
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
            drafts_collapsed,
            sidebar_list_state,
            on_select_session: Rc::new(on_select_session),
            on_new_chat: Rc::new(on_new_chat),
            on_search: Rc::new(on_search),
            on_add_project: Rc::new(on_add_project),
            on_toggle_group: Rc::new(on_toggle_group),
            on_toggle_drafts: Rc::new(on_toggle_drafts),
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

        // Filter out sessions that have drafts so they appear exclusively in the
        // Drafts section without duplicating in date groups below.
        let sessions = self.sessions;
        let grouped: Vec<(SessionDateGroup, Vec<usize>)> = group_indices_by_date(sessions.len(), |index| {
            let session = &sessions[index];
            if draft_sessions.contains(&session.id) {
                0
            } else {
                session.updated_at.max(session.created_at)
            }
        })
        .into_iter()
        .map(|(group, positions)| {
            let filtered: Vec<usize> = positions
                .into_iter()
                .filter(|&idx| !draft_sessions.contains(&sessions[idx].id))
                .collect();
            (group, filtered)
        })
        .filter(|(_, positions)| !positions.is_empty())
        .collect();

        let has_drafts = !draft_summaries.is_empty();

        // The pinned header row above the list: if drafts exist, Drafts is the
        // pinned top group header; otherwise the first date group is pinned.
        let pinned_header = if has_drafts {
            drafts_group_header(
                theme,
                self.drafts_collapsed,
                true,
                on_add.clone(),
                self.on_toggle_drafts.clone(),
            )
            .into_any_element()
        } else {
            let first_group = grouped.first().map(|(g, _)| *g).unwrap_or(SessionDateGroup::Today);
            group_header(
                theme,
                first_group.label(),
                collapsed_groups.contains(&first_group),
                true,
                on_add.clone(),
                on_toggle_group.clone(),
                first_group,
            )
            .into_any_element()
        };

        // Scrollable rows:
        let mut list_rows = Vec::new();
        if has_drafts {
            if !self.drafts_collapsed {
                for i in 0..draft_summaries.len() {
                    list_rows.push(SidebarRow::Draft(i));
                }
            }
            for (group, positions) in grouped {
                let collapsed = collapsed_groups.contains(&group);
                list_rows.push(SidebarRow::GroupHeader { group, collapsed });
                if !collapsed {
                    list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                }
            }
        } else {
            let mut grouped_iter = grouped.into_iter();
            if let Some((group, positions)) = grouped_iter.next() {
                if !collapsed_groups.contains(&group) {
                    list_rows.extend(positions.into_iter().map(SidebarRow::Session));
                }
            }
            for (group, positions) in grouped_iter {
                let collapsed = collapsed_groups.contains(&group);
                list_rows.push(SidebarRow::GroupHeader { group, collapsed });
                if !collapsed {
                    list_rows.extend(positions.into_iter().map(SidebarRow::Session));
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
                SidebarRow::Draft(draft_index) => {
                    let Some(draft) = list_draft_summaries.get(draft_index) else {
                        return div().into_any_element();
                    };
                    render_sidebar_draft_item(
                        draft,
                        list_selected_id.as_deref(),
                        &list_on_sel,
                        &list_on_new,
                        theme,
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
