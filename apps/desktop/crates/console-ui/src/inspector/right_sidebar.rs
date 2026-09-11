//! Conductor-style Right Sidebar Inspector Shell.

use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use console_core::types::{GitFileEntry, SessionFileChange, SubagentInfo};
use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, MouseButton, MouseDownEvent, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::inspector::bottom_split::RightSidebarBottomSplit;
use crate::inspector::changes_list::ChangesListView;
use crate::inspector::file_tree::{FileTreeNode, FileTreeView};
use crate::inspector::subagent_list::SubagentListView;
use crate::markdown::render::MarkdownView;
use crate::primitives::icons::{IconName, app_icon};
use crate::primitives::{ContextMenuHandle, MenuAlign, MenuItem, dropdown_menu};
use crate::theme::Theme;

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Default, serde::Serialize, serde::Deserialize)]
pub enum PrimaryTab {
    #[default]
    AllFiles,
    Changes,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, serde::Serialize, serde::Deserialize)]
pub enum AuxiliaryTab {
    Browser,
    Subagents,
    Devices,
}

impl AuxiliaryTab {
    pub const ALL: [Self; 3] = [Self::Browser, Self::Subagents, Self::Devices];

    pub fn label(self) -> &'static str {
        match self {
            Self::Browser => "Browser",
            Self::Subagents => "Subagents",
            Self::Devices => "Devices",
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, serde::Serialize, serde::Deserialize)]
pub enum InspectorTab {
    Primary(PrimaryTab),
    Auxiliary(AuxiliaryTab),
}

impl Default for InspectorTab {
    fn default() -> Self {
        Self::Primary(PrimaryTab::AllFiles)
    }
}

#[derive(IntoElement)]
pub struct RightSidebar {
    width: f32,
    active_tab: InspectorTab,
    search_query: String,
    tree: Rc<Vec<FileTreeNode>>,
    working_changes: Rc<Vec<GitFileEntry>>,
    session_changes: Rc<Vec<SessionFileChange>>,
    subagents: Rc<Vec<SubagentInfo>>,
    expanded_folders: HashSet<String>,
    expanded_subagents: HashSet<String>,
    selected_path: Option<String>,
    bottom_split: Option<RightSidebarBottomSplit>,
    subagent_markdown_views: Option<Rc<RefCell<HashMap<String, Rc<RefCell<MarkdownView>>>>>>,
    browser_view: Option<AnyElement>,
    open_auxiliary_tabs: Vec<AuxiliaryTab>,
    add_tab_menu: ContextMenuHandle,
    on_select_tab: Rc<dyn Fn(InspectorTab, &mut Window, &mut App) + 'static>,
    on_open_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App) + 'static>,
    on_close_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App) + 'static>,
    on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle_subagent: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_copy_summary: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
}

impl RightSidebar {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        width: f32,
        active_tab: InspectorTab,
        search_query: String,
        tree: Rc<Vec<FileTreeNode>>,
        working_changes: Rc<Vec<GitFileEntry>>,
        session_changes: Rc<Vec<SessionFileChange>>,
        subagents: Rc<Vec<SubagentInfo>>,
        expanded_folders: HashSet<String>,
        expanded_subagents: HashSet<String>,
        selected_path: Option<String>,
        open_auxiliary_tabs: Vec<AuxiliaryTab>,
        add_tab_menu: ContextMenuHandle,
        on_select_tab: Rc<dyn Fn(InspectorTab, &mut Window, &mut App) + 'static>,
        on_open_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App) + 'static>,
        on_close_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App) + 'static>,
        on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_toggle_subagent: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_copy_summary: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            width,
            active_tab,
            search_query,
            tree,
            working_changes,
            session_changes,
            subagents,
            expanded_folders,
            expanded_subagents,
            selected_path,
            open_auxiliary_tabs,
            add_tab_menu,
            bottom_split: None,
            subagent_markdown_views: None,
            browser_view: None,
            on_select_tab,
            on_open_auxiliary_tab,
            on_close_auxiliary_tab,
            on_toggle_folder,
            on_select_file,
            on_toggle_subagent,
            on_copy_summary,
            on_refresh,
            on_begin_resize,
        }
    }

    pub fn with_bottom_split(mut self, bottom_split: Option<RightSidebarBottomSplit>) -> Self {
        self.bottom_split = bottom_split;
        self
    }

    pub fn subagent_markdown_views(
        mut self,
        views: Rc<RefCell<HashMap<String, Rc<RefCell<MarkdownView>>>>>,
    ) -> Self {
        self.subagent_markdown_views = Some(views);
        self
    }

    pub fn with_browser_view(mut self, browser_view: Option<AnyElement>) -> Self {
        self.browser_view = browser_view;
        self
    }
}

