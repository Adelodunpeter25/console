//! Speech-to-text account row for the Providers settings page.
//!
//! Deliberately separate from the LLM provider rows above it. Those
//! authenticate through a browser OAuth flow; speech-to-text stores a
//! Cloudflare account ID + API token on the server for a Whisper model.
//! The two flows have nothing else in common, and merging them would imply
//! a browser round-trip that a pasted pair does not need.
//!
//! The pair is entered in [`SttConfigModal`] — a modal, not an inline form —
//! because two labelled fields read better as a dialog than crammed into the
//! row. Neither half is held anywhere after submission: both inputs are
//! cleared the moment Connect is clicked, and the status payload never
//! carries the token.
//!
//! An environment-supplied pair is not ours to remove — the row hides
//! Disconnect in that case, like the API-key rows do.

use super::Modal;
use crate::input::ComposerInput;
use crate::theme::Theme;
use console_core::types::SttStatus;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct SttSection {
    pub status: Option<SttStatus>,
    /// True while the pair is being saved or removed.
    pub busy: bool,
    /// Server-authored failure text for a disconnect, shown verbatim.
    /// Submit errors render inside the modal instead.
    pub error: Option<String>,
    pub on_connect: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_disconnect: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for SttSection {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // `configured` is an optional bool on the wire: the server always
        // sends it, but an absent key must read as "not connected" rather
        // than panicking on an unwrap.
        let configured = self.status.as_ref().and_then(|s| s.configured) == Some(true);
        let from_env = self.status.as_ref().and_then(|s| s.source.as_deref()) == Some("env");
        let account_id = self
            .status
            .as_ref()
            .and_then(|s| s.account_id.clone())
            .filter(|a| !a.trim().is_empty());
        let model = self
            .status
            .as_ref()
            .map(|s| s.model.clone())
            .unwrap_or_default();

        let busy = self.busy;
        let on_connect = self.on_connect.clone();
        let on_disconnect = self.on_disconnect.clone();

        let status_text = if busy {
            "Working...".to_string()
        } else if configured {
            match &account_id {
                Some(id) => format!("Connected — account {id}"),
                None => "Connected".to_string(),
            }
        } else {
            "Not connected".to_string()
        };

        let detail_text = if configured && from_env {
            "Configured through the server environment.".to_string()
        } else if configured {
            format!("Transcribes recordings with {model}.")
        } else {
            "Add a Cloudflare account ID and API token to dictate recordings into text.".to_string()
        };

        let status_color = if busy {
            theme.accent
        } else if configured {
            theme.accent
        } else {
            theme.text_ghost
        };

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            // Section heading, matching the "GitHub" block above so the two
            // read as siblings rather than one being nested inside the other.
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
                            .child("Speech to Text"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child(
                                "Transcribes recordings via a Cloudflare Whisper model on this server.",
                            ),
                    ),
            )
            .child(
                div()
                    .p(px(12.0))
                    .rounded(px(8.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .flex()
                    .flex_col()
                    .gap(px(10.0))
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
                                        div().w(px(8.0)).h(px(8.0)).rounded_full().bg(status_color),
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
                                                    .child(status_text),
                                            )
                                            .child(
                                                div()
                                                    .text_size(px(11.5))
                                                    .text_color(theme.text_tertiary)
                                                    .child(detail_text),
                                            ),
                                    ),
                            )
                            // A stored pair gets Disconnect; an env pair is
                            // not ours to remove, so it gets no button at all.
                            // Otherwise Connect opens the credentials modal.
                            .child({
                                if busy || (configured && from_env) {
                                    div().into_any_element()
                                } else {
                                    let button = div()
                                        .id("btn-stt-action")
                                        .px(px(10.0))
                                        .py(px(5.0))
                                        .rounded(px(6.0))
                                        .border_1()
                                        .border_color(theme.border_strong)
                                        .bg(theme.raised)
                                        .cursor_pointer()
                                        .hover(|s| s.bg(theme.overlay))
                                        .active(|s| s.bg(theme.overlay_strong))
                                        .child(
                                            div()
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text)
                                                .child(if configured { "Disconnect" } else { "Connect" }),
                                        )
                                        .on_mouse_down(
                                            MouseButton::Left,
                                            move |_event, window, cx| {
                                                cx.stop_propagation();
                                                if configured {
                                                    (on_disconnect)(window, cx);
                                                } else {
                                                    (on_connect)(window, cx);
                                                }
                                            },
                                        );
                                    button.into_any_element()
                                }
                            }),
                    ),
            )
            // Disconnect failures surface under the row — otherwise a failed
            // remove would be silent. Submit errors live in the modal.
            .child(match self.error.clone() {
                Some(error) => div()
                    .text_size(px(11.5))
                    .text_color(theme.text_secondary)
                    .child(error)
                    .into_any_element(),
                None => div().into_any_element(),
            })
            .into_any_element()
    }
}

