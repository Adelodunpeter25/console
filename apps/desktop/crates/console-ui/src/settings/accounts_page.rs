use std::collections::HashSet;
use std::rc::Rc;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use console_core::types::{AuthStatusResponse, ProviderCatalogEntry};
use crate::input::ComposerInput;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct AccountsPage {
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub auth_status: Option<AuthStatusResponse>,
    pub logging_in: HashSet<String>,
    pub gemini_project_input: Option<Entity<ComposerInput>>,
    pub on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_save_gemini_project_id: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for AccountsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_login = self.on_login.clone();
        let on_save_project = self.on_save_gemini_project_id.clone();
        let auth_status = self.auth_status.clone();
        let logging_in = self.logging_in.clone();
        let gemini_input = self.gemini_project_input.clone();

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
                            .child("Manage provider credentials and authentication for LLM agents."),
                    ),
            )
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(10.0))
                    .children(self.providers.iter().filter(|p| p.auth_method != "none").map(|provider| {
                        let provider_id = provider.name.clone();
                        let is_logging_in = logging_in.contains(&provider_id);
                        let prov_status = auth_status.as_ref().and_then(|st| {
                            match provider_id.as_str() {
                                "gemini" => Some(&st.gemini),
                                "antigravity" => Some(&st.antigravity),
                                "codebuff" => st.codebuff.as_ref(),
                                "codex" => Some(&st.codex),
                                _ => None,
                            }
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
                        let on_save_click = on_save_project.clone();
                        let gem_inp = gemini_input.clone();

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
                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                            .text_color(theme.text)
                                                            .child(provider.display_name.clone()),
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
                                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                cx.stop_propagation();
                                                (on_login_click)(pid.clone(), window, cx);
                                            })
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
                            .when(provider_id == "gemini" && is_logged_in, move |row| {
                                row.child(
                                    div()
                                        .pt(px(8.0))
                                        .border_t_1()
                                        .border_color(theme.border)
                                        .flex()
                                        .flex_col()
                                        .gap(px(6.0))
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text_secondary)
                                                .child("Google Cloud Project ID (Optional)"),
                                        )
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(8.0))
                                                .when_some(gem_inp, |d, input| {
                                                    d.child(
                                                        div()
                                                            .flex_1()
                                                            .p(px(6.0))
                                                            .rounded(px(6.0))
                                                            .border_1()
                                                            .border_color(theme.border)
                                                            .bg(theme.canvas)
                                                            .child(input),
                                                    )
                                                })
                                                .child(
                                                    div()
                                                        .id("btn-save-gemini-project")
                                                        .px(px(10.0))
                                                        .py(px(5.0))
                                                        .rounded(px(6.0))
                                                        .bg(theme.accent)
                                                        .cursor_pointer()
                                                        .hover(|s| s.opacity(0.9))
                                                        .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                            cx.stop_propagation();
                                                            (on_save_click)(window, cx);
                                                        })
                                                        .child(
                                                            div()
                                                                .text_size(px(11.5))
                                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                                .text_color(theme.on_inverse)
                                                                .child("Save"),
                                                        ),
                                                ),
                                        ),
                                )
                            })
                    })),
            )
    }
}