impl RenderOnce for RightSidebar {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_tab = self.on_select_tab;
        let on_open = self.on_open_auxiliary_tab;
        let on_close = self.on_close_auxiliary_tab;
        let add_menu = self.add_tab_menu;
        let open_auxiliary = self.open_auxiliary_tabs.clone();
        let on_refresh = self.on_refresh;
        let on_resize = self.on_begin_resize;

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
                    .on_mouse_down(
                        MouseButton::Left,
                        move |event: &MouseDownEvent, window, cx| {
                            cx.stop_propagation();
                            (on_resize)(f32::from(event.position.x), window, cx);
                        },
                    ),
            )
            // Dynamic tab strip: fixed primary tabs plus mounted auxiliary tabs.
            .child({
                let on_tab = on_tab.clone();
                let on_open = on_open.clone();
                let on_close = on_close.clone();
                let add_menu = add_menu.clone();
                let open_auxiliary = open_auxiliary.clone();
                let tab_chip = move |id: &'static str, label: &'static str, tab: InspectorTab, closable: Option<AuxiliaryTab>| {
                    let on_tab = on_tab.clone();
                    let on_close = on_close.clone();
                    let active = self.active_tab == tab;
                    div()
                        .id(id)
                        .px(px(7.0))
                        .py(px(3.0))
                        .rounded(px(4.0))
                        .flex()
                        .items_center()
                        .gap(px(3.0))
                        .text_size(px(11.0))
                        .font_weight(if active { gpui::FontWeight::SEMIBOLD } else { gpui::FontWeight::NORMAL })
                        .text_color(if active { theme.text } else { theme.text_tertiary })
                        .bg(if active { theme.overlay_strong } else { gpui::transparent_black() })
                        .cursor_pointer()
                        .hover(|s| s.bg(theme.overlay))
                        .on_click(move |_, window, cx| (on_tab)(tab, window, cx))
                        .child(label)
                        .when_some(closable, |el, auxiliary| {
                            let on_close = on_close.clone();
                            el.child(
                                div()
                                    .id(SharedString::from(id.to_owned() + "-close"))
                                    .px(px(2.0))
                                    .text_color(theme.text_tertiary)
                                    .hover(|s| s.text_color(theme.text))
                                    .on_mouse_down(MouseButton::Left, move |_event: &MouseDownEvent, window, cx| {
                                        (on_close)(auxiliary, window, cx);
                                    })
                                    .child("×"),
                            )
                        })
                        .into_any_element()
                };
                let mut tabs = div()
                    .flex()
                    .items_center()
                    .gap(px(4.0))
                    .p(px(2.0))
                    .rounded(px(6.0))
                    .bg(theme.surface)
                    .child(tab_chip("tab-all-files", "All files", InspectorTab::Primary(PrimaryTab::AllFiles), None))
                    .child(tab_chip("tab-changes", "Changes", InspectorTab::Primary(PrimaryTab::Changes), None));
                for auxiliary in &open_auxiliary {
                    let (id, tab) = match auxiliary {
                        AuxiliaryTab::Browser => ("tab-browser", InspectorTab::Auxiliary(AuxiliaryTab::Browser)),
                        AuxiliaryTab::Subagents => ("tab-subagents", InspectorTab::Auxiliary(AuxiliaryTab::Subagents)),
                        AuxiliaryTab::Devices => ("tab-devices", InspectorTab::Auxiliary(AuxiliaryTab::Devices)),
                    };
                    tabs = tabs.child(tab_chip(id, auxiliary.label(), tab, Some(*auxiliary)));
                }
                tabs = tabs.child(dropdown_menu(
                    div()
                        .id("inspector-add-tab")
                        .px(px(7.0))
                        .py(px(3.0))
                        .rounded(px(4.0))
                        .cursor_pointer()
                        .hover(|s| s.bg(theme.overlay))
                        .text_color(theme.text_tertiary)
                        .child("+"),
                    "inspector-add-tab-menu",
                    &add_menu,
                    MenuAlign::BelowLeft,
                    move |_cx| {
                        AuxiliaryTab::ALL
                            .into_iter()
                            .filter(|tab| !open_auxiliary.contains(tab))
                            .map(|tab| {
                                let on_open = on_open.clone();
                                MenuItem::new(tab.label(), move |window, cx| (on_open)(tab, window, cx))
                            })
                            .collect()
                    },
                ));
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
                    .child(tabs)
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
                            .on_click(move |_, window, cx| (on_refresh)(window, cx))
                            .child(app_icon(IconName::RotateCw, 12.0, theme.text_tertiary)),
                    )
            })
            // Content Body (Top Section)
            .child(
                div()
                    .flex_1()
                    .w_full()
                    .min_h_0()
                    .overflow_hidden()
                    .child(match self.active_tab {
                        InspectorTab::Primary(PrimaryTab::AllFiles) => FileTreeView::new(
                            self.tree,
                            self.expanded_folders,
                            self.selected_path,
                            self.search_query,
                            self.on_toggle_folder,
                            self.on_select_file,
                        )
                        .into_any_element(),
                        InspectorTab::Primary(PrimaryTab::Changes) => ChangesListView::new(
                            self.working_changes,
                            self.session_changes,
                            self.selected_path,
                            self.on_select_file,
                        )
                        .into_any_element(),
                        InspectorTab::Auxiliary(AuxiliaryTab::Browser) => {
                            if let Some(browser) = self.browser_view {
                                browser
                            } else {
                                div()
                                    .size_full()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .text_color(theme.text_tertiary)
                                    .child("Browser unavailable")
                                    .into_any_element()
                            }
                        }
                        InspectorTab::Auxiliary(AuxiliaryTab::Subagents) => {
                            let mut list = SubagentListView::new(
                                self.subagents,
                                self.expanded_subagents,
                                self.on_toggle_subagent,
                                self.on_copy_summary,
                            );
                            if let Some(views) = self.subagent_markdown_views {
                                list = list.markdown_views(views);
                            }
                            list.into_any_element()
                        }
                        InspectorTab::Auxiliary(AuxiliaryTab::Devices) => div()
                            .size_full()
                            .flex()
                            .items_center()
                            .justify_center()
                            .text_color(theme.text_tertiary)
                            .child("Device simulator unavailable")
                            .into_any_element(),
                    }),
            )
            .when_some(self.bottom_split, |el, bottom| el.child(bottom))
    }
}
