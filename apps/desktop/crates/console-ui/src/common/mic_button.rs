//! The dictation mic button, shared by the docked composer and the floating
//! card so the two surfaces cannot drift apart. Purely presentational: the
//! caller owns the recording flow and hands in the live state plus the click
//! handler (the docked composer passes `None` where a click must be ignored,
//! e.g. mid-transcription).

use std::rc::Rc;

use gpui::{
    AnyElement, App, FontWeight, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// The distinct, mutually exclusive phases of the dictation control.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub enum MicPhase {
    /// False until STT credentials are configured.
    #[default]
    Disabled,
    /// Credentials configured and ready to record.
    Idle,
    /// A take is live with the elapsed seconds ticking.
    Recording { elapsed_secs: u32 },
    /// A finished take is being transcribed.
    Transcribing,
}

/// One frame of dictation state, mirrored from the app's STT flow.
#[derive(Clone, Copy, Default, Debug, PartialEq, Eq)]
pub struct MicState {
    /// False until STT credentials are configured — the idle button is then
    /// disabled with a tooltip naming the fix.
    pub enabled: bool,
    /// A take is live: the stop control replaces the mic, with the elapsed
    /// label ticking.
    pub recording: bool,
    pub elapsed_secs: u32,
    /// A finished take is being transcribed: an inert spinner.
    pub transcribing: bool,
}

impl MicState {
    /// Resolve the current mutually-exclusive phase.
    pub fn phase(&self) -> MicPhase {
        if self.recording {
            MicPhase::Recording {
                elapsed_secs: self.elapsed_secs,
            }
        } else if self.transcribing {
            MicPhase::Transcribing
        } else if self.enabled {
            MicPhase::Idle
        } else {
            MicPhase::Disabled
        }
    }
}

/// The three-state mic control. `id_prefix` namespaces the element ids so the
/// docked composer ("btn") and the floating card ("floating") never collide.
pub fn mic_button(
    id_prefix: &str,
    state: MicState,
    on_toggle: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    theme: &Theme,
) -> AnyElement {
    match state.phase() {
        MicPhase::Recording { elapsed_secs } => {
            let on_toggle = on_toggle.clone();
            div()
                .id(format!("{id_prefix}-mic-stop"))
                .flex()
                .items_center()
                .gap(px(4.0))
                .p(px(4.0))
                .rounded(px(6.0))
                .bg(theme.danger_soft)
                .cursor_default()
                .hover(|style| style.opacity(0.8))
                .on_click(move |_, window, cx| {
                    if let Some(on_toggle) = &on_toggle {
                        (on_toggle)(window, cx);
                    }
                })
                .child(app_icon(IconName::StopFilled, 13.0, theme.danger))
                .child(
                    div()
                        .text_size(px(11.5))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.danger)
                        .child(format!("{}:{:02}", elapsed_secs / 60, elapsed_secs % 60)),
                )
                .into_any_element()
        }
        MicPhase::Transcribing => div()
            .id(format!("{id_prefix}-mic-transcribing"))
            .p(px(5.0))
            .rounded(px(6.0))
            .child(crate::primitives::motion::spin(
                IconName::LoaderCircle,
                13.0,
                theme.text_tertiary,
            ))
            .into_any_element(),
        MicPhase::Idle => div()
            .id(format!("{id_prefix}-mic"))
            .p(px(5.0))
            .rounded(px(6.0))
            .cursor_default()
            .hover(|style| style.bg(theme.overlay))
            .on_click(move |_, window, cx| {
                if let Some(on_toggle) = &on_toggle {
                    (on_toggle)(window, cx);
                }
            })
            .child(app_icon(IconName::Mic, 13.0, theme.text_tertiary))
            .into_any_element(),
        MicPhase::Disabled => div()
            .id(format!("{id_prefix}-mic"))
            .p(px(5.0))
            .rounded(px(6.0))
            .opacity(0.35)
            .tooltip(Tooltip::text(
                "Set up Speech to Text in Settings › Providers",
            ))
            .child(app_icon(IconName::Mic, 13.0, theme.text_tertiary))
            .into_any_element(),
    }
}
