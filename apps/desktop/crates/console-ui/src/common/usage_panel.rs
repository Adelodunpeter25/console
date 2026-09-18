//! The usage panel: a dropdown showing detailed quota limits for the active
//! provider. Displays progress bars for each limit, reset times, and status colors.

use console_core::{UsageLimit, UsageReport, UsageUnit};
use gpui::{
    App, Div, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
    relative,
};

use crate::theme::Theme;

#[derive(Clone, IntoElement)]
pub struct UsagePanel {
    provider: String,
    usage_report: Option<UsageReport>,
    is_loading: bool,
}

impl UsagePanel {
    pub fn new(
        provider: String,
        usage_report: Option<UsageReport>,
        is_loading: bool,
    ) -> Self {
        Self {
            provider,
            usage_report,
            is_loading,
        }
    }
}

impl RenderOnce for UsagePanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let mut panel = div()
            .w(px(320.0))
            .p(px(14.0))
            .rounded(px(10.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.raised)
            .shadow_lg()
            .flex()
            .flex_col()
            .gap(px(12.0))
            .text_size(px(12.5));

        // Header
        panel = panel.child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .child(
                    div()
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child(format!("{} Quota Limits", capitalize_provider(&self.provider))),
                )
                .child(
                    div()
                        .text_size(px(11.0))
                        .text_color(theme.text_tertiary)
                        .child("Rate Limits"),
                ),
        );

        if self.is_loading && self.usage_report.is_none() {
            panel = panel.child(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("Loading usage data…"),
            );
            return panel.into_any_element();
        }

        let Some(report) = &self.usage_report else {
            panel = panel.child(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("No quota limits reported for this provider."),
            );
            return panel.into_any_element();
        };

        if report.limits.is_empty() {
            panel = panel.child(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("No active rate limit windows."),
            );
            return panel.into_any_element();
        }

        for limit in &report.limits {
            panel = panel.child(render_limit_row(limit, &theme));
        }

        panel.into_any_element()
    }
}

fn render_limit_row(limit: &UsageLimit, theme: &Theme) -> impl IntoElement {
    let percent = resolve_used_percent(limit);

    let status_color = if percent >= 95.0 || matches!(limit.status, Some(console_core::UsageStatus::Exhausted)) {
        theme.danger
    } else if percent >= 80.0 || matches!(limit.status, Some(console_core::UsageStatus::Warning)) {
        theme.warning
    } else {
        theme.gauge
    };

    let reset_label = limit
        .window
        .as_ref()
        .and_then(|w| w.reset_label.as_ref())
        .cloned();

    let value_label = format_usage_value(limit, percent);

    div()
        .flex()
        .flex_col()
        .gap(px(7.0))
        .child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .gap(px(8.0))
                .child(
                    div()
                        .flex_1()
                        .min_w(px(0.0))
                        .truncate()
                        .text_color(theme.text)
                        .child(SharedString::from(limit.label.clone())),
                )
                .children(reset_label.map(|label| {
                    div()
                        .flex_none()
                        .text_size(px(11.5))
                        .text_color(theme.text_tertiary)
                        .child(SharedString::from(label))
                }))
                .child(
                    div()
                        .flex_none()
                        .text_size(px(12.0))
                        .text_color(theme.text_secondary)
                        .child(SharedString::from(value_label)),
                ),
        )
        .child(meter_bar(theme, percent, status_color))
}

fn meter_bar(theme: &Theme, percent: f64, fill_color: gpui::Hsla) -> Div {
    let fraction = (percent / 100.0).clamp(0.0, 1.0) as f32;
    let fraction = if fraction > 0.0 {
        fraction.max(0.015)
    } else {
        0.0
    };

    div()
        .h(px(3.0))
        .w_full()
        .flex_none()
        .rounded_full()
        .bg(theme.overlay_strong)
        .child(
            div()
                .h_full()
                .w(relative(fraction))
                .rounded_full()
                .bg(fill_color),
        )
}

fn resolve_used_percent(limit: &UsageLimit) -> f64 {
    let amount = &limit.amount;
    if let Some(fraction) = amount.used_fraction {
        return (fraction * 100.0).clamp(0.0, 100.0);
    }
    if let (Some(used), Some(limit_val)) = (amount.used, amount.limit) {
        if limit_val > 0.0 {
            return (used * 100.0 / limit_val).clamp(0.0, 100.0);
        }
    }
    if amount.unit == UsageUnit::Percent {
        if let Some(used) = amount.used {
            return used.clamp(0.0, 100.0);
        }
    }
    if let Some(remaining_frac) = amount.remaining_fraction {
        return ((1.0 - remaining_frac) * 100.0).clamp(0.0, 100.0);
    }
    0.0
}

fn format_usage_value(limit: &UsageLimit, percent: f64) -> String {
    let amount = &limit.amount;
    match (amount.used, amount.limit) {
        (Some(used), Some(limit_val)) if amount.unit != UsageUnit::Percent => {
            format!("{}/{}", format_amount(used, &amount.unit), format_amount(limit_val, &amount.unit))
        }
        (Some(used), None) if amount.unit != UsageUnit::Percent => {
            format!("{} used", format_amount(used, &amount.unit))
        }
        _ => format!("{:.0}%", percent),
    }
}

fn format_amount(value: f64, unit: &UsageUnit) -> String {
    match unit {
        UsageUnit::Minutes => {
            let hours = value / 60.0;
            if hours >= 1.0 {
                format!("{:.1}h", hours)
            } else {
                format!("{:.0}m", value)
            }
        }
        UsageUnit::Tokens => {
            if value >= 1_000_000.0 {
                format!("{:.1}M", value / 1_000_000.0)
            } else if value >= 1_000.0 {
                format!("{:.1}K", value / 1_000.0)
            } else {
                format!("{:.0}", value)
            }
        }
        UsageUnit::Percent => format!("{:.0}%", value),
        UsageUnit::Requests => format!("{:.0}", value),
        UsageUnit::Usd => format!("${:.2}", value),
        _ => format!("{:.0}", value),
    }
}

fn capitalize_provider(provider: &str) -> String {
    match provider.to_ascii_lowercase().as_str() {
        "claude" => "Claude".to_string(),
        "antigravity" => "Antigravity".to_string(),
        "codex" => "OpenAI Codex".to_string(),
        "openai" => "OpenAI".to_string(),
        "opencode" => "OpenCode".to_string(),
        "deepseek" => "DeepSeek".to_string(),
        "grok" => "Grok".to_string(),
        other => {
            let mut chars = other.chars();
            match chars.next() {
                None => String::new(),
                Some(first) => first.to_uppercase().collect::<String>() + chars.as_str(),
            }
        }
    }
}
