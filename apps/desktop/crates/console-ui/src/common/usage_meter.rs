//! The usage meter: a circular progress gauge in the workspace footer
//! showing the active provider's rate-limit occupancy. Clicking opens
//! a popover with detailed limit breakdowns.

use console_core::{UsageLimit, UsageReport, UsageUnit};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, Hsla, InteractiveElement, IntoElement, ParentElement, PathBuilder, RenderOnce, Styled,
    Window, canvas, div, point, px,
};

use crate::common::UsagePanel;
use crate::primitives::{ContextMenuHandle, MenuAlign, popover};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct UsageMeter {
    provider: String,
    usage_report: Option<UsageReport>,
    menu_handle: ContextMenuHandle,
    is_loading: bool,
}

impl UsageMeter {
    pub fn new(
        provider: String,
        usage_report: Option<UsageReport>,
        menu_handle: ContextMenuHandle,
        is_loading: bool,
    ) -> Self {
        Self {
            provider,
            usage_report,
            menu_handle,
            is_loading,
        }
    }
}

impl RenderOnce for UsageMeter {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // Find the primary limit's percent used
        let percent = self
            .usage_report
            .as_ref()
            .and_then(|report| report.limits.first())
            .map(resolve_used_percent);

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
            .child(circular_progress_gauge(percent, theme.border_strong, fill_color));

        let provider = self.provider;
        let usage_report = self.usage_report;
        let is_loading = self.is_loading;

        popover(
            trigger,
            &self.menu_handle,
            MenuAlign::AboveRight,
            move |_handle, _window, _cx| {
                UsagePanel::new(
                    provider.clone(),
                    usage_report.clone(),
                    is_loading,
                )
                .into_any_element()
            },
        )
    }
}

/// The circular progress gauge glyph: a 13px ring whose arc fills clockwise
/// from 12 o'clock, over a faint track ring.
fn circular_progress_gauge(percent: Option<f64>, track: Hsla, fill: Hsla) -> impl IntoElement {
    const SIZE: f32 = 13.0;
    const STROKE: f32 = 2.5;

    canvas(
        |_, _, _| (),
        move |bounds, _, window, _| {
            let center = bounds.center();
            let radius = px((SIZE - STROKE) / 2.0);

            // A full circle is two 180° arcs
            let full_circle = |builder: &mut PathBuilder| {
                builder.move_to(point(center.x + radius, center.y));
                builder.arc_to(
                    point(radius, radius),
                    px(0.0),
                    false,
                    true,
                    point(center.x - radius, center.y),
                );
                builder.arc_to(
                    point(radius, radius),
                    px(0.0),
                    false,
                    true,
                    point(center.x + radius, center.y),
                );
                builder.close();
            };

            let mut track_builder = PathBuilder::stroke(px(STROKE));
            full_circle(&mut track_builder);
            if let Ok(path) = track_builder.build() {
                window.paint_path(path, track);
            }

            let Some(percent) = percent else {
                return;
            };

            // Keep a visible sliver for a nearly-empty context
            let fraction = ((percent / 100.0) as f32).clamp(0.0, 1.0).max(0.05);
            let mut arc_builder = PathBuilder::stroke(px(STROKE));
            if fraction >= 0.999 {
                full_circle(&mut arc_builder);
            } else {
                let start = -std::f32::consts::FRAC_PI_2;
                let angle = start + fraction * std::f32::consts::TAU;
                arc_builder.move_to(point(center.x, center.y - radius));
                arc_builder.arc_to(
                    point(radius, radius),
                    px(0.0),
                    fraction > 0.5,
                    true,
                    point(
                        center.x + radius * angle.cos(),
                        center.y + radius * angle.sin(),
                    ),
                );
            }
            if let Ok(path) = arc_builder.build() {
                window.paint_path(path, fill);
            }
        },
    )
    .w(px(SIZE))
    .h(px(SIZE))
    .flex_none()
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
