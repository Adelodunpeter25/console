//! The usage panel: a dropdown showing detailed quota limits for the active
//! provider. Displays progress bars for each limit, reset times, and status colors.

use console_core::{UsageLimit, UsageReport};
use gpui::{
    App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px,
};

use crate::theme::Theme;

/// Calculate the fractional usage of a limit (0.0 to 1.0).
fn resolve_used_fraction(limit: &UsageLimit) -> Option<f64> {
    let amount = &limit.amount;
    if let Some(fraction) = amount.used_fraction {
        return Some(fraction);
    }
    if let (Some(used), Some(limit_val)) = (amount.used, amount.limit) {
        if limit_val > 0.0 {
            return Some(used / limit_val);
        }
    }
    if amount.unit == console_core::UsageUnit::Percent {
        if let Some(used) = amount.used {
            return Some(used / 100.0);
        }
    }
    if let Some(remaining_frac) = amount.remaining_fraction {
        return Some((1.0 - remaining_frac).max(0.0));
    }
    None
}

#[derive(IntoElement)]
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

        if self.is_loading {
            return div()
                .p(px(16.0))
                .text_size(px(12.0))
                .text_color(theme.text_tertiary)
                .child("Loading usage data...")
                .into_any_element();
        }

        let Some(report) = self.usage_report else {
            return div()
                .p(px(16.0))
                .text_size(px(12.0))
                .text_color(theme.text_tertiary)
                .child("No usage data available")
                .into_any_element();
        };

        div()
            .p(px(12.0))
            .w(px(320.0))
            .bg(theme.surface)
            .rounded(px(8.0))
            .flex()
            .flex_col()
            .gap(px(12.0))
            .child(
                // Header
                div()
                    .text_size(px(13.0))
                    .font_weight(gpui::FontWeight::SEMIBOLD)
                    .text_color(theme.text)
                    .child(format!("{} Usage Limits", capitalize_provider(&self.provider))),
            )
            .children(report.limits.iter().map(|limit| {
                render_limit_row(&limit, theme)
            }))
            .into_any_element()
    }
}

fn render_limit_row(limit: &UsageLimit, theme: Theme) -> impl IntoElement {
    let status_color = match limit.status {
        Some(console_core::UsageStatus::Ok) => theme.text_secondary,
        Some(console_core::UsageStatus::Warning) => theme.warning,
        Some(console_core::UsageStatus::Exhausted) => theme.danger,
        _ => theme.text_tertiary,
    };

    // Calculate fill percentage
    let fill_percent = resolve_used_fraction(limit)
        .unwrap_or(0.0)
        .clamp(0.0, 1.0);
    let fill_percent = if fill_percent > 0.0 {
        fill_percent.max(0.015)
    } else {
        0.0
    };

    let reset_label = limit
        .window
        .as_ref()
        .and_then(|w| w.reset_label.as_ref())
        .map(|l| l.clone());

    div()
        .flex()
        .flex_col()
        .gap(px(6.0))
        .child(
            // Label and value
            div()
                .flex()
                .justify_between()
                .items_center()
                .child(
                    div()
                        .text_size(px(11.0))
                        .text_color(theme.text)
                        .child(limit.label.clone()),
                )
                .child(
                    div()
                        .text_size(px(10.0))
                        .text_color(theme.text_secondary)
                        .child(format_usage_value(&limit)),
                ),
        )
        .child(
            // Progress bar
            div()
                .h(px(3.0))
                .w_full()
                .rounded_full()
                .bg(theme.overlay_strong)
                .child(
                    div()
                        .h_full()
                        .w(gpui::relative(fill_percent as f32))
                        .rounded_full()
                        .bg(status_color),
                ),
        )
        .child(
            if let Some(label) = reset_label {
                div()
                    .h(px(12.0))
                    .flex()
                    .items_center()
                    .child(
                        div()
                            .text_size(px(10.0))
                            .text_color(theme.text_tertiary)
                            .child(label),
                    )
                    .into_any_element()
            } else {
                div().h_0().into_any_element()
            },
        )
}

fn format_usage_value(limit: &UsageLimit) -> String {
    let amount = &limit.amount;
    match (amount.used, amount.limit) {
        (Some(used), Some(limit)) => {
            format!("{}/{}", format_amount(used, &amount.unit), format_amount(limit, &amount.unit))
        }
        (Some(used), None) => {
            format!("{} used", format_amount(used, &amount.unit))
        }
        (None, Some(limit)) => {
            format!("{} limit", format_amount(limit, &amount.unit))
        }
        _ => "—".to_string(),
    }
}

fn format_amount(value: f64, unit: &console_core::UsageUnit) -> String {
    match unit {
        console_core::UsageUnit::Minutes => {
            let hours = value / 60.0;
            if hours >= 1.0 {
                format!("{:.1}h", hours)
            } else {
                format!("{:.0}m", value)
            }
        }
        console_core::UsageUnit::Tokens => {
            if value >= 1_000_000.0 {
                format!("{:.1}M", value / 1_000_000.0)
            } else if value >= 1_000.0 {
                format!("{:.1}K", value / 1_000.0)
            } else {
                format!("{:.0}", value)
            }
        }
        console_core::UsageUnit::Percent => format!("{:.0}%", value),
        console_core::UsageUnit::Requests => format!("{:.0}", value),
        console_core::UsageUnit::Usd => format!("${:.2}", value),
        _ => format!("{:.0}", value),
    }
}

fn capitalize_provider(provider: &str) -> String {
    match provider {
        "claude" => "Claude".to_string(),
        "antigravity" => "Antigravity".to_string(),
        "codex" => "OpenAI Codex".to_string(),
        "openai" => "OpenAI".to_string(),
        "opencode" => "OpenCode".to_string(),
        other => {
            let mut chars = other.chars();
            match chars.next() {
                None => String::new(),
                Some(first) => first.to_uppercase().collect::<String>() + chars.as_str(),
            }
        }
    }
}
