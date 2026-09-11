use std::cell::RefCell;
use std::rc::Rc;

use console_core::LeafPaneNode;
use gpui::{
    App, AppContext, Bounds, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton,
    ParentElement, Pixels, RenderOnce, ScrollHandle, StatefulInteractiveElement, Styled, Window,
    div, point, prelude::FluentBuilder, px,
};

use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::primitives::{IconName, app_icon};
use crate::theme::{TABBAR_HEIGHT, Theme};

use super::{WorkspaceDrag, WorkspaceDragPreview};

/// Retained follow state for a pane's tab strip scroll handle.
/// Automatically scrolls the strip the minimum distance to bring the active
/// tab into view when active tab changes or viewport resizes, without fighting
/// manual wheel scrolling.
#[derive(Clone, Debug)]
pub struct TabStripFollow {
    pub scroll_handle: ScrollHandle,
    last_active_tab: Rc<RefCell<Option<String>>>,
    last_viewport: Rc<RefCell<Option<Bounds<Pixels>>>>,
}

impl Default for TabStripFollow {
    fn default() -> Self {
        Self::new()
    }
}

impl TabStripFollow {
    pub fn new() -> Self {
        Self {
            scroll_handle: ScrollHandle::new(),
            last_active_tab: Rc::new(RefCell::new(None)),
            last_viewport: Rc::new(RefCell::new(None)),
        }
    }

    /// Measures the active tab's layout bounds and adjusts the scroll offset
    /// if the tab is outside the visible viewport.
    pub fn follow_if_needed(
        &self,
        active_tab_id: &str,
        tab_bounds: Bounds<Pixels>,
        window: &mut Window,
    ) {
        let viewport = self.scroll_handle.bounds();
        if viewport.size.width <= px(0.0) {
            return;
        }

        let mut last_tab = self.last_active_tab.borrow_mut();
        let mut last_vp = self.last_viewport.borrow_mut();

        let tab_changed = last_tab.as_deref() != Some(active_tab_id);
        let viewport_changed = match *last_vp {
            Some(prev) => {
                (prev.origin.x - viewport.origin.x).abs() > px(0.5)
                    || (prev.size.width - viewport.size.width).abs() > px(0.5)
            }
            None => true,
        };

        if !tab_changed && !viewport_changed {
            return;
        }

        *last_tab = Some(active_tab_id.to_string());
        *last_vp = Some(viewport);

        // Visual cushion so tab borders/corners aren't hard-clipped against strip edge.
        let padding = px(4.0);
        let offset = self.scroll_handle.offset();
        let mut x = offset.x;

        // If tab right is beyond viewport right, scroll left (offset.x decreases / becomes more negative).
        if tab_bounds.right() + padding > viewport.right() {
            x -= (tab_bounds.right() + padding) - viewport.right();
        }
        // If tab left is before viewport left, scroll right (offset.x increases / becomes less negative).
        // Evaluated second so that if tab is wider than viewport, left edge is prioritized.
        if tab_bounds.left() - padding < viewport.left() {
            x += viewport.left() - (tab_bounds.left() - padding);
        }

        let max_x = self.scroll_handle.max_offset().x;
        let x = x.clamp(-max_x, px(0.0));

        if (x - offset.x).abs() > px(0.5) {
            self.scroll_handle.set_offset(point(x, offset.y));
            window.request_animation_frame();
        }
    }
}

/// The workspace's tab strip for one leaf pane. Dragging tabs between panes
/// and the per-type icons come from the desktop app's `WorkspaceTabItem`.
#[derive(IntoElement)]
pub struct WorkspaceTabBar {
    pub pane: LeafPaneNode,
    pub is_focused: bool,
    on_select: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_close: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    can_close_pane: bool,
    on_close_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_tab: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    follow: Option<TabStripFollow>,
}

