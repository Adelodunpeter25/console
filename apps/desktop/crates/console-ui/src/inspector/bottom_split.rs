//! Bottom Split Execution & Terminal Panel for the Right Sidebar Inspector.

use gpui::{
    App, InteractiveElement, IntoElement, MouseButton, MouseDownEvent, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct TerminalTabInfo {
    pub id: usize,
    pub title: String,
    pub script_id: Option<String>,
}

/// Play/stop control for the header, shown in place of the refresh button
/// while a script log tab (e.g. "Dev Server") is the active tab.
#[derive(Clone)]
pub struct ScriptTabAction {
    /// True while the script is running or starting: renders a stop icon
    /// and the click stops it. False renders a play icon that starts it.
    pub running: bool,
    pub on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

#[derive(IntoElement)]
pub struct RightSidebarBottomSplit {
    height: f32,
    collapsed: bool,
    tabs: Vec<TerminalTabInfo>,
    active_tab_index: usize,
    terminal_element: Option<gpui::AnyElement>,
    show_run_tab: bool,
    run_tab_active: bool,
    run_element: Option<gpui::AnyElement>,
    on_select_tab: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_close_tab: Option<Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>>,
    on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    on_new_terminal: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    on_refresh_run: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    script_action: Option<ScriptTabAction>,
    on_toggle_collapsed: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl RightSidebarBottomSplit {
    pub fn new(
        height: f32,
        collapsed: bool,
        tabs: Vec<TerminalTabInfo>,
        active_tab_index: usize,
        terminal_element: Option<gpui::AnyElement>,
        on_select_tab: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
        on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            height,
            collapsed,
            tabs,
            active_tab_index,
            terminal_element,
            show_run_tab: false,
            run_tab_active: false,
            run_element: None,
            on_select_tab,
            on_close_tab: None,
            on_begin_resize,
            on_new_terminal: None,
            on_refresh_run: None,
            script_action: None,
            on_toggle_collapsed: None,
        }
    }

    pub fn with_close_tab(
        mut self,
        callback: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    ) -> Self {
        self.on_close_tab = Some(callback);
        self
    }

    /// Prepend the pinned Run tab at index 0 (terminal tabs shift by one).
    /// The Run tab is never closable.
    pub fn with_run_tab(mut self, active: bool, element: impl IntoElement) -> Self {
        self.show_run_tab = true;
        self.run_tab_active = active;
        self.run_element = Some(element.into_any_element());
        self
    }

    pub fn with_new_terminal(
        mut self,
        callback: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        self.on_new_terminal = Some(callback);
        self
    }

    /// Refresh action for the Run tab header (re-reads console.toml).
    /// Only rendered while the Run tab is active.
    pub fn with_refresh_run(
        mut self,
        callback: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        self.on_refresh_run = Some(callback);
        self
    }

    /// Play/stop control for the active script log tab's header. Only
    /// rendered while a script log tab (not the Scripts tab) is active.
    pub fn with_script_action(mut self, action: ScriptTabAction) -> Self {
        self.script_action = Some(action);
        self
    }

    pub fn with_toggle_collapsed(
        mut self,
        callback: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        self.on_toggle_collapsed = Some(callback);
        self
    }
}

impl RenderOnce for RightSidebarBottomSplit {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_resize = self.on_begin_resize;
        let on_tab = self.on_select_tab;
        let on_toggle_collapsed = self.on_toggle_collapsed.clone();
        let is_collapsed = self.collapsed;
        let show_run_tab = self.show_run_tab;
        let run_tab_active = self.run_tab_active;
        // The pinned Run tab occupies index 0; terminal tabs shift by one.
        let terminal_index_offset = if show_run_tab { 1 } else { 0 };

        div()
            .id("right-sidebar-bottom-split")
            .w_full()
            .when(!is_collapsed, |s| s.h(px(self.height)))
            .when(is_collapsed, |s| s.h(px(35.0)))
            .flex_none()
            .flex()
            .flex_col()
            .bg(theme.terminal)
            .border_t_1()
            .border_color(theme.sidebar_border)
            .relative()
            // Top horizontal drag handle for resizing height (only when expanded)
            .when(!is_collapsed, |el| {
                el.child(
                    div()
                        .id("right-sidebar-bottom-resize-handle")
                        .absolute()
                        .top(px(-3.0))
                        .left_0()
                        .right_0()
                        .h(px(6.0))
                        .cursor_row_resize()
                        .on_mouse_down(
                            MouseButton::Left,
                            move |event: &MouseDownEvent, window, cx| {
                                cx.stop_propagation();
                                (on_resize)(f32::from(event.position.y), window, cx);
                            },
                        ),
                )
            })
            // Bottom Bar Header: Chevron ([v] / [^]) + Tabs ([Terminal 1] [Terminal 2] [+])
            .child(
                div()
                    .h(px(35.0))
                    .w_full()
                    .flex_none()
                    .flex()
                    .items_center()
                    .justify_between()
                    .px(px(6.0))
                    .bg(theme.sidebar)
                    .border_b_1()
                    .border_color(theme.sidebar_border)
                    // Tabs row: fixed chevron + [+] at the edges, tabs scroll
                    // horizontally in the middle so 5 tabs fit narrow widths.
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .flex()
                            .items_center()
                            .overflow_hidden()
                            // Chevron collapse/expand button
                            .when_some(on_toggle_collapsed, |el, on_toggle| {
                                el.child(
                                    div()
                                        .id("bottom-collapse-btn")
                                        .size(px(20.0))
                                        .rounded(px(4.0))
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .cursor_pointer()
                                        .hover(|s| s.bg(theme.overlay))
                                        .on_click(move |_, window, cx| {
                                            (on_toggle)(window, cx);
                                        })
                                        .child(app_icon(
                                            if is_collapsed {
                                                IconName::ChevronUp
                                            } else {
                                                IconName::ChevronDown
                                            },
                                            12.0,
                                            theme.text_tertiary,
                                        )),
                                )
                            })
                            .child(
                                div()
                                    .id("bottom-tabs-scroll")
                                    .flex_1()
                                    .min_w_0()
                                    .h(px(34.0))
                                    .flex()
                                    .items_center()
                                    .gap(px(4.0))
                                    .overflow_x_scroll()
                                    .when(show_run_tab, |el| {
                                        let on_run_tab = on_tab.clone();
                                        el.child(
                                            div()
                                                .id(gpui::ElementId::from("bottom-tab-run"))
                                                .h(px(34.0))
                                                .flex_none()
                                                .flex_shrink(0.0)
                                                .flex()
                                                .items_center()
                                                .px(px(8.0))
                                                .cursor_pointer()
                                                .when(run_tab_active, |s| {
                                                    s.border_b_2()
                                                        .border_color(theme.accent)
                                                        .text_color(theme.text)
                                                })
                                                .when(!run_tab_active, |s| {
                                                    s.text_color(theme.text_tertiary)
                                                        .hover(|h| h.text_color(theme.text))
                                                })
                                                .text_size(px(11.0))
                                                .font_weight(if run_tab_active {
                                                    gpui::FontWeight::SEMIBOLD
                                                } else {
                                                    gpui::FontWeight::NORMAL
                                                })
                                                .on_click(move |_, window, cx| {
                                                    (on_run_tab)(0, window, cx);
                                                })
                                                .child("Scripts"),
                                        )
                                    })
                                    .children(self.tabs.into_iter().enumerate().map(
                                        |(idx, tab_info)| {
                                            let is_active =
                                                !run_tab_active && idx == self.active_tab_index;
                                            let on_tab = on_tab.clone();
                                            let on_close = self.on_close_tab.clone();
                                            let tab_id = format!("bottom-tab-{}", tab_info.id);
                                            let group_name =
                                                format!("bottom-tab-group-{}", tab_info.id);
                                            // Terminal tabs sit after the pinned Run tab.
                                            let tab_index = idx + terminal_index_offset;

                                            div()
                                                .id(gpui::ElementId::from(tab_id))
                                                .h(px(34.0))
                                                .flex_none()
                                                .flex_shrink(0.0)
                                                .flex()
                                                .items_center()
                                                .gap(px(5.0))
                                                .px(px(8.0))
                                                .cursor_pointer()
                                                .group(group_name.clone())
                                                .when(is_active, |s| {
                                                    s.border_b_2()
                                                        .border_color(theme.accent)
                                                        .text_color(theme.text)
                                                })
                                                .when(!is_active, |s| {
                                                    s.text_color(theme.text_tertiary)
                                                        .hover(|h| h.text_color(theme.text))
                                                })
                                                .text_size(px(11.0))
                                                .font_weight(if is_active {
                                                    gpui::FontWeight::SEMIBOLD
                                                } else {
                                                    gpui::FontWeight::NORMAL
                                                })
                                                .on_click(move |_, window, cx| {
                                                    (on_tab)(tab_index, window, cx);
                                                })
                                                .child(tab_info.title)
                                                // Close Tab Button: hidden until the tab is hovered
                                                .when_some(on_close, |el, on_close| {
                                                    el.child(
                                                        div()
                                                            .id(gpui::ElementId::from(format!(
                                                                "close-terminal-{}",
                                                                tab_info.id
                                                            )))
                                                            .size(px(14.0))
                                                            .rounded(px(2.0))
                                                            .flex()
                                                            .items_center()
                                                            .justify_center()
                                                            .cursor_pointer()
                                                            .invisible()
                                                            .group_hover(group_name.clone(), |el| {
                                                                el.visible()
                                                            })
                                                            .hover(|s| s.bg(theme.overlay_strong))
                                                            .on_mouse_down(
                                                                MouseButton::Left,
                                                                move |_, window, cx| {
                                                                    cx.stop_propagation();
                                                                    (on_close)(
                                                                        tab_index, window, cx,
                                                                    );
                                                                },
                                                            )
                                                            .child(app_icon(
                                                                IconName::X,
                                                                10.0,
                                                                theme.text_tertiary,
                                                            )),
                                                    )
                                                })
                                        },
                                    )),
                            )
                            .when_some(self.on_new_terminal, |el, on_new| {
                                el.child(
                                    div()
                                        .id("bottom-new-terminal-btn")
                                        .size(px(20.0))
                                        .flex_none()
                                        .flex_shrink(0.0)
                                        .rounded(px(4.0))
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .cursor_pointer()
                                        .hover(|s| s.bg(theme.overlay))
                                        .on_click(move |_, window, cx| {
                                            (on_new)(window, cx);
                                        })
                                        .child(app_icon(IconName::Plus, 11.0, theme.text)),
                                )
                            })
                            .when_some(
                                self.on_refresh_run.filter(|_| run_tab_active),
                                |el, on_refresh| {
                                    el.child(
                                        div()
                                            .id("bottom-refresh-run-btn")
                                            .size(px(20.0))
                                            .flex_none()
                                            .flex_shrink(0.0)
                                            .rounded(px(4.0))
                                            .flex()
                                            .items_center()
                                            .justify_center()
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay))
                                            .on_click(move |_, window, cx| {
                                                (on_refresh)(window, cx);
                                            })
                                            .child(app_icon(
                                                IconName::Refresh,
                                                11.0,
                                                theme.text,
                                            )),
                                    )
                                },
                            )
                            .when_some(
                                self.script_action.filter(|_| !run_tab_active),
                                |el, action| {
                                    let on_click = action.on_click;
                                    el.child(
                                        div()
                                            .id("bottom-script-action-btn")
                                            .size(px(20.0))
                                            .flex_none()
                                            .flex_shrink(0.0)
                                            .rounded(px(4.0))
                                            .flex()
                                            .items_center()
                                            .justify_center()
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay))
                                            .on_click(move |_, window, cx| {
                                                (on_click)(window, cx);
                                            })
                                            .child(app_icon(
                                                if action.running {
                                                    IconName::StopFilled
                                                } else {
                                                    IconName::Play
                                                },
                                                11.0,
                                                if action.running {
                                                    theme.danger
                                                } else {
                                                    theme.text
                                                },
                                            )),
                                    )
                                },
                            ),
                    ),
            )
            // Bottom Content Body (only shown when expanded)
            .when(!is_collapsed, |el| {
                el.child(div().flex_1().w_full().min_h_0().overflow_hidden().child(
                    if run_tab_active {
                        match self.run_element {
                            Some(run) => run,
                            None => div()
                                .size_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .text_size(px(11.0))
                                .text_color(theme.text_ghost)
                                .child("No project selected")
                                .into_any_element(),
                        }
                    } else {
                        match self.terminal_element {
                            Some(term) => term,
                            None => div()
                                .size_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .text_size(px(11.0))
                                .text_color(theme.text_ghost)
                                .child("No terminal active")
                                .into_any_element(),
                        }
                    },
                ))
            })
    }
}
