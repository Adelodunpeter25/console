//! Forwarded ports popover matching the Conductor top-right title bar design.

use std::rc::Rc;

use console_core::ForwardedPort;
use gpui::{
    div, prelude::*, px, App, IntoElement,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window,
};

use crate::primitives::menu::{popover, MenuAlign};
use crate::primitives::{app_icon, ContextMenuHandle, IconName};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct PortsPopover {
    ports: Rc<Vec<ForwardedPort>>,
    menu_handle: ContextMenuHandle,
    on_open_url: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_unforward: Rc<dyn Fn(u16, &mut Window, &mut App) + 'static>,
    on_forward: Rc<dyn Fn(u16, &mut Window, &mut App) + 'static>,
}

impl PortsPopover {
    pub fn new(
        ports: Rc<Vec<ForwardedPort>>,
        menu_handle: ContextMenuHandle,
        on_open_url: impl Fn(String, &mut Window, &mut App) + 'static,
        on_unforward: impl Fn(u16, &mut Window, &mut App) + 'static,
        on_forward: impl Fn(u16, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            ports,
            menu_handle,
            on_open_url: Rc::new(on_open_url),
            on_unforward: Rc::new(on_unforward),
            on_forward: Rc::new(on_forward),
        }
    }
}

impl RenderOnce for PortsPopover {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let ports = self.ports.clone();
        let has_ports = !ports.is_empty();
        let on_open = self.on_open_url.clone();
        let on_unforward = self.on_unforward.clone();
        let on_forward = self.on_forward.clone();

        let trigger_icon_color = if has_ports {
            theme.accent
        } else {
            theme.text_tertiary
        };

        let trigger = div()
            .id("titlebar-ports-trigger")
            .mt(px(4.0))
            .size(px(26.0))
            .rounded(px(6.0))
            .flex()
            .items_center()
            .justify_center()
            .cursor_pointer()
            .hover(|s| s.bg(theme.overlay))
            .active(|s| s.bg(theme.overlay_strong))
            .child(app_icon(IconName::Port, 14.0, trigger_icon_color));

        popover(
            trigger,
            &self.menu_handle,
            MenuAlign::BelowLeft,
            move |handle, _window, cx| {
                let theme = Theme::current(cx);
                let ports = ports.clone();
                let on_open = on_open.clone();
                let on_unforward = on_unforward.clone();
                let on_forward = on_forward.clone();
                let handle_close = handle.clone();

                div()
                    .w(px(240.0))
                    .p(px(8.0))
                    .rounded(px(8.0))
                    .bg(theme.surface)
                    .border_1()
                    .border_color(theme.border)
                    .shadow_md()
                    .flex()
                    .flex_col()
                    .gap(px(6.0))
                    .child(
                        div()
                            .px(px(6.0))
                            .py(px(4.0))
                            .text_size(px(12.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text_secondary)
                            .child("Forwarded ports"),
                    )
                    .child(if ports.is_empty() {
                        div()
                            .px(px(6.0))
                            .py(px(8.0))
                            .text_size(px(11.5))
                            .text_color(theme.text_ghost)
                            .child("No active forwarded ports")
                            .into_any_element()
                    } else {
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(2.0))
                            .children(ports.iter().map(|p| {
                                let port_num = p.port;
                                let port_url = p.url.clone();
                                let on_open = on_open.clone();
                                let on_unforward = on_unforward.clone();
                                let handle_close = handle_close.clone();

                                div()
                                    .h(px(28.0))
                                    .px(px(6.0))
                                    .rounded(px(4.0))
                                    .flex()
                                    .items_center()
                                    .justify_between()
                                    .hover(|s| s.bg(theme.overlay))
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(6.0))
                                            // Active green dot
                                            .child(
                                                div()
                                                    .size(px(6.0))
                                                    .rounded_full()
                                                    .bg(theme.success),
                                            )
                                            // Port number
                                            .child(
                                                div()
                                                    .text_size(px(12.5))
                                                    .font_weight(gpui::FontWeight::MEDIUM)
                                                    .text_color(theme.text)
                                                    .child(port_num.to_string()),
                                            ),
                                    )
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(2.0))
                                            // Globe icon button -> open in browser
                                            .child(
                                                div()
                                                    .id(format!("open-port-{}", port_num))
                                                    .size(px(22.0))
                                                    .rounded(px(4.0))
                                                    .flex()
                                                    .items_center()
                                                    .justify_center()
                                                    .cursor_pointer()
                                                    .hover(|s| s.bg(theme.overlay_strong))
                                                    .on_click({
                                                        let port_url = port_url.clone();
                                                        let on_open = on_open.clone();
                                                        let handle_close = handle_close.clone();
                                                        move |_, window, cx| {
                                                            handle_close.close(window, cx);
                                                            (on_open)(port_url.clone(), window, cx);
                                                        }
                                                    })
                                                    .child(app_icon(IconName::Globe, 12.0, theme.text_secondary)),
                                            )
                                            // X icon button -> unforward
                                            .child(
                                                div()
                                                    .id(format!("unforward-port-{}", port_num))
                                                    .size(px(22.0))
                                                    .rounded(px(4.0))
                                                    .flex()
                                                    .items_center()
                                                    .justify_center()
                                                    .cursor_pointer()
                                                    .hover(|s| s.bg(theme.danger_soft))
                                                    .on_click({
                                                        let on_unforward = on_unforward.clone();
                                                        let handle_close = handle_close.clone();
                                                        move |_, window, cx| {
                                                            handle_close.close(window, cx);
                                                            (on_unforward)(port_num, window, cx);
                                                        }
                                                    })
                                                    .child(app_icon(IconName::X, 11.0, theme.text_tertiary)),
                                            ),
                                    )
                            }))
                            .into_any_element()
                    })
                    .child(
                        div()
                            .h(px(1.0))
                            .w_full()
                            .bg(theme.border)
                            .my(px(2.0)),
                    )
                    // Bottom "+ Add port" action
                    .child(
                        div()
                            .id("add-forwarded-port-btn")
                            .h(px(26.0))
                            .px(px(6.0))
                            .rounded(px(4.0))
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .on_click(move |_, _window, _cx| {
                                (on_forward)(3000, _window, _cx);
                            })
                            .child(app_icon(IconName::Plus, 11.0, theme.text_tertiary))
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .text_color(theme.text_secondary)
                                    .child("Add port"),
                            ),
                    )
                    .into_any_element()
            },
        )
    }
}
