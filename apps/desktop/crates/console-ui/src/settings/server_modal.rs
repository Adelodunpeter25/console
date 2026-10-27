//! The server modals: adding or editing a server, and pairing a phone.
//!
//! Both are modals rather than inline forms. The settings window is already
//! a tab away from everything else, so a form that pushes the list out of
//! the way reads as a second page rather than as a small task on top of
//! the one you were on.

use super::ProbeState;
use crate::input::ComposerInput;
use crate::theme::Theme;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, img, px,
};
use std::rc::Rc;
use std::sync::Arc;

/// A device's name, or its platform when it has no name.
fn device_label(device: &console_core::services::ServerDevice) -> String {
    if device.name.trim().is_empty() {
        device.platform.clone()
    } else {
        device.name.clone()
    }
}

/// The add/edit server modal.
#[derive(IntoElement)]
pub struct ServerModal {
    /// True when an existing server is being edited.
    pub is_edit: bool,
    pub name_input: Option<Entity<ComposerInput>>,
    pub host_input: Option<Entity<ComposerInput>>,
    /// The pairing code, redeemed for this device's own credential.
    pub code_input: Option<Entity<ComposerInput>>,
    pub probe_state: ProbeState,
    /// The server's failure text, shown verbatim.
    pub error: Option<String>,
    pub on_close: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_probe: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for ServerModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close.clone();
        let on_probe = self.on_probe.clone();
        let on_save = self.on_save.clone();

        let (probe_text, probe_color) = match self.probe_state {
            ProbeState::Ok => ("Connected successfully", theme.accent),
            ProbeState::Failed => ("Could not reach server", theme.danger),
            ProbeState::Probing => ("Testing connection...", theme.accent),
            ProbeState::Unknown => ("Click Test to verify connection", theme.text_ghost),
        };

        // The fields scroll inside the card: on a short window the
        // buttons below have to stay reachable.
        let body = div()
            .id("server-modal-body")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .flex()
            .flex_col()
            .gap(px(12.0))
            // The name is only editable for a server that already exists:
            // a new one is named after the server it pairs with.
            .when(self.is_edit, |el| {
                el.child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(4.0))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.text_secondary)
                                .child("Environment Name"),
                        )
                        .when_some(self.name_input.clone(), |el, input| {
                            el.child(
                                div()
                                    .p(px(6.0))
                                    .rounded(px(6.0))
                                    .border_1()
                                    .border_color(theme.border)
                                    .bg(theme.canvas)
                                    .child(input),
                            )
                        }),
                )
            })
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(11.5))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child("Host (e.g. 192.168.1.50:3000)"),
                    )
                    .when_some(self.host_input.clone(), |el, input| {
                        el.child(
                            div()
                                .p(px(6.0))
                                .rounded(px(6.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.canvas)
                                .child(input),
                        )
                    }),
            )
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(11.5))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child("Pairing code (from 'console pair' on the server)"),
                    )
                    .when_some(self.code_input.clone(), |el, input| {
                        el.child(
                            div()
                                .p(px(6.0))
                                .rounded(px(6.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.canvas)
                                .child(input),
                        )
                    }),
            )
            .when_some(self.error.clone(), |el, error| {
                el.child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.danger)
                        .child(error),
                )
            });

        // The action row: probe status on the left, Cancel/Test/Save on the
        // right. The shell draws the title bar and backdrop; only the body
        // and footer are this modal's own.
        let footer = div()
            .flex()
            .items_center()
            .justify_between()
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .child(div().w(px(7.0)).h(px(7.0)).rounded_full().bg(probe_color))
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(probe_color)
                            .child(probe_text),
                    ),
            )
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child(
                        div()
                            .px(px(8.0))
                            .py(px(4.0))
                            .rounded(px(5.0))
                            .border_1()
                            .border_color(theme.border_strong)
                            .bg(theme.raised)
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_probe)(window, cx);
                            })
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .text_color(theme.text)
                                    .child("Test connection"),
                            ),
                    )
                    .child(
                        div()
                            .px(px(8.0))
                            .py(px(4.0))
                            .rounded(px(5.0))
                            .cursor_pointer()
                            .hover(|s| s.bg(theme.overlay))
                            .on_mouse_down(MouseButton::Left, {
                                let close_click = on_close.clone();
                                move |_event, window, cx| {
                                    cx.stop_propagation();
                                    (close_click)(window, cx);
                                }
                            })
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .text_color(theme.text_secondary)
                                    .child("Cancel"),
                            ),
                    )
                    .child(
                        div()
                            .px(px(10.0))
                            .py(px(4.0))
                            .rounded(px(5.0))
                            .bg(theme.accent)
                            .cursor_pointer()
                            .hover(|s| s.opacity(0.9))
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_save)(window, cx);
                            })
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .text_color(theme.on_inverse)
                                    .child(if self.is_edit { "Save" } else { "Add" }),
                            ),
                    ),
            );

        super::Modal {
            title: if self.is_edit {
                "Edit Server".to_string()
            } else {
                "Add Server".to_string()
            },
            width: 520.0,
            content: body.into_any_element(),
            footer: footer.into_any_element(),
            on_close,
        }
    }
}

/// The "pair a phone" modal: a QR code the phone scans, and the devices
/// already paired with this server.
#[derive(IntoElement)]
pub struct PairDeviceModal {
    /// The grant waiting to be redeemed, with its QR code.
    pub grant: Option<console_core::services::PairingGrantData>,
    /// This device's record on the server, when it is paired.
    pub this_device: Option<console_core::services::ServerDevice>,
    /// Every device paired with the server.
    pub devices: Vec<console_core::services::ServerDevice>,
    /// The server's failure text, shown verbatim.
    pub error: Option<String>,
    pub on_close: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    /// Revoke another device by id.
    pub on_revoke: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    /// Forget this device's own credential.
    pub on_unpair: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for PairDeviceModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close.clone();
        let on_refresh = self.on_refresh.clone();
        let on_revoke = self.on_revoke.clone();
        let on_unpair = self.on_unpair.clone();

