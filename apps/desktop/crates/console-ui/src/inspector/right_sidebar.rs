//! Conductor-style Right Sidebar Inspector Shell.

use std::collections::HashSet;
use std::rc::Rc;

use console_core::types::{FsTreeEntry, GitFileEntry, SessionFileChange};
use gpui::{
    App, InteractiveElement, IntoElement, MouseButton, MouseDownEvent, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::inspector::changes_list::ChangesListView;
use crate::inspector::file_tree::FileTreeView;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub enum InspectorTab {
    #[default]
    AllFiles,
    Changes,
}

#[derive(IntoElement)]
pub struct RightSidebar {
    width: f32,
    active_tab: InspectorTab,
    search_query: String,
    file_entries: Vec<FsTreeEntry>,
    working_changes: Vec<GitFileEntry>,
    session_changes: Vec<SessionFileChange>,
    expanded_folders: HashSet<String>,
    selected_path: Option<String>,
    on_select_tab: Rc<dyn Fn(InspectorTab, &mut Window, &mut App) + 'static>,
    on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
}

impl RightSidebar {
    pub fn new(
        width: f32,
        active_tab: InspectorTab,
        search_query: String,
        file_entries: Vec<FsTreeEntry>,
        working_changes: Vec<GitFileEntry>,
        session_changes: Vec<SessionFileChange>,
        expanded_folders: HashSet<String>,
        selected_path: Option<String>,
        on_select_tab: Rc<dyn Fn(InspectorTab, &mut Window, &mut App) + 'static>,
        on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            width,
            active_tab,
            search_query,
            file_entries,
            working_changes,
            session_changes,
            expanded_folders,
            selected_path,
            on_select_tab,
            on_toggle_folder,
            on_select_file,
            on_refresh,
            on_begin_resize,
        }
    }
}

impl RenderOnce for RightSidebar {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_tab = self.on_select_tab;
        let on_refresh = self.on_refresh;
        let on_resize = self.on_begin_resize;
        let changes_count = self.working_changes.len() + self.session_changes.len();

        div()
            .id("right-sidebar-shell")
            .w(px(self.width))
            .h_full()
            .flex_none()
            .flex()
            .flex_col()
            .bg(theme.sidebar)
            .border_l_1()
            .border_color(theme.sidebar_border)
            .relative()
            // Left drag handle for resizing
            .child(
                div()
                    .id("right-sidebar-resize-handle")
                    .absolute()
                    .left(px(-3.0))
                    .top_0()
                    .bottom_0()
                    .w(px(6.0))
                    .cursor_col_resize()
                    .hover(|s| s.bg(theme.accent.opacity(0.3)))
                    .on_mouse_down(
                        MouseButton::Left,
                        move |event: &MouseDownEvent, window, cx| {
                            (on_resize)(f32::from(event.position.x), window, cx);
                        },
                    ),
            )
            // Header: Segmented Tabs (All files | Changes) & Refresh Button
            .child(
                div()
                    .h(px(36.0))
                    .w_full()
                    .flex_none()
                    .flex()
                    .items_center()
                    .justify_between()
                    .px(px(8.0))
                    .border_b_1()
                    .border_color(theme.sidebar_border)
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap(px(4.0))
                            .p(px(2.0))
                            .rounded(px(6.0))
                            .bg(theme.surface)
                            // Tab 1: All files
                            .child({
                                let on_tab = on_tab.clone();
                                let is_active = self.active_tab == InspectorTab::AllFiles;
                                div()
                                    .id("tab-all-files")
                                    .px(px(8.0))
                                    .py(px(3.0))
                                    .rounded(px(4.0))
                                    .text_size(px(11.0))
                                    .font_weight(if is_active {
                                        gpui::FontWeight::SEMIBOLD
                                    } else {
                                        gpui::FontWeight::NORMAL
                                    })
                                    .text_color(if is_active {
                                        theme.text
                                    } else {
                                        theme.text_tertiary
                                    })
                                    .bg(if is_active {
                                        theme.overlay_strong
                                    } else {
                                        gpui::transparent_black()
                                    })
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .on_click(move |_, window, cx| {
                                        (on_tab)(InspectorTab::AllFiles, window, cx);
                                    })
                                    .child("All files")
                            })
                            // Tab 2: Changes (N)
                            .child({
                                let on_tab = on_tab.clone();
                                let is_active = self.active_tab == InspectorTab::Changes;
                                div()
                                    .id("tab-changes")
                                    .px(px(8.0))
                                    .py(px(3.0))
                                    .rounded(px(4.0))
                                    .flex()
                                    .items_center()
                                    .gap(px(4.0))
                                    .text_size(px(11.0))
                                    .font_weight(if is_active {
                                        gpui::FontWeight::SEMIBOLD
                                    } else {
                                        gpui::FontWeight::NORMAL
                                    })
                                    .text_color(if is_active {
                                        theme.text
                                    } else {
                                        theme.text_tertiary
                                    })
                                    .bg(if is_active {
                                        theme.overlay_strong
                                    } else {
                                        gpui::transparent_black()
                                    })
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .on_click(move |_, window, cx| {
                                        (on_tab)(InspectorTab::Changes, window, cx);
                                    })
                                    .child("Changes")
                                    .when(changes_count > 0, |el| {
                                        el.child(
                                            div()
                                                .px(px(4.0))
                                                .py(px(1.0))
                                                .rounded(px(4.0))
                                                .bg(theme.overlay)
                                                .text_size(px(10.0))
                                                .font_weight(gpui::FontWeight::BOLD)
                                                .text_color(theme.accent)
                                                .child(changes_count.to_string()),
                                        )
                                    })
                            }),
                    )
                    // Refresh icon button
                    .child(
                        div()
                            .id("inspector-refresh-btn")
                            .size(px(24.0))
                            .rounded(px(4.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .on_click(move |_, window, cx| {
                                (on_refresh)(window, cx);
                            })
                            .child(app_icon(IconName::RotateCw, 12.0, theme.text_tertiary)),
                    ),
            )
            // Content Body
            .child(match self.active_tab {
                InspectorTab::AllFiles => FileTreeView::new(
                    self.file_entries,
                    self.expanded_folders,
                    self.selected_path,
                    self.search_query,
                    self.on_toggle_folder,
                    self.on_select_file,
                )
                .into_any_element(),
                InspectorTab::Changes => ChangesListView::new(
                    self.working_changes,
                    self.session_changes,
                    self.selected_path,
                    self.on_select_file,
                )
                .into_any_element(),
            })
    }
}
