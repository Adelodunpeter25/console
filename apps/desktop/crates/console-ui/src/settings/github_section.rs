//! GitHub account row for the Providers settings page.
//!
//! Deliberately separate from the LLM provider rows above it. Those
//! authenticate through a browser OAuth flow; GitHub uses a pasted personal
//! access token, because its token exists for the git CLI on the server host
//! (private repos) rather than for chat. The two flows have nothing else in
//! common, and merging them would imply a browser round-trip that a PAT does
//! not need.
//!
//! The token is held only in the caller's input entity while it is being
//! submitted, then cleared. Nothing here retains it.

use crate::theme::Theme;
use console_core::types::GitHubAuthStatus;
use gpui::{
    App, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce, Styled,
    Window, div, px,
};
use std::rc::Rc;

use crate::input::ComposerInput;

#[derive(IntoElement)]
pub struct GitHubSection {
    pub status: Option<GitHubAuthStatus>,
    /// True while a PAT is being validated or a disconnect is in flight.
    pub busy: bool,
    /// Whether the token input is showing.
    pub form_open: bool,
    /// Server-authored failure text, displayed verbatim.
    pub error: Option<String>,
    /// Present only while the form is open.
    pub token_input: Option<Entity<ComposerInput>>,
    pub on_toggle_form: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_disconnect: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for GitHubSection {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // `connected` is an optional bool on the wire: the server always sends
        // it, but an absent key must read as "not connected" rather than
        // panicking on an unwrap.
        let connected = self.status.as_ref().and_then(|s| s.connected) == Some(true);
        let username = self
            .status
            .as_ref()
            .and_then(|s| s.username.clone())
            .filter(|u| !u.trim().is_empty());
        let scopes = self
            .status
            .as_ref()
            .map(|s| s.scopes.clone())
            .unwrap_or_default();

        let busy = self.busy;
        let form_open = self.form_open;
        let on_toggle_form = self.on_toggle_form.clone();
        let on_disconnect = self.on_disconnect.clone();

        let status_text = if busy {
            "Working...".to_string()
        } else if connected {
            match &username {
                Some(name) => format!("Connected as @{name}"),
                None => "Connected".to_string(),
            }
        } else {
            "Not connected".to_string()
        };

        // Scopes are only worth a second line when there are any; a
        // fine-grained token legitimately returns none.
        let detail_text = if connected && !scopes.is_empty() {
            format!("Scopes: {}", scopes.join(", "))
        } else if connected {
            "Used by git to clone private repositories.".to_string()
        } else {
            "Add a personal access token so git can clone private repositories.".to_string()
        };

        let status_color = if busy {
            theme.accent
        } else if connected {
            theme.accent
        } else {
            theme.text_ghost
        };

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            // Section heading, matching the "AI Providers" block above so the
            // two read as siblings rather than one being nested inside the
            // other.
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
                            .child("GitHub"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child(
                                "Lets this backend clone private repositories over HTTPS or SSH.",
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
                            // Connected shows Disconnect; otherwise Connect toggles the
                            // token form. Never both.
                            .child({
                                let button = div()
                                    .id("btn-github-action")
                                    .px(px(10.0))
                                    .py(px(5.0))
                                    .rounded(px(6.0))
                                    .border_1()
                                    .border_color(theme.border_strong)
                                    .bg(theme.raised)
                                    .opacity(if busy { 0.5 } else { 1.0 })
                                    .child(
                                        div()
                                            .text_size(px(12.0))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.text)
                                            .child(if busy {
                                                "Working..."
                                            } else if connected {
                                                "Disconnect"
                                            } else if form_open {
                                                "Cancel"
                                            } else {
                                                "Connect"
                                            }),
                                    )
                                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                        cx.stop_propagation();
                                        if busy {
                                            return;
                                        }
                                        if connected {
                                            (on_disconnect)(window, cx);
                                        } else {
                                            (on_toggle_form)(window, cx);
                                        }
                                    });
                                // No pointer and no hover while a request is
                                // in flight: a live-looking button that ignores
                                // clicks reads as a dropped click.
                                if busy {
                                    button.into_any_element()
                                } else {
                                    button.hover(|s| s.bg(theme.overlay)).into_any_element()
                                }
                            }),
                    ),
            )
            .child(build_form_area(
                form_open && !connected,
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

/// Token input + submit button, shown only while the form is open.
///
/// Built as a standalone element rather than via a conditional style method:
/// gpui's Div in this version has no `if_else`, so the branch has to be an
/// ordinary `if` that picks between two already-built elements.
fn build_form_area(
    show: bool,
    input: Option<Entity<ComposerInput>>,
    error: Option<String>,
    on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> gpui::AnyElement {
    if !show {
        return div().into_any_element();
    }

    let mut body = div().flex().flex_col().gap(px(8.0));
    if let Some(input) = input.clone() {
        body = body.child(
            div()
                .border_1()
                .border_color(theme.border)
                .bg(theme.canvas)
                .rounded(px(6.0))
                .px(px(8.0))
                .py(px(6.0))
                .child(input.clone()),
        );

        let submit_input = input;
        body = body.child(
            div()
                .id("btn-github-connect")
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
                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                    cx.stop_propagation();
                    // Read the token out of the input at click time and hand
                    // ownership to the caller, which drops it once the request
                    // completes. Nothing here keeps a copy.
                    let token = submit_input.read(cx).content().to_string();
                    (on_submit)(token, window, cx);
                }),
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
