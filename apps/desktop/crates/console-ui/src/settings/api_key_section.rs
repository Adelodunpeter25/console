use crate::input::ComposerInput;
use crate::theme::Theme;
use console_core::types::ProviderCredentialStatus;
use gpui::{
    App, Entity, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

/// A provider whose credential is a key the user pastes once, rather than an
/// OAuth session the server renews. Rendered in place of the login button when
/// the catalog reports `auth_method == "api-key"`.
///
/// Three states, because the third one matters:
///  - no key: the provider still works on its free tier, so the row says so
///    rather than implying something is broken;
///  - key stored: "Using your API key";
///  - key from the server's environment: removing it here cannot take effect,
///    so no remove button is offered — one that appeared to work and changed
///    nothing would be worse than none.
///
/// The key is read out of the input at submit time and handed to the caller,
/// which drops it; nothing on this side keeps a copy.
#[derive(IntoElement)]
pub struct ApiKeyProviderSection {
    pub provider_id: String,
    pub display_name: String,
    pub description: String,
    pub status: Option<ProviderCredentialStatus>,
    /// True while the key is being saved or removed.
    pub busy: bool,
    /// Whether the key input is showing.
    pub form_open: bool,
    /// Server-authored failure text, displayed verbatim.
    pub error: Option<String>,
    /// Present only while the form is open.
    pub key_input: Option<Entity<ComposerInput>>,
    pub on_toggle_form: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_remove: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

/// What a key row should say and offer, derived from the server's status.
///
/// Split out of the render fn because the interesting part is not drawing it:
/// it is that "a key is configured" and "a key is configured *here*" are
/// different claims. A key from the server's environment survives a remove, so
/// `can_remove` is false even though `configured` is true — a disconnect button
/// that appeared to work and changed nothing would be worse than none.
#[derive(Debug, PartialEq, Eq)]
pub struct KeyRowState {
    pub configured: bool,
    pub from_env: bool,
    /// True only when this client can actually take the key away.
    pub can_remove: bool,
    pub status_text: &'static str,
}

impl KeyRowState {
    pub fn from_status(status: Option<&ProviderCredentialStatus>, busy: bool) -> Self {
        // Both fields are explicit-presence on the wire. The server always
        // sends `configured`, but an absent key must read as "not configured"
        // rather than being treated as true.
        let configured = status.and_then(|s| s.configured) == Some(true);
        let from_env = status.and_then(|s| s.source.as_deref()) == Some("env");

        let status_text = if busy {
            "Saving..."
        } else if from_env {
            "Using API key from server environment"
        } else if configured {
            "Using your API key"
        } else {
            "Using the free tier"
        };

        Self {
            configured,
            from_env,
            can_remove: configured && !from_env,
            status_text,
        }
    }
}

impl RenderOnce for ApiKeyProviderSection {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let state = KeyRowState::from_status(self.status.as_ref(), self.busy);
        let configured = state.configured;
        let status_text = state.status_text.to_string();
        let status_color = if configured {
            theme.accent
        } else {
            theme.text_ghost
        };

        let pid = self.provider_id.clone();
        let mut row = div()
            .id(ElementId::from(format!("apikey-row-{pid}")))
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
                                div()
                                    .w(px(8.0))
                                    .h(px(8.0))
                                    .rounded_full()
                                    .bg(status_color),
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
                                            .child(self.display_name.clone()),
                                    )
                                    .child(
                                        div()
                                            .text_size(px(11.5))
                                            .text_color(theme.text_tertiary)
                                            .child(status_text),
                                    ),
                            ),
                    )
                    .child(div().flex().items_center().gap(px(8.0))
                        // An env-supplied key is not ours to remove.
                        .child(if state.can_remove {
                            div()
                                .id(ElementId::from(format!("btn-apikey-remove-{pid}")))
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
                                        .child("Remove"),
                                )
                                .on_mouse_down(MouseButton::Left, {
                                    let on_remove = self.on_remove.clone();
                                    move |_event, window, cx| {
                                        cx.stop_propagation();
                                        (on_remove)(window, cx);
                                    }
                                })
                                .into_any_element()
                        } else {
                            div().into_any_element()
                        })
                        .child({
                            let on_toggle = self.on_toggle_form.clone();
                            div()
                                .id(ElementId::from(format!("btn-apikey-{pid}")))
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
                                        .child(if configured { "Replace" } else { "Add key" }),
                                )
                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                    cx.stop_propagation();
                                    (on_toggle)(window, cx);
                                })
                        })),
            );

        if !self.description.is_empty() {
            row = row.child(
                div()
                    .text_size(px(11.5))
                    .text_color(theme.text_tertiary)
                    .child(self.description.clone()),
            );
        }

        if self.form_open {
            if let Some(input) = self.key_input.clone() {
                // Clone for the widget, keep the original for the click
                // handler that reads the typed value.
                let submit_input = input.clone();
                row = row.child(
                    div()
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.canvas)
                        .rounded(px(6.0))
                        .px(px(8.0))
                        .py(px(6.0))
                        .child(input.clone()),
                );
                row = row.child(
                    div()
                        .id(ElementId::from(format!("btn-apikey-save-{pid}")))
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
                                .child("Save"),
                        )
                        .on_mouse_down(MouseButton::Left, {
                            let on_submit = self.on_submit.clone();
                            move |_event, window, cx| {
                                cx.stop_propagation();
                                // Read the key out of the input at click time and
                                // hand ownership to the caller, which drops it once
                                // the request completes.
                                let key = submit_input.read(cx).content().to_string();
                                if key.trim().is_empty() {
                                    return;
                                }
                                (on_submit)(key.trim().to_string(), window, cx);
                            }
                        }),
                );
            }
            if let Some(error) = self.error {
                row = row.child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_secondary)
                        .child(error),
                );
            }
        }

        row.into_any_element()
    }
}