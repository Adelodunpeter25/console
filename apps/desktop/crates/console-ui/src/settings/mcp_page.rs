use crate::input::ComposerInput;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use console_core::types::mcp::{
    McpAuthType, McpConnectionStatus, McpServerConfig, McpTransportType,
};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum McpModalMode {
    Add,
    Edit(String),
}

#[derive(IntoElement)]
pub struct McpPage {
    pub servers: Vec<McpServerConfig>,
    pub is_modal_open: bool,
    pub modal_mode: McpModalMode,
    pub selected_transport: McpTransportType,
    pub selected_auth: McpAuthType,
    pub name_input: Option<Entity<ComposerInput>>,
    pub url_input: Option<Entity<ComposerInput>>,
    pub command_input: Option<Entity<ComposerInput>>,
    pub args_input: Option<Entity<ComposerInput>>,
    pub env_input: Option<Entity<ComposerInput>>,
    pub expanded_server_id: Option<String>,
    pub action_error: Option<String>,
    pub is_submitting: bool,

    // Callbacks
    pub on_open_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_open_edit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_close_modal: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_select_transport: Rc<dyn Fn(McpTransportType, &mut Window, &mut App) + 'static>,
    pub on_select_auth: Rc<dyn Fn(McpAuthType, &mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_delete: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_connect: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_disconnect: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_toggle_expand: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl RenderOnce for McpPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_open_add = self.on_open_add.clone();

        div()
            .flex()
            .flex_col()
            .w_full()
            .min_w_0()
            .gap(px(16.0))
            // Header
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_between()
                    .gap(px(12.0))
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .flex_1()
                            .min_w_0()
                            .gap(px(4.0))
                            .child(
                                div()
                                    .text_size(px(16.0))
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .text_color(theme.text)
                                    .child("Model Context Protocol (MCP)"),
                            )
                            .child(
                                div()
                                    .text_size(px(12.5))
                                    .text_color(theme.text_secondary)
                                    .child("Connect external tools and services via local stdio or remote HTTP servers."),
                            ),
                    )
                    .when(!self.is_modal_open, |el| {
                        let on_add = on_open_add.clone();
                        el.child(
                            div()
                                .id("btn-add-mcp-server")
                                .px(px(10.0))
                                .py(px(5.0))
                                .rounded(px(6.0))
                                .bg(theme.accent)
                                .flex()
                                .items_center()
                                .gap(px(6.0))
                                .cursor_pointer()
                                .hover(|s| s.opacity(0.9))
                                .active(|s| s.opacity(0.75))
                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                    cx.stop_propagation();
                                    (on_add)(window, cx);
                                })
                                .child(app_icon(IconName::Plus, 13.0, theme.on_inverse))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.on_inverse)
                                        .child("Add Server"),
                                ),
                        )
                    }),
            )
            // Error banner if any
            .when_some(self.action_error.clone(), |el, err| {
                el.child(
                    div()
                        .px(px(12.0))
                        .py(px(8.0))
                        .rounded(px(6.0))
                        .bg(theme.danger.opacity(0.12))
                        .border_1()
                        .border_color(theme.danger)
                        .text_size(px(12.0))
                        .text_color(theme.danger)
                        .min_w_0()
                        .child(err),
                )
            })
            // Add/Edit Modal / Inline Form
            .when(self.is_modal_open, |el| el.child(self.render_modal(cx)))
            // Main Content: Empty State or Server Cards
            .child(if self.servers.is_empty() {
                self.render_empty_state(cx).into_any_element()
            } else {
                self.render_server_list(cx).into_any_element()
            })
    }
}