impl WorkspaceTabBar {
    pub fn new(
        pane: LeafPaneNode,
        is_focused: bool,
        on_select: impl Fn(String, String, &mut Window, &mut App) + 'static,
        on_close: impl Fn(String, String, &mut Window, &mut App) + 'static,
        can_close_pane: bool,
        on_close_pane: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            pane,
            is_focused,
            on_select: Rc::new(on_select),
            on_close: Rc::new(on_close),
            can_close_pane,
            on_close_pane: Rc::new(on_close_pane),
            on_new_tab: None,
            follow: None,
        }
    }

    pub fn with_new_tab(
        mut self,
        on_new_tab: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_new_tab = Some(Rc::new(on_new_tab));
        self
    }

    pub fn with_scroll_follow(mut self, follow: Option<TabStripFollow>) -> Self {
        self.follow = follow;
        self
    }
}

impl RenderOnce for WorkspaceTabBar {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let pane_id = self.pane.id.clone();
        let active_id = self.pane.active_tab_id.clone();
        let pane_focused = self.is_focused;
        let on_sel = self.on_select;
        let on_cls = self.on_close;
        let can_close_pane = self.can_close_pane;
        let on_close_pane = self.on_close_pane;
        let on_new_tab = self.on_new_tab;
        let follow = self.follow;

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
                    .overflow_x_scroll()
                    .when_some(follow.as_ref(), |el, fol| {
                        el.track_scroll(&fol.scroll_handle)
                    })
                    .children(self.pane.tabs.into_iter().map(|tab| {
                        let tab_id = tab.id();
                        let is_active = active_id.as_deref() == Some(&tab_id.as_str());
                        let title = tab.title().to_string();
                        let drag = WorkspaceDrag::new(tab.clone(), Some(pane_id.clone()));

                        let id_for_select = tab_id.clone();
                        let id_for_close = tab_id.clone();
                        let s_on_sel = on_sel.clone();
                        let s_on_cls = on_cls.clone();
                        let group_name = format!("workspace-tab-{}", tab_id);
                        let tab_follow = follow.clone();

                        div()
                            .id(ElementId::Name(tab_id.clone().into()))
                            .relative()
                            .flex_shrink(0.0)
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
                                let s = s.bg(theme.surface)
                                    .text_color(theme.text)
                                    // Only the focused pane shows the orange
                                    // underline; other panes show none.
                                    .when(pane_focused, |s| {
                                        s.border_b_2().border_color(theme.accent)
                                    });

                                if let Some(follow) = tab_follow {
                                    let tab_id = tab_id.clone();
                                    s.child(
                                        gpui::canvas(
                                            |_, _, _| (),
                                            move |bounds, _, window, _cx| {
                                                follow.follow_if_needed(&tab_id, bounds, window);
                                            },
                                        )
                                        .absolute()
                                        .inset_0(),
                                    )
                                } else {
                                    s
                                }
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
                            // Tab Icon
                            .child(match &tab {
                                console_core::WorkspaceTabConfig::Chat { .. } => app_icon(
                                    IconName::ChatRoundLine,
                                    11.0,
                                    if is_active {
                                        theme.text
                                    } else {
                                        theme.text_tertiary
                                    },
                                )
                                .into_any_element(),
                                console_core::WorkspaceTabConfig::Terminal { .. } => app_icon(
                                    IconName::Terminal,
                                    11.0,
                                    if is_active {
                                        theme.text
                                    } else {
                                        theme.text_tertiary
                                    },
                                )
                                .into_any_element(),
                                console_core::WorkspaceTabConfig::File { path, .. }
                                | console_core::WorkspaceTabConfig::Diff { path, .. } => {
                                    let name = std::path::Path::new(path)
                                        .file_name()
                                        .and_then(|n| n.to_str())
                                        .unwrap_or(path);
                                    file_icon(file_icon_for_name(name), 13.0).into_any_element()
                                }
                            })
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
                    }))
                    .when_some(on_new_tab, |scroll, on_new| {
                        let pane_id = pane_id.clone();
                        scroll.child(
                            div()
                                .id(ElementId::Name(
                                    format!("workspace-new-tab-{}", pane_id).into(),
                                ))
                                .flex_shrink(0.0)
                                .size(px(22.0))
                                .rounded(px(4.0))
                                .cursor_default()
                                .flex()
                                .items_center()
                                .justify_center()
                                .hover(|s| s.bg(theme.overlay))
                                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                    cx.stop_propagation();
                                    (on_new)(pane_id.clone(), window, cx);
                                })
                                .child(app_icon(IconName::Plus, 12.0, theme.text_ghost)),
                        )
                    }),
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
