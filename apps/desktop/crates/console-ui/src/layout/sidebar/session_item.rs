use console_core::{ProjectInfo, ProjectInfoExt, SessionHeader, SessionHeaderExt};
use gpui::{
    App, AppContext, ElementId, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, actions, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use super::item_row::{draft_badge, row_container, status_slot, title_label};
use super::session_card::SessionHoverCard;
use crate::input::ComposerInput;
use crate::layout::sidebar_loading::{self as sidebar, SidebarLoadingState};
use crate::primitives::motion;
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon, session_context_menu};
use crate::theme::Theme;
use crate::workspace::{WorkspaceDrag, WorkspaceDragPreview};

actions!(
    console_sidebar_rename,
    [CommitSessionRename, CancelSessionRename]
);

pub const SESSION_RENAME_PARENT_CONTEXT: &str = "ConsoleSessionRename";
pub const SESSION_RENAME_FIELD_CONTEXT: &str = "ConsoleSessionRename > ComposerInput";

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

/// The settle shortcut as it reads on this platform.
pub const SETTLE_SHORTCUT_LABEL: &str = if cfg!(target_os = "macos") {
    "⇧⌘S"
} else {
    "Ctrl+Shift+S"
};

/// Tooltip for the settle toggle: the action it performs, plus the shortcut
/// that does the same thing from the keyboard.
pub fn settle_tooltip_label(settled: bool) -> String {
    let action = if settled {
        "Move back to active"
    } else {
        "Settle chat"
    };
    format!("{action} ({SETTLE_SHORTCUT_LABEL})")
}

#[derive(IntoElement)]
pub struct SidebarSessionItem {
    session: SessionHeader,
    is_active: bool,
    loading_state: Option<SidebarLoadingState>,
    has_draft: bool,
    is_renaming: bool,
    rename_input: Option<Entity<ComposerInput>>,
    project_name: String,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_open_in_new_window: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_commit_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_pin: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_settle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_unsettle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_delete: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl SidebarSessionItem {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        session: SessionHeader,
        is_active: bool,
        loading_state: Option<SidebarLoadingState>,
        has_draft: bool,
        is_renaming: bool,
        rename_input: Option<Entity<ComposerInput>>,
        project_name: String,
        on_click: impl Fn(&mut Window, &mut App) + 'static,
        on_open_in_new_window: impl Fn(&mut Window, &mut App) + 'static,
        on_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_commit_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_cancel_rename: impl Fn(&mut Window, &mut App) + 'static,
        on_toggle_pin: impl Fn(&mut Window, &mut App) + 'static,
        on_settle: impl Fn(&mut Window, &mut App) + 'static,
        on_unsettle: impl Fn(&mut Window, &mut App) + 'static,
        on_delete: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            session,
            is_active,
            loading_state,
            has_draft,
            is_renaming,
            rename_input,
            project_name,
            on_click: Rc::new(on_click),
            on_open_in_new_window: Rc::new(on_open_in_new_window),
            on_rename: Rc::new(on_rename),
            on_commit_rename: Rc::new(on_commit_rename),
            on_cancel_rename: Rc::new(on_cancel_rename),
            on_toggle_pin: Rc::new(on_toggle_pin),
            on_settle: Rc::new(on_settle),
            on_unsettle: Rc::new(on_unsettle),
            on_delete: Rc::new(on_delete),
        }
    }
}

impl RenderOnce for SidebarSessionItem {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let session = &self.session;
        let is_active = self.is_active;
        let on_click = self.on_click;
        let on_open_in_new_window = self.on_open_in_new_window;
        let on_rename = self.on_rename;
        let on_commit_rename = self.on_commit_rename;
        let on_cancel_rename = self.on_cancel_rename;
        let on_toggle_pin = self.on_toggle_pin;
        let on_delete = self.on_delete;
        let pinned = session.pinned_at.is_some();
        let settled = session.settled_at.is_some();

        let loading_state = self.loading_state;
        let is_renaming = self.is_renaming;
        let rename_input = self.rename_input;

        let title_pending = sidebar::title_is_pending(session, loading_state);
        let display_title = if title_pending {
            "Generating title…".to_string()
        } else {
            session.display_title().to_string()
        };
        let drag = WorkspaceDrag::new(
            console_core::WorkspaceTabConfig::Chat {
                session_id: session.id.clone(),
                title: display_title.clone(),
                provider: Some(session.provider.clone()),
                project_id: session.project_id.clone(),
                last_active_at_ms: None,
            },
            None,
        );

        let group_name = format!("sidebar-session-{}", session.id);
        let is_running = matches!(loading_state, Some(SidebarLoadingState::Working { .. }))
            || session.status == "working"
            || session.status == "running";
        let needs_attention = matches!(loading_state, Some(SidebarLoadingState::Waiting))
            || session.status == "needs_attention"
            || session.status == "waiting";
        let is_done = session.status == "done" || session.status == "completed";

