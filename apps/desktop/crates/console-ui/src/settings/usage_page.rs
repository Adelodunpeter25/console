use std::collections::HashMap;
use std::rc::Rc;
use console_core::types::{AuthStatusResponse, ProviderCatalogEntry, UsageLimit, UsageReport, UsageStatus};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct UsagePage {
    pub reports: Option<Rc<HashMap<String, Option<UsageReport>>>>,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub auth_status: Option<AuthStatusResponse>,
    pub loading: bool,
    pub on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

fn format_resets_at(epoch_ms: i64) -> String {
    let now = chrono::Utc::now().timestamp_millis();
    let diff_secs = (epoch_ms - now) / 1000;
    if diff_secs <= 0 {
        "Resets momentarily".to_string()
    } else if diff_secs < 60 {
        format!("Resets in {diff_secs}s")
    } else if diff_secs < 3600 {
        let mins = diff_secs / 60;
        format!("Resets in {mins}m")
    } else if diff_secs < 86400 {
        let hours = diff_secs / 3600;
        let mins = (diff_secs % 3600) / 60;
        if mins > 0 {
            format!("Resets in {hours}h {mins}m")
        } else {
            format!("Resets in {hours}h")
        }
    } else {
        let days = diff_secs / 86400;
        let hours = (diff_secs % 86400) / 3600;
        if hours > 0 {
            format!("Resets in {days}d {hours}h")
        } else {
            format!("Resets in {days}d")
        }
    }
}

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
                    .children(self.providers.iter().filter(|p| p.auth_method != "none").map(|provider| {
                        let provider_id = provider.name.clone();
                        let prov_status = auth_status.as_ref().and_then(|st| {
                            match provider_id.as_str() {
                                "gemini" => Some(&st.gemini),
                                "antigravity" => Some(&st.antigravity),
                                "codebuff" => st.codebuff.as_ref(),
                                "codex" | "openai" => Some(&st.codex),
                                _ => None,
                            }
                        });

                        let is_logged_in = prov_status.map_or(false, |s| s.logged_in);
                        let user_email = prov_status.and_then(|s| s.email.clone());

                        let report_opt = reports_map.as_ref().and_then(|map| {
                            map.get(&provider_id).cloned().flatten()
                        });

                        let mut sorted_limits: Vec<UsageLimit> = report_opt
                            .as_ref()
                            .map(|r| r.limits.clone())
                            .unwrap_or_default();

                        // Sort most pressured first (lowest remaining fraction)
                        sorted_limits.sort_by(|a, b| {
                            let a_rem = a.resolved_remaining_fraction();
                            let b_rem = b.resolved_remaining_fraction();
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
                                                            .child(if is_logged_in {
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
                                    .when(!is_logged_in, |header| {
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
                                                let used_frac = limit.resolved_used_fraction();
                                                let used_percent = (used_frac * 100.0).clamp(0.0, 100.0);
                                                let remaining_percent = (100.0 - used_percent).clamp(0.0, 100.0);

                                                let status_color = match limit.status {
                                                    Some(UsageStatus::Exhausted) => theme.danger,
                                                    Some(UsageStatus::Warning) => theme.warning,
                                                    _ if used_percent >= 90.0 => theme.danger,
                                                    _ if used_percent >= 75.0 => theme.warning,
                                                    _ => theme.accent,
                                                };

                                                let reset_str = limit.window.as_ref().and_then(|w| {
                                                    if let Some(ts) = w.resets_at {
                                                        Some(format_resets_at(ts))
                                                    } else if let Some(ref l) = w.reset_label {
                                                        Some(l.clone())
                                                    } else if !w.label.is_empty() {
                                                        Some(w.label.clone())
                                                    } else {
                                                        None
                                                    }
                                                });

                                                let tier_label = limit.scope.tier.clone();

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
                    }))
            )
    }
}