impl McpPage {
    fn render_empty_state(&self, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_add = self.on_open_add.clone();

        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .py(px(48.0))
            .gap(px(12.0))
            .border_1()
            .border_color(theme.border)
            .rounded(px(8.0))
            .bg(theme.surface)
            .child(app_icon(IconName::Server, 36.0, theme.text_ghost))
            .child(
                div()
                    .text_size(px(14.0))
                    .font_weight(gpui::FontWeight::MEDIUM)
                    .text_color(theme.text)
                    .child("No MCP servers configured"),
            )
            .child(
                div()
                    .max_w(px(380.0))
                    .text_align(gpui::TextAlign::Center)
                    .text_size(px(12.0))
                    .text_color(theme.text_secondary)
                    .child("Add local stdio commands or remote HTTP servers like Atlassian to equip the harness with dynamic tools."),
            )
            .child(
                div()
                    .mt(px(4.0))
                    .px(px(12.0))
                    .py(px(6.0))
                    .rounded(px(6.0))
                    .bg(theme.accent)
                    .text_color(theme.on_inverse)
                    .text_size(px(12.0))
                    .font_weight(gpui::FontWeight::MEDIUM)
                    .cursor_pointer()
                    .hover(|s| s.opacity(0.9))
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        cx.stop_propagation();
                        (on_add)(window, cx);
                    })
                    .child("Add your first MCP server"),
            )
    }

    fn render_server_list(&self, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let expanded_id = self.expanded_server_id.clone();
        let on_open_edit = self.on_open_edit.clone();
        let on_delete = self.on_delete.clone();
        let on_connect = self.on_connect.clone();
        let on_disconnect = self.on_disconnect.clone();
        let on_toggle_expand = self.on_toggle_expand.clone();

        div().flex().flex_col().min_w_0().gap(px(10.0)).children(
            self.servers.iter().map(|server| {
                let id = server.id.clone();
                let is_expanded = expanded_id.as_deref() == Some(&id);
                let on_edit_click = on_open_edit.clone();
                let on_del_click = on_delete.clone();
                let on_conn_click = on_connect.clone();
                let on_disc_click = on_disconnect.clone();
                let on_exp_click = on_toggle_expand.clone();

                let (status_badge, status_color) = match &server.status {
                    McpConnectionStatus::Connected => (
                        format!("Connected ({} tools)", server.tools.len()),
                        theme.accent,
                    ),
                    McpConnectionStatus::Connecting => ("Connecting...".to_string(), theme.accent),
                    McpConnectionStatus::NeedsAuth => ("Needs Auth".to_string(), theme.warning),
                    McpConnectionStatus::Disconnected => ("Disconnected".to_string(), theme.text_secondary),
                    McpConnectionStatus::Error(e) => (format!("Error: {}", e), theme.danger),
                };

                let transport_badge = match server.transport {
                    McpTransportType::Stdio => "stdio",
                    McpTransportType::Http => "http",
                };

                let target_info = match server.transport {
                    McpTransportType::Stdio => {
                        let cmd = server.command.clone().unwrap_or_default();
                        let args = server.args.join(" ");
                        if args.is_empty() { cmd } else { format!("{} {}", cmd, args) }
                    }
                    McpTransportType::Http => server.url.clone().unwrap_or_default(),
                };

                let id_clone = id.clone();
                let id_clone2 = id.clone();
                let id_clone3 = id.clone();
                let id_clone4 = id.clone();

                div()
                    .id(ElementId::from(format!("mcp-card-{}", id)))
                    .p(px(14.0))
                    .rounded(px(8.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .flex()
                    .flex_col()
                    .min_w_0()
                    .gap(px(8.0))
                    // Header line
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .justify_between()
                            .gap(px(12.0))
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .flex_1()
                                    .min_w_0()
                                    .gap(px(8.0))
                                    .child(
                                        div()
                                            .min_w_0()
                                            .truncate()
                                            .text_size(px(14.0))
                                            .font_weight(gpui::FontWeight::SEMIBOLD)
                                            .text_color(theme.text)
                                            .child(server.name.clone()),
                                    )
                                    .child(
                                        div()
                                            .flex_shrink_0()
                                            .px(px(6.0))
                                            .py(px(1.5))
                                            .rounded(px(4.0))
                                            .bg(theme.overlay)
                                            .text_size(px(10.5))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.text_secondary)
                                            .child(transport_badge),
                                    )
                                    .child(
                                        div()
                                            .px(px(6.0))
                                            .py(px(1.5))
                                            .rounded(px(4.0))
                                            .min_w_0()
                                            .truncate()
                                            .bg(status_color.opacity(0.15))
                                            .text_size(px(10.5))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(status_color)
                                            .child(status_badge),
                                    ),
                            )
                            // Actions
                            .child(
                                div()
                                    .flex()
                                    .flex_shrink_0()
                                    .items_center()
                                    .gap(px(6.0))
                                    // Connect / Reconnect / Authorize Button
                                    .child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(4.0))
                                            .bg(theme.overlay)
                                            .text_size(px(11.5))
                                            .text_color(theme.text)
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay_strong))
                                            .on_mouse_down(
                                                MouseButton::Left,
                                                move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_conn_click)(id_clone.clone(), window, cx);
                                                },
                                            )
                                            .child(match server.status {
                                                McpConnectionStatus::NeedsAuth => "Authorize",
                                                McpConnectionStatus::Connected => "Reconnect",
                                                _ => "Connect",
                                            }),
                                    )
                                    // Disconnect Button (if connected)
                                    .when(server.status == McpConnectionStatus::Connected, |el| {
                                        el.child(
                                            div()
                                                .px(px(8.0))
                                                .py(px(4.0))
                                                .rounded(px(4.0))
                                                .bg(theme.overlay)
                                                .text_size(px(11.5))
                                                .text_color(theme.text_secondary)
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay_strong))
                                                .on_mouse_down(
                                                    MouseButton::Left,
                                                    move |_event, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_disc_click)(id_clone2.clone(), window, cx);
                                                    },
                                                )
                                                .child("Disconnect"),
                                        )
                                    })
                                    // Edit Button
                                    .child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(4.0))
                                            .bg(theme.overlay)
                                            .text_size(px(11.5))
                                            .text_color(theme.text)
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay_strong))
                                            .on_mouse_down(
                                                MouseButton::Left,
                                                move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_edit_click)(id_clone3.clone(), window, cx);
                                                },
                                            )
                                            .child("Edit"),
                                    )
                                    // Delete Button
                                    .child(
                                        div()
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(4.0))
                                            .bg(theme.danger.opacity(0.12))
                                            .text_size(px(11.5))
                                            .text_color(theme.danger)
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.danger.opacity(0.2)))
                                            .on_mouse_down(
                                                MouseButton::Left,
                                                move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_del_click)(id_clone4.clone(), window, cx);
                                                },
                                            )
                                            .child("Delete"),
                                    ),
                            ),
                    )
                    // Target command / URL
                    .child(
                        div()
                            .min_w_0()
                            .truncate()
                            .text_size(px(12.0))
                            .font_family(".AppleSystemUIFontMonospaced")
                            .text_color(theme.text_secondary)
                            .child(target_info),
                    )
                    // Toggle Tools Accordion (if server has tools)
                    .when(!server.tools.is_empty(), |el| {
                        let id_clone = id.clone();
                        el.child(
                            div()
                                .flex()
                                .flex_col()
                                .gap(px(6.0))
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .gap(px(4.0))
                                        .cursor_pointer()
                                        .on_mouse_down(
                                            MouseButton::Left,
                                            move |_event, window, cx| {
                                                cx.stop_propagation();
                                                (on_exp_click)(id_clone.clone(), window, cx);
                                            },
                                        )
                                        .child(app_icon(
                                            if is_expanded {
                                                IconName::ChevronDown
                                            } else {
                                                IconName::ChevronRight
                                            },
                                            12.0,
                                            theme.text_secondary,
                                        ))
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text_secondary)
                                                .child(format!(
                                                    "{} Advertised Tools",
                                                    server.tools.len()
                                                )),
                                        ),
                                )
                                .when(is_expanded, |el| {
                                    el.child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .min_w_0()
                                            .gap(px(4.0))
                                            .p(px(8.0))
                                            .rounded(px(6.0))
                                            .bg(theme.canvas)
                                            .children(server.tools.iter().map(|t| {
                                                div()
                                                    .flex()
                                                    .flex_col()
                                                    .min_w_0()
                                                    .gap(px(2.0))
                                                    .child(
                                                        div()
                                                            .min_w_0()
                                                            .truncate()
                                                            .text_size(px(11.5))
                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                            .font_family(".AppleSystemUIFontMonospaced")
                                                            .text_color(theme.text)
                                                            .child(t.name.clone()),
                                                    )
                                                    .when_some(t.description.clone(), |el, desc| {
                                                        el.child(
                                                            div()
                                                                .min_w_0()
                                                                .truncate()
                                                                .text_size(px(11.0))
                                                                .text_color(theme.text_secondary)
                                                                .child(desc),
                                                        )
                                                    })
                                            })),
                                    )
                                }),
                        )
                    })
            }),
        )
    }

    fn render_modal(&self, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close_modal.clone();
        let on_save = self.on_save.clone();
        let on_select_trans = self.on_select_transport.clone();
        let on_select_auth = self.on_select_auth.clone();

        let is_edit = match self.modal_mode {
            McpModalMode::Add => false,
            McpModalMode::Edit(_) => true,
        };

        div()
            .p(px(14.0))
            .rounded(px(8.0))
            .border_1()
            .border_color(theme.accent)
            .bg(theme.surface)
            .flex()
            .flex_col()
            .gap(px(12.0))
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
                            .cursor_pointer()
                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                cx.stop_propagation();
                                (on_close)(window, cx);
                            })
                            .child(app_icon(IconName::Close, 14.0, theme.text_secondary)),
                    ),
            )
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
                                                .bg(if is_sel { theme.accent } else { theme.overlay })
                                                .text_color(if is_sel { theme.on_inverse } else { theme.text })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_sel)(McpAuthType::Oauth2, window, cx);
                                                })
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
                                                .bg(if is_sel { theme.accent } else { theme.overlay })
                                                .text_color(if is_sel { theme.on_inverse } else { theme.text })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_sel)(McpAuthType::Static, window, cx);
                                                })
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
                                                .bg(if is_sel { theme.accent } else { theme.overlay })
                                                .text_color(if is_sel { theme.on_inverse } else { theme.text })
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_sel)(McpAuthType::None, window, cx);
                                                })
                                                .child("None")
                                        }),
                                ),
                        ),
                )
            })
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
                            .child(if self.is_submitting { "Saving..." } else { "Save Server" })
                    }),
            )
    }
}