        let qr = self.grant.as_ref().and_then(|grant| {
            (!grant.qr_png.is_empty()).then(|| {
                Arc::new(gpui::Image::from_bytes(
                    gpui::ImageFormat::Png,
                    grant.qr_png.clone(),
                ))
            })
        });

        // The card is bounded by the window and its middle scrolls: a
        // server with a dozen paired devices must not push the buttons
        // off the bottom of the screen.
        let mut body = div()
            .id("pair-modal-body")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .flex()
            .flex_col()
            .gap(px(12.0))
            .child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_secondary)
                    .child("Scan this with the Console app. It works once."),
            );

        if let Some(grant) = self.grant.as_ref() {
            body = body
                .when_some(qr, |el, image| {
                    el.child(
                        div().flex().justify_center().child(
                            div()
                                .p(px(10.0))
                                .rounded(px(8.0))
                                .bg(gpui::white())
                                // The size belongs on the image itself:
                                // an image lays out at its natural
                                // size, so a box around it does not
                                // shrink it.
                                .child(img(image).w(px(200.0)).h(px(200.0)).flex_none()),
                        ),
                    )
                })
                .child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(4.0))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.text_secondary)
                                .child("Or type this code"),
                        )
                        .child(
                            div()
                                .text_size(px(16.0))
                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                .text_color(theme.text)
                                .child(grant.grant.clone()),
                        ),
                );
        } else {
            body = body.child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_secondary)
                    .child("Making a code..."),
            );
        }

        // The devices already paired with this server. The QR code above
        // adds one more.
        let others: Vec<_> = self
            .devices
            .iter()
            .filter(|device| {
                self.this_device
                    .as_ref()
                    .map(|this| device.id != this.id)
                    .unwrap_or(true)
            })
            .collect();
        if !others.is_empty() {
            body = body.child(
                div()
                    .text_size(px(11.5))
                    .font_weight(gpui::FontWeight::MEDIUM)
                    .text_color(theme.text_secondary)
                    .child("Paired devices"),
            );
            for device in others {
                let id = device.id.clone();
                let on_revoke_click = on_revoke.clone();
                body = body.child(
                    div()
                        .flex()
                        .items_center()
                        .justify_between()
                        .child(
                            div()
                                .text_size(px(12.5))
                                .text_color(theme.text)
                                .child(device_label(device)),
                        )
                        .child(
                            div()
                                .id(ElementId::from(format!("btn-revoke-device-{id}")))
                                .flex_shrink_0()
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(5.0))
                                .border_1()
                                .border_color(theme.border_strong)
                                .bg(theme.raised)
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay))
                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                    cx.stop_propagation();
                                    (on_revoke_click)(id.clone(), window, cx);
                                })
                                .child(
                                    div()
                                        .text_size(px(11.5))
                                        .text_color(theme.text_secondary)
                                        .child("Revoke"),
                                ),
                        ),
                );
            }
        }

        if let Some(error) = self.error.clone() {
            body = body.child(
                div()
                    .text_size(px(11.5))
                    .text_color(theme.danger)
                    .child(error),
            );
        }

        // "Unpair this device" belongs to this modal's own state, so it sits
        // above the action row; the shell's footer slot holds both.
        let mut footer_wrapper = div().flex().flex_col().gap(px(12.0));
        if self.this_device.is_some() {
            let on_unpair_click = on_unpair.clone();
            footer_wrapper = footer_wrapper.child(
                div()
                    .id("btn-unpair-device")
                    .self_start()
                    .px(px(8.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.raised)
                    .cursor_pointer()
                    .hover(|s| s.bg(theme.overlay))
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        cx.stop_propagation();
                        (on_unpair_click)(window, cx);
                    })
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .child("Unpair this device"),
                    ),
            );
        }

        footer_wrapper = footer_wrapper.child(
            div()
                .flex()
                .items_center()
                .justify_end()
                .gap(px(8.0))
                .child(
                    div()
                        .px(px(8.0))
                        .py(px(4.0))
                        .rounded(px(5.0))
                        .border_1()
                        .border_color(theme.border_strong)
                        .bg(theme.raised)
                        .cursor_pointer()
                        .hover(|s| s.bg(theme.overlay))
                        .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                            cx.stop_propagation();
                            (on_refresh)(window, cx);
                        })
                        .child(
                            div()
                                .text_size(px(11.5))
                                .text_color(theme.text)
                                .child("Make a new code"),
                        ),
                )
                .child(
                    div()
                        .px(px(10.0))
                        .py(px(4.0))
                        .rounded(px(5.0))
                        .bg(theme.accent)
                        .cursor_pointer()
                        .hover(|s| s.opacity(0.9))
                        .on_mouse_down(MouseButton::Left, {
                            let close_click = on_close.clone();
                            move |_event, window, cx| {
                                cx.stop_propagation();
                                (close_click)(window, cx);
                            }
                        })
                        .child(
                            div()
                                .text_size(px(11.5))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.on_inverse)
                                .child("Done"),
                        ),
                ),
        );

        super::Modal {
            title: "Pair a Phone".to_string(),
            width: 360.0,
            content: body.into_any_element(),
            footer: footer_wrapper.into_any_element(),
            on_close,
        }
    }
}
