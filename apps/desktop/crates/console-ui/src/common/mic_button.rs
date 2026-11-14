//! The dictation mic button, shared by the docked composer and the floating
//! card so the two surfaces cannot drift apart. Purely presentational: the
//! caller owns the recording flow and hands in the live state plus the click
//! handler (the docked composer passes `None` where a click must be ignored,
//! e.g. mid-transcription).

use std::rc::Rc;

use gpui::{
    App, AnyElement, FontWeight, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use gpui::prelude::FluentBuilder;

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// One frame of dictation state, mirrored from the app's STT flow.
#[derive(Clone, Copy, Default)]
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

/// The three-state mic control. `id_prefix` namespaces the element ids so the
/// docked composer ("btn") and the floating card ("floating") never collide.
pub fn mic_button(
    id_prefix: &str,
    state: MicState,
    on_toggle: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    theme: &Theme,
) -> AnyElement {
    if state.recording {
        let on_toggle = on_toggle.clone();
        return div()
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
                    .child(format!(
                        "{}:{:02}",
                        state.elapsed_secs / 60,
                        state.elapsed_secs % 60
                    )),
            )
            .into_any_element();
    }
    if state.transcribing {
        return div()
            .id(format!("{id_prefix}-mic-transcribing"))
            .p(px(5.0))
            .rounded(px(6.0))
            .child(crate::primitives::motion::spin(app_icon(
                IconName::LoaderCircle,
                13.0,
                theme.text_tertiary,
            )))
            .into_any_element();
    }
    let button = div()
        .id(format!("{id_prefix}-mic"))
        .p(px(5.0))
        .rounded(px(6.0))
        .when(state.enabled, |el| {
            el.cursor_default()
                .hover(|style| style.bg(theme.overlay))
                .on_click(move |_, window, cx| {
                    if let Some(on_toggle) = &on_toggle {
                        (on_toggle)(window, cx);
                    }
                })
        })
        .when(!state.enabled, |el| {
            el.opacity(0.35)
                .tooltip(Tooltip::text("Set up Speech to Text in Settings › Providers"))
        })
        .child(app_icon(IconName::Mic, 13.0, theme.text_tertiary));
    button.into_any_element()
}
