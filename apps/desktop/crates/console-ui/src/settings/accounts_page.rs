use crate::theme::Theme;
use console_core::types::{AuthStatusResponse, ProviderCatalogEntry};
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::collections::HashSet;
use std::rc::Rc;

#[derive(IntoElement)]
pub struct AccountsPage {
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub auth_status: Option<AuthStatusResponse>,
    pub logging_in: HashSet<String>,
    pub on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl RenderOnce for AccountsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_login = self.on_login.clone();
        let auth_status = self.auth_status.clone();
        let logging_in = self.logging_in.clone();

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("AI Accounts & Providers"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child(
                                "Manage provider credentials and authentication for LLM agents.",
                            ),
                    ),
            )
            .child(
                div().flex().flex_col().gap(px(10.0)).children(
                    self.providers
                        .iter()
                        .filter(|p| p.auth_method != "none")
                        .map(|provider| {
                            let provider_id = provider.name.clone();
                            let is_logging_in = logging_in.contains(&provider_id);
                            let prov_status = auth_status.as_ref().and_then(|st| match provider_id
                                .as_str()
                            {
                                "antigravity" => Some(&st.antigravity),
                                "codex" | "openai" => Some(&st.codex),
                                _ => None,
                            });

                            let is_logged_in = prov_status.map_or(false, |s| s.logged_in);
                            let user_email = prov_status.and_then(|s| s.email.clone());

                            let status_text = if is_logging_in {
                                "Authenticating...".to_string()
                            } else if is_logged_in {
                                if let Some(email) = user_email {
                                    format!("Connected as {email}")
                                } else {
                                    "Connected".to_string()
                                }
                            } else {
                                "Not connected".to_string()
                            };

                            let status_color = if is_logging_in {
                                theme.accent
                            } else if is_logged_in {
                                theme.accent
                            } else {
                                theme.text_ghost
                            };

                            let pid = provider_id.clone();
                            let on_login_click = on_login.clone();

                            div()
                                .id(ElementId::from(format!("provider-row-{pid}")))
                                .p(px(12.0))
                                .rounded(px(8.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.surface)
                                .flex()
                                .flex_col()
                                .gap(px(10.0))
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .justify_between()
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(10.0))
                                                .child(
                                                    div()
                                                        .w(px(8.0))
                                                        .h(px(8.0))
                                                        .rounded_full()
                                                        .bg(status_color),
                                                )
                                                .child(
                                                    div()
                                                        .flex()
                                                        .flex_col()
                                                        .gap(px(2.0))
                                                        .child(
                                                            div()
                                                                .text_size(px(13.5))
                                                                .font_weight(
                                                                    gpui::FontWeight::MEDIUM,
                                                                )
                                                                .text_color(theme.text)
                                                                .child(
                                                                    provider.display_name.clone(),
                                                                ),
                                                        )
                                                        .child(
                                                            div()
                                                                .text_size(px(11.5))
                                                                .text_color(theme.text_tertiary)
                                                                .child(status_text),
                                                        ),
                                                ),
                                        )
                                        .child(
                                            div()
                                                .id(ElementId::from(format!("btn-login-{pid}")))
                                                .px(px(10.0))
                                                .py(px(5.0))
                                                .rounded(px(6.0))
                                                .border_1()
                                                .border_color(theme.border_strong)
                                                .bg(theme.raised)
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .active(|s| s.bg(theme.overlay_strong))
                                                .on_mouse_down(
                                                    MouseButton::Left,
                                                    move |_event, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_login_click)(pid.clone(), window, cx);
                                                    },
                                                )
                                                .child(
                                                    div()
                                                        .text_size(px(12.0))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.text)
                                                        .child(if is_logging_in {
                                                            "Authenticating..."
                                                        } else if is_logged_in {
                                                            "Re-login"
                                                        } else {
                                                            "Login"
                                                        }),
                                                ),
                                        ),
                                )
                        }),
                ),
            )
    }
}