/// The Connect dialog, rendered in the shared settings modal shell above the
/// whole window (like the rename dialog). The two inputs arrive empty each
/// time it opens; on Connect the caller reads them, hands the pair to the
/// server, and clears both fields.
///
/// `busy` keeps the dialog up with its error on a failed save, so the pair
/// the user typed is still there to fix and resubmit.
#[derive(IntoElement)]
pub struct SttConfigModal {
    pub account_input: Option<Entity<ComposerInput>>,
    pub token_input: Option<Entity<ComposerInput>>,
    /// True while the pair is being saved: Connect reads "Connecting...".
    pub busy: bool,
    /// Server-authored failure text for the last save attempt, verbatim.
    pub error: Option<String>,
    pub on_close: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for SttConfigModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let busy = self.busy;

        let field = |label: &'static str, input: Option<Entity<ComposerInput>>| {
            div()
                .flex()
                .flex_col()
                .gap(px(4.0))
                .child(
                    div()
                        .text_size(px(11.5))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text_secondary)
                        .child(label),
                )
                .child(
                    div()
                        .p(px(6.0))
                        .rounded(px(6.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.canvas)
                        .child(match input {
                            Some(input) => input.into_any_element(),
                            None => div().into_any_element(),
                        }),
                )
        };

        let content = div()
            .flex()
            .flex_col()
            .gap(px(10.0))
            .child(field("Cloudflare account ID", self.account_input.clone()))
            .child(field("API token", self.token_input.clone()))
            .child(
                div()
                    .text_size(px(11.0))
                    .text_color(theme.text_tertiary)
                    .child(
                        "Stored on this server only. The token is never shown again — to replace it, disconnect and connect with the new pair.",
                    ),
            )
            .when_some(self.error.clone(), |el, error| {
                el.child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.danger)
                        .child(error),
                )
            });

        let footer = div()
            .flex()
            .items_center()
            .justify_end()
            .gap(px(8.0))
            .child({
                let on_close = self.on_close.clone();
                div()
                    .px(px(10.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.raised)
                    .cursor_pointer()
                    .hover(|s| s.bg(theme.overlay))
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        cx.stop_propagation();
                        (on_close)(window, cx);
                    })
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .child("Cancel"),
                    )
            })
            .child({
                let on_save = self.on_save.clone();
                div()
                    .id("btn-stt-modal-connect")
                    .px(px(10.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .when(!busy, |el| {
                        el.bg(theme.accent)
                            .cursor_pointer()
                            .hover(|s| s.opacity(0.9))
                    })
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        if busy {
                            return;
                        }
                        cx.stop_propagation();
                        (on_save)(window, cx);
                    })
                    .child(
                        div()
                            .text_size(px(11.5))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.on_inverse)
                            .child(if busy { "Connecting..." } else { "Connect" }),
                    )
            });

        Modal {
            title: "Connect Speech to Text".to_string(),
            width: 420.0,
            content: content.into_any_element(),
            footer: footer.into_any_element(),
            on_close: self.on_close.clone(),
        }
    }
}
