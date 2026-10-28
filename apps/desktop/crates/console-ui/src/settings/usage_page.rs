use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use console_core::types::{
    AuthStatusResponse, ProviderCatalogEntry, ProviderCredentialStatus, UsageLimit, UsageReport,
    auth_status_for,
};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::collections::HashMap;
use std::rc::Rc;

#[derive(IntoElement)]
pub struct UsagePage {
    pub reports: Option<Rc<HashMap<String, Option<UsageReport>>>>,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub auth_status: Option<AuthStatusResponse>,
    /// Pasted provider API keys, keyed by provider id. Only "is there a key"
    /// is read here — the key itself never reaches a client.
    pub credentials: Rc<HashMap<String, ProviderCredentialStatus>>,
    pub loading: bool,
    pub on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

// The server precomputes reset labels ("resets in 3h 20m") in
// ToProtoReport; this local fallback only covers hand-made reports in
// tests/previews that carry a raw resets_at.

impl RenderOnce for UsagePage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_refresh = self.on_refresh.clone();
        let on_login = self.on_login.clone();
        let auth_status = self.auth_status.clone();
        let is_loading = self.loading;

        let reports_map = self.reports.clone();

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_between()
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
                                    .child("AI Quota & Usage"),
                            )
                            .child(
                                div()
                                    .text_size(px(12.5))
                                    .text_color(theme.text_secondary)
                                    .child("Live quota tracking and rate limits across AI providers."),
                            ),
                    )
                    .child(
                        div()
                            .id("btn-refresh-usage")
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
                                (on_refresh)(window, cx);
                            })
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(
                                IconName::Refresh,
                                13.0,
                                if is_loading { theme.accent } else { theme.text_secondary },
                            ))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .text_color(theme.text)
                                    .child(if is_loading { "Refreshing..." } else { "Refresh" }),
                            ),
                    ),
            )
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(12.0))
                    .children(self.providers.iter().filter(|p| {
                        matches!(p.name.as_str(), "antigravity" | "codex" | "claude" | "opencode")
                    }).map(|provider| {
                        let provider_id = provider.name.clone();
                        let prov_status = auth_status
                            .as_ref()
                            .and_then(|st| auth_status_for(st, &provider_id));

                        // A provider whose credential is a pasted key has no
                        // OAuth session, so "signed in" means "a key is
                        // configured" — the only thing the server reports.
                        let uses_api_key = provider.auth_method == "api-key";
                        let key_configured = self
                            .credentials
                            .get(&provider_id)
                            .and_then(|c| c.configured)
                            .unwrap_or(false);

                        let is_logged_in = if uses_api_key {
                            key_configured
                        } else {
                            prov_status.map_or(false, |s| s.logged_in)
                        };
                        let user_email = prov_status.and_then(|s| s.email.clone());

                        let report_opt = reports_map.as_ref().and_then(|map| {
                            map.get(&provider_id).cloned().flatten()
                        });

                        let mut sorted_limits: Vec<UsageLimit> = report_opt
                            .as_ref()
                            .map(|r| r.limits.clone())
                            .unwrap_or_default();

                        // Sort most pressured first (lowest remaining fraction).
                        // The server derives used_percent; remaining is its complement.
                        sorted_limits.sort_by(|a, b| {
                            let a_rem = a.amount.as_ref().and_then(|amt| amt.used_percent).map_or(100.0, |p| 100.0 - p);
                            let b_rem = b.amount.as_ref().and_then(|amt| amt.used_percent).map_or(100.0, |p| 100.0 - p);
                            a_rem.partial_cmp(&b_rem).unwrap_or(std::cmp::Ordering::Equal)
                        });

                        let pid = provider_id.clone();
                        let on_login_click = on_login.clone();

                        div()
                            .id(ElementId::from(format!("usage-provider-card-{pid}")))
                            .p(px(14.0))
                            .rounded(px(8.0))
                            .border_1()
                            .border_color(theme.border)
                            .bg(theme.surface)
                            .flex()
                            .flex_col()
                            .gap(px(12.0))
                            // Provider header row
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
                                                    .bg(if is_logged_in { theme.accent } else { theme.text_ghost }),
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
                                                            .child(if uses_api_key {
                                                                // A pasted key has no account
                                                                // behind it, so there is no
                                                                // email to show.
                                                                if is_logged_in {
                                                                    "Using your API key".to_string()
                                                                } else {
                                                                    "No API key set".to_string()
                                                                }
                                                            } else if is_logged_in {
                                                                if let Some(ref email) = user_email {
                                                                    format!("Connected as {email}")
                                                                } else {
                                                                    "Connected".to_string()
                                                                }
                                                            } else {
                                                                "Not connected".to_string()
                                                            }),
                                                    ),
                                            ),
                                    )
                                    // OAuth login only. A key provider is
                                    // configured under Providers, and a
                                    // "Login" button that silently does
                                    // nothing would be worse than none.
                                    .when(!is_logged_in && !uses_api_key, |header| {
                                        let p = pid.clone();
                                        let cb = on_login_click.clone();
                                        header.child(
                                            div()
                                                .id(ElementId::from(format!("btn-usage-login-{p}")))
                                                .px(px(10.0))
                                                .py(px(4.0))
                                                .rounded(px(6.0))
                                                .border_1()
                                                .border_color(theme.border_strong)
                                                .bg(theme.raised)
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .active(|s| s.bg(theme.overlay_strong))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (cb)(p.clone(), window, cx);
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.text)
                                                        .child("Login"),
                                                ),
                                        )
                                    }),
                            )
                            // Limits list
                            .when(is_logged_in, |card| {
                                if sorted_limits.is_empty() {
                                    card.child(
                                        div()
                                            .pt(px(6.0))
                                            .border_t_1()
                                            .border_color(theme.border)
                                            .text_size(px(12.0))
                                            .text_color(theme.text_tertiary)
                                            .child(if is_loading || reports_map.is_none() {
                                                "Fetching quota details..."
                                            } else {
                                                "No quota limits reported for this account."
                                            }),
                                    )
                                } else {
                                    card.child(
                                        div()
                                            .pt(px(8.0))
                                            .border_t_1()
                                            .border_color(theme.border)
                                            .flex()
                                            .flex_col()
                                            .gap(px(10.0))
                                            .children(sorted_limits.into_iter().map(|limit| {
                                                // The server derives both; render verbatim.
                                                let used_percent = limit
                                                    .amount
                                                    .as_ref()
                                                    .and_then(|amt| amt.used_percent)
                                                    .unwrap_or(0.0);
                                                let used_frac = used_percent / 100.0;
                                                let remaining_percent = (100.0 - used_percent).clamp(0.0, 100.0);

                                                let status_color = match limit.status.as_deref() {
                                                    Some("exhausted") => theme.danger,
                                                    Some("warning") => theme.warning,
                                                    Some("ok") => theme.success,
                                                    _ => theme.success,
                                                };

                                                // Server-computed countdown ("resets in 3h 20m").
                                                let reset_str = limit.window.as_ref().and_then(|w| {
                                                    w.reset_label.clone().or_else(|| {
                                                        if w.label.is_empty() {
                                                            None
                                                        } else {
                                                            Some(w.label.clone())
                                                        }
                                                    })
                                                });

                                                let tier_label = limit
                                                    .scope
                                                    .as_ref()
                                                    .and_then(|scope| scope.tier.clone());

                                                div()
                                                    .flex()
                                                    .flex_col()
                                                    .gap(px(5.0))
                                                    .child(
                                                        div()
                                                            .flex()
                                                            .items_center()
                                                            .justify_between()
                                                            .child(
                                                                div()
                                                                    .flex()
                                                                    .items_center()
                                                                    .gap(px(6.0))
                                                                    .child(
                                                                        div()
                                                                            .text_size(px(12.5))
                                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                                            .text_color(theme.text)
                                                                            .child(limit.label.clone()),
                                                                    )
                                                                    .when_some(tier_label, |row, tier| {
                                                                        row.child(
                                                                            div()
                                                                                .px(px(6.0))
                                                                                .py(px(1.5))
                                                                                .rounded(px(4.0))
                                                                                .bg(theme.raised)
                                                                                .border_1()
                                                                                .border_color(theme.border)
                                                                                .text_size(px(10.0))
                                                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                                                .text_color(theme.text_secondary)
                                                                                .child(tier),
                                                                        )
                                                                    }),
                                                            )
                                                            .child(
                                                                div()
                                                                    .text_size(px(11.5))
                                                                    .text_color(theme.text_secondary)
                                                                    .child(format!("{used_percent:.1}% used ({remaining_percent:.1}% remaining)")),
                                                            ),
                                                    )
                                                    // Progress bar
                                                    .child(
                                                        div()
                                                            .w_full()
                                                            .h(px(6.0))
                                                            .rounded_full()
                                                            .bg(theme.raised)
                                                            .overflow_hidden()
                                                            .child(
                                                                div()
                                                                    .h_full()
                                                                    .w(gpui::relative(used_frac as f32))
                                                                    .rounded_full()
                                                                    .bg(status_color),
                                                            ),
                                                    )
                                                    .when_some(reset_str, |row, reset_text| {
                                                        row.child(
                                                            div()
                                                                .text_size(px(11.0))
                                                                .text_color(theme.text_tertiary)
                                                                .child(reset_text),
                                                        )
                                                    })
                                            })),
                                    )
                                }
                            })
                            // A key provider with no key cannot report quota.
                            // Say where to fix it instead of showing an empty
                            // card.
                            .when(!is_logged_in && uses_api_key, |card| {
                                card.child(
                                    div()
                                        .pt(px(6.0))
                                        .border_t_1()
                                        .border_color(theme.border)
                                        .text_size(px(12.0))
                                        .text_color(theme.text_tertiary)
                                        .child("Add an API key under Providers to see quota."),
                                )
                            })
                    }))
            )
    }
}
