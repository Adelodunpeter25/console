use std::rc::Rc;

use console_core::LeafPaneNode;
use gpui::{
    App, AppContext, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div,
    prelude::FluentBuilder, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::{TABBAR_HEIGHT, Theme};

use super::{WorkspaceDrag, WorkspaceDragPreview};

/// The workspace's tab strip for one leaf pane. Dragging tabs between panes
/// and the per-type icons come from the desktop app's `WorkspaceTabItem`.
#[derive(IntoElement)]
pub struct WorkspaceTabBar {
    pub pane: LeafPaneNode,
    on_select: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_close: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    can_close_pane: bool,
    on_close_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl WorkspaceTabBar {
    pub fn new(
        pane: LeafPaneNode,
        on_select: impl Fn(String, String, &mut Window, &mut App) + 'static,
        on_close: impl Fn(String, String, &mut Window, &mut App) + 'static,
        can_close_pane: bool,
        on_close_pane: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            pane,
            on_select: Rc::new(on_select),
            on_close: Rc::new(on_close),
            can_close_pane,
            on_close_pane: Rc::new(on_close_pane),
        }
    }
}

fn tab_icon(config: &console_core::WorkspaceTabConfig) -> IconName {
    match config {
        console_core::WorkspaceTabConfig::Chat { .. } => IconName::Bot,
        console_core::WorkspaceTabConfig::Terminal { .. } => IconName::Terminal,
        console_core::WorkspaceTabConfig::File { .. } => IconName::File,
        console_core::WorkspaceTabConfig::Diff { .. } => IconName::GitBranch,
    }
}

impl RenderOnce for WorkspaceTabBar {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let pane_id = self.pane.id.clone();
        let active_id = self.pane.active_tab_id.clone();
        let on_sel = self.on_select;
        let on_cls = self.on_close;
        let can_close_pane = self.can_close_pane;
        let on_close_pane = self.on_close_pane;

        div()
            .id("workspace-tab-bar")
            .h(TABBAR_HEIGHT)
            .w_full()
            .bg(theme.canvas)
            .border_b_1()
            .border_color(theme.border)
            .flex()
            .items_center()
            .justify_between()
            .px(px(4.0))
            // Tabs Scroll List
            .child(
                div()
                    .id("workspace-tabs-scroll")
                    .flex_1()
                    .h_full()
                    .flex()
                    .items_center()
                    .gap_x(px(2.0))
                    .overflow_hidden()
                    .children(self.pane.tabs.into_iter().map(|tab| {
                        let tab_id = tab.id();
                        let is_active = active_id.as_deref() == Some(&tab_id.as_str());
                        let title = tab.title().to_string();
                        let tab_icon_name = tab_icon(&tab);
                        let drag = WorkspaceDrag::new(tab.clone(), Some(pane_id.clone()));

                        let id_for_select = tab_id.clone();
                        let id_for_close = tab_id.clone();
                        let s_on_sel = on_sel.clone();
                        let s_on_cls = on_cls.clone();
                        let group_name = format!("workspace-tab-{}", tab_id);

                        div()
                            .id(ElementId::Name(tab_id.into()))
                            .on_drag(drag, |drag, _, _, cx| {
                                cx.new(|_| WorkspaceDragPreview::new(drag.tab.title()))
                            })
                            .h(px(30.0))
                            .px(px(10.0))
                            .cursor_default()
                            .flex()
                            .items_center()
                            .gap_x(px(6.0))
                            .group(group_name.clone())
                            .when(is_active, |s| {
                                s.bg(theme.surface)
                                    .border_b_2()
                                    .border_color(theme.accent)
                                    .text_color(theme.text)
                            })
                            .when(!is_active, |s| {
                                s.text_color(theme.text_tertiary)
                                    .hover(|h| h.bg(theme.raised).text_color(theme.text))
                            })
                            .on_mouse_down(MouseButton::Left, {
                                let id = id_for_select;
                                let on_s = s_on_sel;
                                let pane_id = pane_id.clone();
                                move |_, window, cx| {
                                    (on_s)(pane_id.clone(), id.clone(), window, cx);
                                }
                            })
                            // Tab Icon — white for the active tab, muted for
                            // inactive ones; no brand color.
                            .child(app_icon(
                                tab_icon_name,
                                11.0,
                                if is_active {
                                    theme.text
                                } else {
                                    theme.text_tertiary
                                },
                            ))
                            // Tab Title
                            .child(
                                div()
                                    .max_w(px(140.0))
                                    .truncate()
                                    .text_size(px(12.0))
                                    .font_weight(if is_active {
                                        FontWeight::MEDIUM
                                    } else {
                                        FontWeight::NORMAL
                                    })
                                    .child(title),
                            )
                            // Close Tab Button: hidden until the tab is hovered.
                            .child(
                                div()
                                    .p(px(3.0))
                                    .rounded(px(4.0))
                                    .cursor_default()
                                    .invisible()
                                    .group_hover(group_name.clone(), |el| el.visible())
                                    .hover(|s| s.bg(theme.overlay))
                                    .on_mouse_down(MouseButton::Left, {
                                        let id = id_for_close;
                                        let on_c = s_on_cls;
                                        let pane_id = pane_id.clone();
                                        move |_, window, cx| {
                                            cx.stop_propagation();
                                            (on_c)(pane_id.clone(), id.clone(), window, cx);
                                        }
                                    })
                                    .child(app_icon(IconName::X, 12.0, theme.text_ghost)),
                            )
                    })),
            )
            .when(can_close_pane, |bar| {
                let pane_id = pane_id.clone();
                bar.child(
                    div()
                        .id(ElementId::Name(
                            format!("workspace-close-pane-{}", pane_id).into(),
                        ))
                        .p(px(4.0))
                        .rounded(px(4.0))
                        .cursor_default()
                        .hover(|style| style.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                            cx.stop_propagation();
                            (on_close_pane)(pane_id.clone(), window, cx);
                        })
                        .child(app_icon(IconName::X, 13.0, theme.text_ghost)),
                )
            })
        // New Chat remains available from the sidebar and workspace empty state.
    }
}
