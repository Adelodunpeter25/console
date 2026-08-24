//! The live "still working" indicator shown below the streaming transcript:
//! three dots chasing a brightness wave plus a ticking elapsed-time label
//! ("Working for 37s").
//!
//! One 1-second pulse drives both: the pulse clock leases the hosting view,
//! so the transcript re-renders at a modest cadence while the indicator is
//! mounted, and the label recomputes its elapsed seconds from `started_at` on
//! every render.

use crate::primitives::motion;
use crate::theme::Theme;
use gpui::{App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

/// Three dots chasing a brightness wave, phase-offset along the shared pulse
/// clock so the bright spot travels left to right.
fn working_wave_dots(color: gpui::Hsla) -> gpui::AnyElement {
    const DOT_PHASE_STEP: f32 = 0.18;
    motion::pulse(std::time::Duration::from_millis(1000), move |phase| {
        div()
            .flex()
            .items_center()
            .gap(px(3.5))
            .children((0..3).map(|index| {
                let dot_phase = (phase + 1.0 - index as f32 * DOT_PHASE_STEP) % 1.0;
                let wave = ((dot_phase * std::f32::consts::TAU).sin() + 1.0) / 2.0;
                div()
                    .size(px(4.5))
                    .flex_none()
                    .rounded_full()
                    .bg(color)
                    .opacity(0.25 + 0.75 * wave)
            }))
            .into_any_element()
    })
    // The transcript subtree is expensive to rebuild; 15 FPS is sufficient
    // for the dots, while the elapsed label only changes once per second.
    .every(2)
    .into_any_element()
}

/// The live working indicator: animated dots plus "Working for Ns".
#[derive(IntoElement)]
pub struct WorkingIndicator {
    /// Unix seconds when the run started streaming.
    started_at: i64,
    theme: Theme,
}

impl WorkingIndicator {
    pub fn new(started_at: i64, theme: Theme) -> Self {
        Self { started_at, theme }
    }
}

impl RenderOnce for WorkingIndicator {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let theme = self.theme;
        let started_at = self.started_at;
        let now = chrono::Utc::now().timestamp();
        let seconds = (now - started_at).max(0) as u64;

        div()
            .flex()
            .items_center()
            .gap(px(7.0))
            .text_size(px(11.5))
            .text_color(theme.text_tertiary)
            .child(working_wave_dots(theme.accent))
            .child(format!(
                "Working for {}",
                crate::utils::format_working_elapsed(seconds)
            ))
    }
}
