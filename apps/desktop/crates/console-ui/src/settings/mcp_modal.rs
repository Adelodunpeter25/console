//! The add/edit MCP server form, as a dialog over the settings.
//!
//! A dialog, like the server form: the list stays where it was, and the
//! form is something you do on top of it rather than a page you go to.

use crate::input::ComposerInput;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use console_core::types::mcp::{McpAuthType, McpTransportType};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

use super::McpModalMode;

/// The add/edit MCP server form.
#[derive(IntoElement)]
pub struct McpModal {
    pub modal_mode: McpModalMode,
    pub selected_transport: McpTransportType,
    pub selected_auth: McpAuthType,
    pub name_input: Option<Entity<ComposerInput>>,
    pub url_input: Option<Entity<ComposerInput>>,
    pub command_input: Option<Entity<ComposerInput>>,
    pub args_input: Option<Entity<ComposerInput>>,
    pub env_input: Option<Entity<ComposerInput>>,
    /// The static-auth secret. Only shown for a remote server with static
    /// auth, and only ever empty: the stored token is never sent back.
    pub token_input: Option<Entity<ComposerInput>>,
    pub action_error: Option<String>,
    pub is_submitting: bool,
    pub on_close_modal: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_select_transport: Rc<dyn Fn(McpTransportType, &mut Window, &mut App) + 'static>,
    pub on_select_auth: Rc<dyn Fn(McpAuthType, &mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for McpModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close_modal.clone();
        // The backdrop closes the dialog; the card stops the click so
        // clicking inside it does not dismiss.
        let backdrop_close = on_close.clone();
        let on_save = self.on_save.clone();
        let on_select_trans = self.on_select_transport.clone();
        let on_select_auth = self.on_select_auth.clone();

        let is_edit = match self.modal_mode {
            McpModalMode::Add => false,
            McpModalMode::Edit(_) => true,
        };

        // The fields scroll; the title and the buttons stay put, so a long
        // form on a short window still has a way out.
        let body = div()
            .id("mcp-modal-body")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .flex()
            .flex_col()
            .gap(px(12.0))
            // Name Input
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(12.0))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text)
                            .child("Server Name / ID"),
                    )
                    .when_some(self.name_input.clone(), |el, input| {
                        el.child(
                            div()
                                .px(px(8.0))
                                .py(px(4.0))
                                .rounded(px(6.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.canvas)
                                .child(input),
                        )
                    }),
            )
            // Transport Type Selector (stdio vs http)
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(12.0))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text)
                            .child("Transport Type"),
                    )
                    .child(
                        div()
                            .flex()
                            .gap(px(8.0))
                            .child({
                                let on_sel = on_select_trans.clone();
                                let is_sel = self.selected_transport == McpTransportType::Stdio;
                                div()
                                    .px(px(10.0))
                                    .py(px(5.0))
                                    .rounded(px(6.0))
                                    .cursor_pointer()
                                    .bg(if is_sel { theme.accent } else { theme.overlay })
                                    .text_color(if is_sel { theme.on_inverse } else { theme.text })
                                    .text_size(px(12.0))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                        cx.stop_propagation();
                                        (on_sel)(McpTransportType::Stdio, window, cx);
                                    })
                                    .child("Local (stdio)")
                            })
                            .child({
                                let on_sel = on_select_trans.clone();
                                let is_sel = self.selected_transport == McpTransportType::Http;
                                div()
                                    .px(px(10.0))
                                    .py(px(5.0))
                                    .rounded(px(6.0))
                                    .cursor_pointer()
                                    .bg(if is_sel { theme.accent } else { theme.overlay })
                                    .text_color(if is_sel { theme.on_inverse } else { theme.text })
                                    .text_size(px(12.0))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                        cx.stop_propagation();
                                        (on_sel)(McpTransportType::Http, window, cx);
                                    })
                                    .child("Remote (HTTP / SSE)")
                            }),
                    ),
            )
            // Fields for stdio
            .when(self.selected_transport == McpTransportType::Stdio, |el| {
                el.child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(8.0))
                        .child(
                            div()
                                .flex()
                                .flex_col()
                                .gap(px(4.0))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child("Command"),
                                )
                                .when_some(self.command_input.clone(), |el, input| {
                                    el.child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
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
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child("Arguments (space-delimited)"),
                                )
                                .when_some(self.args_input.clone(), |el, input| {
                                    el.child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
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
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child("Environment Variables (KEY=VAL, comma-separated)"),
                                )
                                .when_some(self.env_input.clone(), |el, input| {
                                    el.child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(6.0))
                                            .border_1()
                                            .border_color(theme.border)
                                            .bg(theme.canvas)
                                            .child(input),
                                    )
                                }),
                        ),
                )
            })
            // Fields for HTTP
            .when(self.selected_transport == McpTransportType::Http, |el| {
                el.child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(8.0))
                        .child(
                            div()
                                .flex()
                                .flex_col()
                                .gap(px(4.0))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child("Endpoint URL"),
                                )
                                .when_some(self.url_input.clone(), |el, input| {
                                    el.child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
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
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child("Authentication"),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .gap(px(8.0))
                                        .child({
                                            let on_sel = on_select_auth.clone();
                                            let is_sel = self.selected_auth == McpAuthType::Oauth2;
                                            div()
                                                .px(px(10.0))
                                                .py(px(5.0))
                                                .rounded(px(6.0))
                                                .cursor_pointer()
                                                .bg(if is_sel {
                                                    theme.accent
                                                } else {
                                                    theme.overlay
                                                })
                                                .text_color(if is_sel {
                                                    theme.on_inverse
                                                } else {
                                                    theme.text
                                                })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(
                                                    MouseButton::Left,
                                                    move |_event, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_sel)(McpAuthType::Oauth2, window, cx);
                                                    },
                                                )
                                                .child("OAuth 2.1 (Browser)")
                                        })
                                        .child({
                                            let on_sel = on_select_auth.clone();
                                            let is_sel = self.selected_auth == McpAuthType::Static;
                                            div()
                                                .px(px(10.0))
                                                .py(px(5.0))
                                                .rounded(px(6.0))
                                                .cursor_pointer()
                                                .bg(if is_sel {
                                                    theme.accent
                                                } else {
                                                    theme.overlay
                                                })
                                                .text_color(if is_sel {
                                                    theme.on_inverse
                                                } else {
                                                    theme.text
                                                })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(
                                                    MouseButton::Left,
                                                    move |_event, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_sel)(McpAuthType::Static, window, cx);
                                                    },
                                                )
                                                .child("Static Token")
                                        })
                                        .child({
                                            let on_sel = on_select_auth.clone();
                                            let is_sel = self.selected_auth == McpAuthType::None;
                                            div()
                                                .px(px(10.0))
                                                .py(px(5.0))
                                                .rounded(px(6.0))
                                                .cursor_pointer()
                                                .bg(if is_sel {
                                                    theme.accent
                                                } else {
                                                    theme.overlay
                                                })
                                                .text_color(if is_sel {
                                                    theme.on_inverse
                                                } else {
                                                    theme.text
                                                })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(
                                                    MouseButton::Left,
                                                    move |_event, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_sel)(McpAuthType::None, window, cx);
                                                    },
                                                )
                                                .child("None")
                                        }),
                                ),
                        ),
                )
            });

        let card = div()
            .w(px(520.0))
            .max_h_full()
            .p(px(16.0))
            .rounded(px(10.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.surface)
            .shadow_lg()
            .flex()
            .flex_col()
            .gap(px(12.0))
            // The card stops the click so clicking inside it does not
            // dismiss the dialog.
            .on_mouse_down(MouseButton::Left, |_, _, cx| {
                cx.stop_propagation();
            })
            // Modal Title
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_between()
                    .child(
                        div()
                            .text_size(px(14.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child(if is_edit {
                                "Edit MCP Server"
                            } else {
                                "Add MCP Server"
                            }),
                    )
                    .child(
                        div()
                            .id("btn-close-mcp-modal")
                            .cursor_pointer()
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_close)(window, cx);
                            })
                            .child(app_icon(IconName::Close, 14.0, theme.text_secondary)),
                    ),
            )
            .child(body)
            // The static-auth secret. Only a remote server authenticates
            // with a token, and the field is always empty: the server
            // stores the token and never sends it back, so an empty field
            // leaves the stored one alone.
            .when(
                self.selected_transport == McpTransportType::Http
                    && self.selected_auth == McpAuthType::Static,
                |el| {
                    el.child(
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(4.0))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .text_color(theme.text)
                                    .child("Token"),
                            )
                            .when_some(self.token_input.clone(), |el, input| {
                                el.child(
                                    div()
                                        .px(px(8.0))
                                        .py(px(4.0))
                                        .rounded(px(6.0))
                                        .border_1()
                                        .border_color(theme.border)
                                        .bg(theme.canvas)
                                        .child(input),
                                )
                            }),
                    )
                },
            )
            // Footer Action buttons
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_end()
                    .gap(px(8.0))
                    .mt(px(6.0))
                    .child({
                        let on_cl = self.on_close_modal.clone();
                        div()
                            .px(px(10.0))
                            .py(px(5.0))
                            .rounded(px(6.0))
                            .cursor_pointer()
                            .bg(theme.overlay)
                            .hover(|s| s.bg(theme.overlay_strong))
                            .text_size(px(12.0))
                            .text_color(theme.text)
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_cl)(window, cx);
                            })
                            .child("Cancel")
                    })
                    .child({
                        let on_sv = on_save.clone();
                        let disabled = self.is_submitting;
                        div()
                            .px(px(12.0))
                            .py(px(5.0))
                            .rounded(px(6.0))
                            .cursor_pointer()
                            .bg(theme.accent)
                            .hover(|s| if !disabled { s.opacity(0.9) } else { s })
                            .text_size(px(12.0))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.on_inverse)
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                if !disabled {
                                    cx.stop_propagation();
                                    (on_sv)(window, cx);
                                }
                            })
                            .child(if self.is_submitting {
                                "Saving..."
                            } else {
                                "Save Server"
                            })
                    }),
            );

        div()
            .id("mcp-modal")
            .absolute()
            .inset_0()
            .bg(gpui::hsla(0.0, 0.0, 0.0, 0.6))
            .flex()
            .items_center()
            .justify_center()
            .p(px(24.0))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                (backdrop_close)(window, cx);
            })
            .child(card)
    }
}
