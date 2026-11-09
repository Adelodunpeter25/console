//! Speech-to-text account row for the Providers settings page.
//!
//! Deliberately separate from the LLM provider rows above it. Those
//! authenticate through a browser OAuth flow; speech-to-text stores a
//! Cloudflare account ID + API token on the server for a Whisper model.
//! The two flows have nothing else in common, and merging them would imply
//! a browser round-trip that a pasted pair does not need.
//!
//! Neither half is held anywhere after submission: the account ID is not a
//! secret, but the token is. Both inputs are cleared the moment Save is
//! clicked, and the status payload never carries the token.
//!
//! An environment-supplied pair is not ours to remove — the row hides
//! Disconnect in that case, like the API-key rows do.

use crate::theme::Theme;
use console_core::types::SttStatus;
use gpui::{
    App, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

use crate::input::ComposerInput;

#[derive(IntoElement)]
pub struct SttSection {
    pub status: Option<SttStatus>,
    /// True while the pair is being saved or removed.
    pub busy: bool,
    /// Whether the credentials form is showing.
    pub form_open: bool,
    /// Server-authored failure text, displayed verbatim.
    pub error: Option<String>,
    /// Present only while the form is open.
    pub account_input: Option<Entity<ComposerInput>>,
    /// Present only while the form is open.
    pub token_input: Option<Entity<ComposerInput>>,
    pub on_toggle_form: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_submit: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
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
        let form_open = self.form_open;
        let on_toggle_form = self.on_toggle_form.clone();
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
                            // Otherwise Connect toggles the credentials form.
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
                                                .child(if configured {
                                                    "Disconnect"
                                                } else if form_open {
                                                    "Cancel"
                                                } else {
                                                    "Connect"
                                                }),
                                        )
                                        .on_mouse_down(
                                            MouseButton::Left,
                                            move |_event, window, cx| {
                                                cx.stop_propagation();
                                                if configured {
                                                    (on_disconnect)(window, cx);
                                                } else {
                                                    (on_toggle_form)(window, cx);
                                                }
                                            },
                                        );
                                    button.into_any_element()
                                }
                            }),
                    ),
            )
            .child(build_form_area(
                form_open && !configured,
                self.account_input.clone(),
                self.token_input.clone(),
                self.error.clone(),
                self.on_submit.clone(),
                &theme,
            ))
            // Outside the form, errors still surface (e.g. a failed
            // disconnect) — otherwise the failure would be silent.
            .child(match (form_open, self.error.clone()) {
                (false, Some(error)) => div()
                    .text_size(px(11.5))
                    .text_color(theme.text_secondary)
                    .child(error)
                    .into_any_element(),
                _ => div().into_any_element(),
            })
            .into_any_element()
    }
}

/// Account ID + token inputs + save button, shown only while the form is
/// open.
///
/// Built as a standalone element rather than via a conditional style method:
/// gpui's Div in this version has no `if_else`, so the branch has to be an
/// ordinary `if` that picks between two already-built elements.
fn build_form_area(
    show: bool,
    account_input: Option<Entity<ComposerInput>>,
    token_input: Option<Entity<ComposerInput>>,
    error: Option<String>,
    on_submit: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> gpui::AnyElement {
    if !show {
        return div().into_any_element();
    }

    let mut body = div().flex().flex_col().gap(px(8.0));
    if let (Some(account_input), Some(token_input)) = (account_input, token_input) {
        body = body.child(
            div()
                .border_1()
                .border_color(theme.border)
                .bg(theme.canvas)
                .rounded(px(6.0))
                .px(px(8.0))
                .py(px(6.0))
                .child(account_input.clone()),
        );
        body = body.child(
            div()
                .border_1()
                .border_color(theme.border)
                .bg(theme.canvas)
                .rounded(px(6.0))
                .px(px(8.0))
                .py(px(6.0))
                .child(token_input.clone()),
        );

        let submit_account = account_input;
        let submit_token = token_input;
        body = body.child(
            div()
                .id("btn-stt-connect")
                .px(px(10.0))
                .py(px(5.0))
                .rounded(px(6.0))
                .bg(theme.accent)
                .cursor_pointer()
                .hover(|s| s.opacity(0.9))
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.on_inverse)
                        .child("Connect"),
                )
                .on_mouse_down(
                    MouseButton::Left,
                    move |_event, window, cx| {
                        cx.stop_propagation();
                        // Read both halves out of their inputs at click time
                        // and hand ownership to the caller, which drops them
                        // once the request completes. Nothing here keeps a
                        // copy.
                        let account_id = submit_account.read(cx).content().to_string();
                        let api_token = submit_token.read(cx).content().to_string();
                        (on_submit)(account_id, api_token, window, cx);
                    },
                ),
        );
    }
    if let Some(error) = error {
        body = body.child(
            div()
                .text_size(px(11.5))
                .text_color(theme.text_secondary)
                .child(error),
        );
    }
    body.into_any_element()
}
