use console_core::{ProjectInfo, SessionHeader};
use gpui::{
    App, AppContext, ElementId, Entity, FontWeight, InteractiveElement, IntoElement, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, actions, div,
    prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::input::ComposerInput;
use crate::layout::sidebar_loading::{self as sidebar, SidebarLoadingState};
use crate::primitives::{IconName, app_icon, session_context_menu};
use crate::theme::Theme;
use crate::utils::format_time_ago;
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

        let folder_name = self
            .project_name
            .clone()
            .unwrap_or_else(|| crate::utils::format_folder_display_name(&session.cwd));

        let display_title = session.display_title().to_string();
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

pub fn render_sidebar_session_item(
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
        .find(|project| project.matches_session(&session))
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
