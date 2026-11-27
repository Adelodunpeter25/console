//! The usage meter: a circular progress gauge in the workspace footer
//! showing the active provider's rate-limit occupancy. Clicking opens
//! a popover with detailed limit breakdowns.

use console_core::{ContextSnapshot, UsageLimit, UsageReport};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, Hsla, InteractiveElement, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px,
};

use crate::common::UsagePanel;
use crate::primitives::{ContextMenuHandle, MenuAlign, RingArc, popover, ring_chart};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct UsageMeter {
    usage_report: Option<UsageReport>,
    context_snapshot: Option<ContextSnapshot>,
    menu_handle: ContextMenuHandle,
    is_loading: bool,
}

impl UsageMeter {
    pub fn new(
        usage_report: Option<UsageReport>,
        context_snapshot: Option<ContextSnapshot>,
        menu_handle: ContextMenuHandle,
        is_loading: bool,
    ) -> Self {
        Self {
            usage_report,
            context_snapshot,
            menu_handle,
            is_loading,
        }
    }
}

impl RenderOnce for UsageMeter {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // The ring shows context-window occupancy when known, falling back
        // to the primary quota limit otherwise.
        let percent = self
            .context_snapshot
            .as_ref()
            .map(|snap| snap.percent_used.clamp(0.0, 100.0))
            .or_else(|| {
                self.usage_report
                    .as_ref()
                    .and_then(|report| report.limits.first())
                    .map(resolve_used_percent)
            });

        let fill_color = match percent {
            Some(p) if p >= 95.0 => theme.danger,
            Some(p) if p >= 80.0 => theme.warning,
            Some(_) => theme.gauge,
            None => theme.gauge,
        };

        let is_open = self.menu_handle.is_open();
        let trigger = div()
            .id("usage-meter-button")
            .h(px(22.0))
            .px(px(6.0))
            .rounded(px(5.0))
            .flex()
            .items_center()
            .justify_center()
            .cursor_default()
            .hover(|element| element.bg(theme.overlay))
            .when(is_open, |element| element.bg(theme.overlay_strong))
            .child(circular_progress_gauge(
                percent,
                theme.border_strong,
                fill_color,
            ));

        let usage_report = self.usage_report;
        let context_snapshot = self.context_snapshot;
        let is_loading = self.is_loading;

        popover(
            trigger,
            &self.menu_handle,
            MenuAlign::AboveRight,
            move |_handle, _window, _cx| {
                UsagePanel::new(usage_report.clone(), context_snapshot.clone(), is_loading)
                    .into_any_element()
            },
        )
    }
}

/// The circular progress gauge glyph: a 13px ring whose arc fills clockwise
/// from 12 o'clock, over a faint track ring.
fn circular_progress_gauge(percent: Option<f64>, track: Hsla, fill: Hsla) -> impl IntoElement {
    // Keep a visible sliver for a nearly-empty context
    let arcs = percent
        .map(|percent| {
            let fraction = ((percent / 100.0) as f32).clamp(0.0, 1.0).max(0.05);
            vec![RingArc {
                start: 0.0,
                len: fraction,
                color: fill,
            }]
        })
        .unwrap_or_default();
    ring_chart(13.0, 2.5, Some(track), arcs)
}

fn resolve_used_percent(limit: &UsageLimit) -> f64 {
    let Some(amount) = limit.amount.as_ref() else {
        return 0.0;
    };
    if let Some(fraction) = amount.used_fraction {
        return (fraction * 100.0).clamp(0.0, 100.0);
    }
    if let (Some(used), Some(limit_val)) = (amount.used, amount.limit)
        && limit_val > 0.0
    {
        return (used * 100.0 / limit_val).clamp(0.0, 100.0);
    }
    if amount.unit == "percent"
        && let Some(used) = amount.used
    {
        return used.clamp(0.0, 100.0);
    }
    if let Some(remaining_frac) = amount.remaining_fraction {
        return ((1.0 - remaining_frac) * 100.0).clamp(0.0, 100.0);
    }
    0.0
}
