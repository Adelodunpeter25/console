//! Integrations settings page: helper binaries the *server* shells out to
//! (gh CLI today), with live installed/missing status and the command to
//! install what is missing.
//!
//! Read-only by design — there is no install button. The install happens on
//! the box, outside this UI; the page tells the user what to run and where.
//! Detection runs live server-side on every refresh, so installing then
//! hitting Refresh flips the row with no restart.

use crate::theme::Theme;
use console_core::types::IntegrationStatus;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct IntegrationsPage {
    pub integrations: Vec<IntegrationStatus>,
    pub loading: bool,
    pub error: Option<String>,
    pub on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    /// Install a missing binary the official way for the server's platform.
    /// Shown only on rows that report not installed.
    pub on_install: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for IntegrationsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .items_start()
                    .justify_between()
                    .gap(px(12.0))
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
                                    .child("Integrations"),
                            )
                            .child(
                                div()
                                    .text_size(px(12.5))
                                    .text_color(theme.text_secondary)
                                    .child(
                                        "Helper tools the server uses on your behalf. Status is checked live on the server — install anything missing, then refresh.",
                                    ),
                            ),
                    )
                    .child(refresh_button(self.loading, self.on_refresh.clone(), &theme)),
            )
            .when_some(self.error.clone(), |el, err| {
                el.child(
                    div()
                        .px(px(12.0))
                        .py(px(8.0))
                        .rounded(px(6.0))
                        .bg(theme.danger.opacity(0.12))
                        .border_1()
                        .border_color(theme.danger)
                        .text_size(px(12.0))
                        .text_color(theme.danger)
                        .child(err),
                )
            })
            .child(if self.integrations.is_empty() {
                div()
                    .px(px(2.0))
                    .text_size(px(12.5))
                    .text_color(theme.text_ghost)
                    .child(if self.loading {
                        "Checking the server…"
                    } else {
                        "No integrations reported."
                    })
                    .into_any_element()
            } else {
                div()
                    .flex()
                    .flex_col()
                    .gap(px(10.0))
                    .children(self.integrations.iter().map(|row| {
                        integration_row(
                            row,
                            self.loading,
                            self.on_install.clone(),
                            &theme,
                        )
                    }))
                    .into_any_element()
            })
    }
}

fn refresh_button(
    loading: bool,
    on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> impl IntoElement {
    let button = div()
        .id("integrations-refresh")
        .px(px(12.0))
        .py(px(6.0))
        .rounded(px(6.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .opacity(if loading { 0.5 } else { 1.0 })
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(theme.text)
                .child(if loading { "Checking…" } else { "Refresh" }),
        );
    if loading {
        button.into_any_element()
    } else {
        button
            .cursor_pointer()
            .hover(|s| s.opacity(0.9))
            .on_click(move |_event, window, cx| {
                cx.stop_propagation();
                on_refresh(window, cx);
            })
            .into_any_element()
    }
}

/// One integration: name + status pill, description, then either the detected
/// version/path or the install hint + URL with an Install button.
fn integration_row(
    row: &IntegrationStatus,
    loading: bool,
    on_install: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> impl IntoElement {
    // `installed` has explicit presence on the wire — a missing key reads as
    // not installed, never as unknown.
    let installed = row.installed.unwrap_or(false);
    let (pill_text, pill_color) = if installed {
        ("Installed", theme.success)
    } else {
        ("Not installed", theme.danger)
    };

    let mut card = div()
        .flex()
        .flex_col()
        .gap(px(8.0))
        .p(px(12.0))
        .rounded(px(8.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(13.0))
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child(row.name.clone()),
                )
                .child(
                    div()
                        .px(px(8.0))
                        .py(px(2.0))
                        .rounded(px(999.0))
                        .bg(pill_color.opacity(0.12))
                        .child(
                            div()
                                .text_size(px(11.0))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(pill_color)
                                .child(pill_text),
                        ),
                ),
        )
        .child(
            div()
                .text_size(px(12.0))
                .text_color(theme.text_secondary)
                .child(row.description.clone()),
        );

    if installed {
        // Version/path are optional on the wire: a binary that resolves but
        // whose probe fails still reads installed, just without details.
        let mut details = String::new();
        if let Some(version) = row.version.as_ref() {
            details.push_str(&format!("v{version}"));
        }
        if let Some(path) = row.binary_path.as_ref() {
            if !details.is_empty() {
                details.push_str(" · ");
            }
            details.push_str(path);
        }
        if !details.is_empty() {
            card = card.child(
                div()
                    .text_size(px(11.5))
                    .text_color(theme.text_ghost)
                    .child(details),
            );
        }
    } else {
        let install_button = {
            let button = div()
                .id("integrations-install-gh")
                .px(px(12.0))
                .py(px(6.0))
                .rounded(px(6.0))
                .bg(theme.accent)
                .opacity(if loading { 0.5 } else { 1.0 })
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.on_inverse)
                        .child(if loading { "Installing…" } else { "Install" }),
                );
            if loading {
                button.into_any_element()
            } else {
                button
                    .cursor_pointer()
                    .hover(|s| s.opacity(0.9))
                    .on_click(move |_event, window, cx| {
                        cx.stop_propagation();
                        on_install(window, cx);
                    })
                    .into_any_element()
            }
        };
        card = card.child(
            div()
                .flex()
                .flex_col()
                .gap(px(8.0))
                .rounded(px(6.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.canvas)
                .px(px(10.0))
                .py(px(8.0))
                .child(
                    div()
                        .flex()
                        .items_center()
                        .justify_between()
                        .gap(px(8.0))
                        .child(
                            div()
                                .flex_1()
                                .flex()
                                .flex_col()
                                .gap(px(4.0))
                                .child(
                                    div()
                                        .text_size(px(11.5))
                                        .text_color(theme.text_secondary)
                                        .child(format!("Install with: {}", row.install_hint)),
                                )
                                .child(
                                    div()
                                        .text_size(px(11.0))
                                        .text_color(theme.text_ghost)
                                        .child(row.install_url.clone()),
                                ),
                        )
                        .child(install_button),
                ),
        );
    }

    card.into_any_element()
}
