//! The usage panel: a dropdown showing detailed quota limits for the active
//! provider. Displays progress bars for each limit, reset times, and status colors.

use console_core::{ContextSnapshot, UsageLimit, UsageLimitExt, UsageReport};
use gpui::{
    App, Div, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
    relative,
};

use crate::theme::Theme;

#[derive(Clone, IntoElement)]
pub struct UsagePanel {
    usage_report: Option<UsageReport>,
    context_snapshot: Option<ContextSnapshot>,
    is_loading: bool,
}

impl UsagePanel {
    pub fn new(
        usage_report: Option<UsageReport>,
        context_snapshot: Option<ContextSnapshot>,
        is_loading: bool,
    ) -> Self {
        Self {
            usage_report,
            context_snapshot,
            is_loading,
        }
    }

    /// Quota section body: loading / empty states or one row per limit.
    fn quota_body(&self, theme: &Theme) -> Option<impl IntoElement> {
        if self.is_loading && self.usage_report.is_none() {
            return Some(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("Loading usage data…")
                    .into_any_element(),
            );
        }

        let Some(report) = &self.usage_report else {
            return Some(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("No quota limits reported for this provider.")
                    .into_any_element(),
            );
        };

        if report.limits.is_empty() {
            return Some(
                div()
                    .py(px(12.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("No active rate limit windows.")
                    .into_any_element(),
            );
        }

        let theme = *theme;
        let rows = report
            .limits
            .iter()
            .map(move |limit| render_limit_row(limit, &theme).into_any_element())
            .collect::<Vec<_>>();
        Some(
            div()
                .flex()
                .flex_col()
                .gap(px(12.0))
                .children(rows)
                .into_any_element(),
        )
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

        // Section 1: context occupancy.
        panel = panel.child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .child(
                    div()
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child("Context"),
                )
                .children(self.context_snapshot.as_ref().map(|snapshot| {
                    div()
                        .flex_none()
                        .text_size(px(12.0))
                        .text_color(theme.text_secondary)
                        .child(SharedString::from(snapshot.used_vs_window()))
                })),
        );

        if let Some(snapshot) = &self.context_snapshot {
            panel = panel.child(render_context_bar(snapshot, &theme));
        } else {
            panel = panel.child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("Context usage unavailable."),
            );
        }

        // Divider, then section 2: provider quota limits.
        panel = panel.child(
            div()
                .flex()
                .flex_col()
                .gap(px(10.0))
                .pt(px(8.0))
                .border_t_1()
                .border_color(theme.border)
                .child(
                    div()
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child("Usage Limits"),
                )
                .children(self.quota_body(&theme)),
        );

        panel.into_any_element()
    }
}

fn render_context_bar(snapshot: &ContextSnapshot, theme: &Theme) -> impl IntoElement {
    let percent = snapshot.percent_used.clamp(0.0, 100.0);
    let threshold = (snapshot.threshold_ratio * 100.0).clamp(0.0, 100.0);

    let status_color = if percent >= threshold {
        theme.danger
    } else if percent >= 50.0 {
        theme.warning
    } else {
        theme.gauge
    };

    meter_bar(theme, percent, status_color)
}

fn render_limit_row(limit: &UsageLimit, theme: &Theme) -> impl IntoElement {
    let percent = resolve_used_percent(limit);

    let status_color = if percent >= 95.0 || limit.status.as_deref() == Some("exhausted") {
        theme.danger
    } else if percent >= 80.0 || limit.status.as_deref() == Some("warning") {
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
    let Some(amount) = limit.amount.as_ref() else {
        return 0.0;
    };
    if let Some(fraction) = amount.used_fraction {
        return (fraction * 100.0).clamp(0.0, 100.0);
    }
    if let (Some(used), Some(limit_val)) = (amount.used, amount.limit) {
        if limit_val > 0.0 {
            return (used * 100.0 / limit_val).clamp(0.0, 100.0);
        }
    }
    if amount.unit == "percent" {
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
    let Some(amount) = limit.amount.as_ref() else {
        return format!("{:.0}%", percent);
    };
    match (amount.used, amount.limit) {
        (Some(used), Some(limit_val)) if amount.unit != "percent" => {
            format!(
                "{}/{}",
                format_amount(used, &amount.unit),
                format_amount(limit_val, &amount.unit)
            )
        }
        (Some(used), None) if amount.unit != "percent" => {
            format!("{} used", format_amount(used, &amount.unit))
        }
        _ => format!("{:.0}%", percent),
    }
}

fn format_amount(value: f64, unit: &str) -> String {
    match unit {
        "minutes" => {
            let hours = value / 60.0;
            if hours >= 1.0 {
                format!("{:.1}h", hours)
            } else {
                format!("{:.0}m", value)
            }
        }
        "tokens" => {
            if value >= 1_000_000.0 {
                format!("{:.1}M", value / 1_000_000.0)
            } else if value >= 1_000.0 {
                format!("{:.1}K", value / 1_000.0)
            } else {
                format!("{:.0}", value)
            }
        }
        "percent" => format!("{:.0}%", value),
        "requests" => format!("{:.0}", value),
        "usd" => format!("${:.2}", value),
        _ => format!("{:.0}", value),
    }
}
