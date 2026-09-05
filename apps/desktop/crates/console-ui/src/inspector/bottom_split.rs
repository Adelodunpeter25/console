//! Bottom Split Execution & Terminal Panel for the Right Sidebar Inspector.

use gpui::{
    App, InteractiveElement, IntoElement, MouseButton, MouseDownEvent, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub enum RightSidebarBottomTab {
    #[default]
    Terminal,
}

#[derive(IntoElement)]
pub struct RightSidebarBottomSplit {
    height: f32,
    active_tab: RightSidebarBottomTab,
    terminal_element: Option<gpui::AnyElement>,
    on_select_tab: Rc<dyn Fn(RightSidebarBottomTab, &mut Window, &mut App) + 'static>,
    on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    on_new_terminal: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl RightSidebarBottomSplit {
    pub fn new(
        height: f32,
        active_tab: RightSidebarBottomTab,
        terminal_element: Option<gpui::AnyElement>,
        on_select_tab: Rc<dyn Fn(RightSidebarBottomTab, &mut Window, &mut App) + 'static>,
        on_begin_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            height,
            active_tab,
            terminal_element,
            on_select_tab,
            on_begin_resize,
            on_new_terminal: None,
        }
    }

    pub fn with_new_terminal(
        mut self,
        callback: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        self.on_new_terminal = Some(callback);
        self
    }
}

impl RenderOnce for RightSidebarBottomSplit {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_resize = self.on_begin_resize;
        let on_tab = self.on_select_tab;

        div()
            .id("right-sidebar-bottom-split")
            .w_full()
            .h(px(self.height))
            .flex_none()
            .flex()
            .flex_col()
            .bg(theme.terminal)
            .border_t_1()
            .border_color(theme.sidebar_border)
            .relative()
            // Top horizontal drag handle for resizing height
            .child(
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
            // Bottom Bar Header: Tabs ([Terminal] [+])
            .child(
                div()
                    .h(px(32.0))
                    .w_full()
                    .flex_none()
                    .flex()
                    .items_center()
                    .justify_between()
                    .px(px(8.0))
                    .bg(theme.sidebar)
                    .border_b_1()
                    .border_color(theme.sidebar_border)
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .gap(px(4.0))
                            .child({
                                let is_active = self.active_tab == RightSidebarBottomTab::Terminal;
                                let on_tab = on_tab.clone();
                                div()
                                    .id("bottom-tab-terminal")
                                    .flex()
                                    .items_center()
                                    .gap(px(4.0))
                                    .px(px(6.0))
                                    .py(px(2.0))
                                    .rounded(px(4.0))
                                    .cursor_pointer()
                                    .bg(if is_active {
                                        theme.surface
                                    } else {
                                        gpui::transparent_black()
                                    })
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
                                    .hover(|s| s.bg(theme.overlay))
                                    .on_click(move |_, window, cx| {
                                        (on_tab)(RightSidebarBottomTab::Terminal, window, cx);
                                    })
                                    .child(app_icon(
                                        IconName::Terminal,
                                        12.0,
                                        if is_active {
                                            theme.text
                                        } else {
                                            theme.text_tertiary
                                        },
                                    ))
                                    .child("Terminal")
                            }),
                    )
                    .when_some(self.on_new_terminal, |el, on_new| {
                        el.child(
                            div()
                                .id("bottom-new-terminal-btn")
                                .size(px(20.0))
                                .rounded(px(4.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay))
                                .on_click(move |_, window, cx| {
                                    (on_new)(window, cx);
                                })
                                .child(app_icon(IconName::Plus, 11.0, theme.text_tertiary)),
                        )
                    }),
            )
            // Bottom Content Body
            .child(
                div()
                    .flex_1()
                    .w_full()
                    .min_h_0()
                    .overflow_hidden()
                    .child(match self.terminal_element {
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
                    }),
            )
    }
}
