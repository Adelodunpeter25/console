//! The unified window title bar — mirrors the Electron app's `TitleBar`.
//!
//! The title bar spans the whole window and is reserved for the **selected
//! session's identity**: its chat title and folder name, centered. The sidebar
//! toggle lives here too, pinned to the right edge of the sidebar region so it
//! sits right beside the sidebar divider.

use std::rc::Rc;

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::{TITLEBAR_HEIGHT, Theme};

/// Full-width window title bar. Shows `title` centered (e.g.
/// "can you see this image — Gpui-ui") plus a sidebar toggle on the left
/// and an inspector toggle on the right.
#[derive(IntoElement)]
pub struct TitleBar {
    title: Option<String>,
    sidebar_width: f32,
    on_toggle_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle_right_sidebar: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    right_sidebar_open: bool,
}

impl TitleBar {
    pub fn new(
        title: Option<String>,
        sidebar_width: f32,
        on_toggle_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            title,
            sidebar_width,
            on_toggle_sidebar,
            on_toggle_right_sidebar: None,
            right_sidebar_open: false,
        }
    }

    pub fn with_right_sidebar_toggle(
        mut self,
        open: bool,
        on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        self.right_sidebar_open = open;
        self.on_toggle_right_sidebar = Some(on_toggle);
        self
    }
}

impl RenderOnce for TitleBar {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_toggle = self.on_toggle_sidebar;
        let on_toggle_right = self.on_toggle_right_sidebar;
        let title = self.title;
        let sidebar_width = self.sidebar_width;

        div()
            .id("window-titlebar")
            .h(TITLEBAR_HEIGHT)
            .w_full()
            .flex_none()
            .flex()
            .items_center()
            .bg(theme.sidebar)
            .border_b_1()
            .border_color(theme.sidebar_border)
            // Left: sidebar-width block keeps the center title optically
            // centered. The toggle sits right after the macOS traffic lights
            // (traffic lights end around x≈66, so 72 leaves a small gap).
            .child(
                div()
                    .w(px(sidebar_width))
                    .flex_none()
                    .flex()
                    .items_center()
                    .pl(px(72.0))
                    .child(
                        div()
                            .id("titlebar-sidebar-toggle")
                            .mt(px(4.0))
                            .size(px(26.0))
                            .rounded(px(6.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_default()
                            .hover(|s| s.bg(theme.overlay))
                            .active(|s| s.bg(theme.overlay_strong))
                            .on_click(move |_, window, cx| (on_toggle)(window, cx))
                            .child(app_icon(IconName::PanelLeft, 14.0, theme.text_tertiary)),
                    ),
            )
            // Center: the selected session's chat title — folder name. It is
            // dead-center because the side blocks are equal width.
            .child(
                div()
                    .flex_1()
                    .flex()
                    .justify_center()
                    .min_w_0()
                    .when_some(title, |el, title| {
                        el.child(
                            div()
                                .truncate()
                                .max_w(px(480.0))
                                .text_size(px(12.0))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.text_secondary)
                                .child(title),
                        )
                    }),
            )
            // Right: spacer + right inspector toggle button
            .child(
                div()
                    .w(px(sidebar_width))
                    .flex_none()
                    .flex()
                    .items_center()
                    .justify_end()
                    .pr(px(12.0))
                    .when_some(on_toggle_right, |el, on_toggle_right| {
                        el.child(
                            div()
                                .id("titlebar-right-sidebar-toggle")
                                .mt(px(4.0))
                                .size(px(26.0))
                                .rounded(px(6.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .cursor_default()
                                .hover(|s| s.bg(theme.overlay))
                                .active(|s| s.bg(theme.overlay_strong))
                                .on_click(move |_, window, cx| (on_toggle_right)(window, cx))
                                .child(app_icon(
                                    IconName::PanelRight,
                                    14.0,
                                    theme.text_tertiary,
                                )),
                        )
                    }),
            )
    }
}