        // Status colour is information, not decoration: a chat that needs
        // attention stays orange whether or not it is the selected one, so
        // the state is readable at a glance in a long list.
        let status_id = || ElementId::Name(format!("status-{}", session.id).into());
        let status_indicator = if is_running {
            status_slot()
                .id(status_id())
                .tooltip(Tooltip::text("Running"))
                .child(motion::spin_slow(app_icon(
                    IconName::LoaderCircle,
                    15.0,
                    theme.accent,
                )))
                .into_any_element()
        } else if needs_attention {
            status_slot()
                .id(status_id())
                .tooltip(Tooltip::text("Needs attention"))
                .child(app_icon(IconName::Alert, 15.0, theme.warning))
                .into_any_element()
        } else if is_done {
            status_slot()
                .id(status_id())
                .tooltip(Tooltip::text("Done"))
                .child(app_icon(IconName::Done, 15.0, theme.success))
                .into_any_element()
        } else {
            status_slot().into_any_element()
        };

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
            title_label(is_active, theme)
                .when(title_pending, |s| {
                    s.italic().text_color(theme.text_tertiary)
                })
                .child(display_title.clone())
                .into_any_element()
        };

        let row = row_container(ElementId::Name(session.id.clone().into()), is_active, theme)
            .on_drag(drag, |drag, _, _, cx| {
                cx.new(|_| WorkspaceDragPreview::new(drag.tab.title()))
            })
            .group(group_name)
            // Settled chats read as archived: slightly dimmed everywhere.
            .when(settled, |s| s.opacity(0.7))
            .when(!is_renaming, |element| {
                let hover_title = display_title.clone();
                let hover_created = session.created_at;
                let hover_updated = session.updated_at;
                let hover_project = self.project_name.clone();

                element
                    .tooltip(move |window, cx| {
                        SessionHoverCard::new(
                            hover_title.clone(),
                            hover_created,
                            hover_updated,
                            hover_project.clone(),
                        )
                        .build(window, cx)
                    })
                    .on_click(move |_, w, cx| {
                        (on_click)(w, cx);
                    })
            })
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .flex()
                    .items_center()
                    .gap_x(px(6.0))
                    .child(status_indicator)
                    .child(title)
                    .when(self.has_draft, |el| el.child(draft_badge(theme))),
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
            let on_toggle_pin = on_toggle_pin;
            let menu_id = format!("session-menu-{}", session.id);
            session_context_menu(
                row,
                menu_id,
                window,
                cx,
                move |window, cx| (on_open_in_new_window)(window, cx),
                move |window, cx| (on_rename)(window, cx),
                move |window, cx| (on_toggle_pin)(window, cx),
                move |window, cx| (self.on_settle)(window, cx),
                move |window, cx| (self.on_unsettle)(window, cx),
                move |window, cx| (on_delete)(window, cx),
                pinned,
                settled,
            )
        }
    }
}

#[allow(clippy::too_many_arguments)]
pub fn render_sidebar_session_item(
    session: SessionHeader,
    projects: &[ProjectInfo],
    selected_id: Option<&str>,
    running_started_at: Option<i64>,
    is_waiting: bool,
    has_draft: bool,
    on_select: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_open_in_new_window: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_rename: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_commit_rename: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel_rename: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_pin: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_settle: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_unsettle: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_delete: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    renaming_session_id: Option<&str>,
    rename_input: Option<Entity<ComposerInput>>,
) -> gpui::AnyElement {
    let is_active = selected_id == Some(session.id.as_str());
    let loading_state = sidebar::session_loading_state(&session, running_started_at, is_waiting);
    let project_name = projects
        .iter()
        .find(|project| project.matches_session(&session))
        .map(|project| project.name.clone())
        .unwrap_or_else(|| {
            std::path::Path::new(&session.cwd)
                .file_name()
                .and_then(|n| n.to_str())
                .filter(|s| !s.is_empty())
                .unwrap_or("General")
                .to_string()
        });
    let session_id = session.id.clone();
    let is_renaming = renaming_session_id == Some(session_id.as_str());
    SidebarSessionItem::new(
        session,
        is_active,
        loading_state,
        has_draft,
        is_renaming,
        is_renaming.then_some(rename_input).flatten(),
        project_name,
        {
            let session_id = session_id.clone();
            let on_select = on_select.clone();
            move |window, cx| (on_select)(session_id.clone(), window, cx)
        },
        {
            let session_id = session_id.clone();
            let on_open = on_open_in_new_window.clone();
            move |window, cx| (on_open)(session_id.clone(), window, cx)
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
            let session_id = session_id.clone();
            let on_toggle_pin = on_toggle_pin.clone();
            move |window, cx| (on_toggle_pin)(session_id.clone(), window, cx)
        },
        {
            let session_id = session_id.clone();
            let on_settle = on_settle.clone();
            move |window, cx| (on_settle)(session_id.clone(), window, cx)
        },
        {
            let session_id = session_id.clone();
            let on_unsettle = on_unsettle.clone();
            move |window, cx| (on_unsettle)(session_id.clone(), window, cx)
        },
        {
            let on_delete = on_delete.clone();
            move |window, cx| (on_delete)(session_id.clone(), window, cx)
        },
    )
    .into_any_element()
}
