//! Browser settings page: Chrome data import (profiles, history, cookies).

use crate::primitives::icons::{IconName, app_icon};
use crate::primitives::tooltip::Tooltip;
use crate::theme::Theme;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

/// One row in the profile picker.
#[derive(Clone, Debug)]
pub struct BrowserProfileRow {
    pub id: String,
    pub name: String,
}

#[derive(IntoElement)]
pub struct BrowserPage {
    /// Chrome profiles with data on this machine.
    pub profiles: Vec<BrowserProfileRow>,
    pub selected_profile: Option<usize>,
    pub loading_profiles: bool,
    pub importing: bool,
    /// Import result / error line.
    pub status: Option<String>,
    pub history_count: usize,
    pub on_refresh_profiles: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_select_profile: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    pub on_import: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for BrowserPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_refresh = self.on_refresh_profiles.clone();
        let on_import = self.on_import.clone();

        let mut container = div().flex().flex_col().gap(px(16.0));

        container = container.child(
            div()
                .flex()
                .flex_col()
                .gap(px(4.0))
                .child(
                    div()
                        .text_size(px(16.0))
                        .font_weight(FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child("Browser"),
                )
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_secondary)
                        .child(
                            "Import your Chrome profile into the built-in browser: its history feeds the address bar, and its cookies log you in where you already are.",
                        ),
                ),
        );

        // History summary: the store keeps unique sites, not raw visits.
        container = container.child(
            div()
                .flex()
                .items_center()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_secondary)
                        .child(format!(
                            "{} {} in history",
                            self.history_count,
                            if self.history_count == 1 {
                                "site"
                            } else {
                                "sites"
                            }
                        )),
                )
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_tertiary)
                        .child("(repeated Chrome visits merge into one entry)"),
                ),
        );

        // Profile picker.
        let profiles_card = if self.loading_profiles {
            div()
                .p(px(24.0))
                .rounded(px(8.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .flex()
                .items_center()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_secondary)
                        .child("Looking for Chrome profiles…"),
                )
        } else if self.profiles.is_empty() {
            div()
                .p(px(24.0))
                .rounded(px(8.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .flex()
                .flex_col()
                .items_center()
                .justify_center()
                .gap(px(8.0))
                .child(app_icon(IconName::Globe, 24.0, theme.text_ghost))
                .child(
                    div()
                        .text_size(px(13.0))
                        .text_color(theme.text_secondary)
                        .child("No Chrome profiles found on this machine."),
                )
        } else {
            div()
                .flex()
                .flex_col()
                .rounded(px(8.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .overflow_hidden()
                .children(self.profiles.iter().enumerate().map(|(index, profile)| {
                    let on_select = self.on_select_profile.clone();
                    let selected = self.selected_profile == Some(index);
                    let id = ElementId::from(format!("chrome-profile-{}", profile.id));
                    div()
                        .id(id)
                        .cursor_pointer()
                        .hover(|s| s.bg(theme.overlay))
                        .when(selected, |s| s.bg(theme.overlay_strong))
                        .tooltip(Tooltip::text(profile.id.clone()))
                        .on_click(move |_event, window, cx| (on_select)(index, window, cx))
                        .flex()
                        .items_center()
                        .gap(px(10.0))
                        .px(px(12.0))
                        .py(px(10.0))
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .flex()
                                .flex_col()
                                .gap(px(2.0))
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .gap(px(6.0))
                                        .child(
                                            div()
                                                .text_size(px(13.5))
                                                .font_weight(FontWeight::MEDIUM)
                                                .text_color(theme.text)
                                                .child(profile.name.clone()),
                                        )
                                        .when(selected, |s| {
                                            s.child(
                                                div()
                                                    .rounded(px(3.0))
                                                    .bg(theme.accent)
                                                    .px(px(5.0))
                                                    .text_size(px(10.0))
                                                    .font_weight(FontWeight::MEDIUM)
                                                    .text_color(theme.on_inverse)
                                                    .child("selected"),
                                            )
                                        }),
                                )
                                .child(
                                    div()
                                        .text_size(px(11.5))
                                        .text_color(theme.text_tertiary)
                                        .child(profile.id.clone()),
                                ),
                        )
                }))
        };

        container = container.child(
            div()
                .flex()
                .flex_col()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(13.0))
                        .font_weight(FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child("Import from Chrome"),
                )
                .child(profiles_card)
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(10.0))
                        .child(
                            div()
                                .id("browser-import-refresh")
                                .cursor_pointer()
                                .rounded(px(6.0))
                                .border_1()
                                .border_color(theme.border)
                                .px(px(10.0))
                                .py(px(6.0))
                                .text_size(px(12.0))
                                .text_color(theme.text_secondary)
                                .hover(|s| s.bg(theme.overlay))
                                .child("Refresh profiles")
                                .tooltip(Tooltip::text("Re-scan Chrome on this machine"))
                                .on_click(move |_event, window, cx| (on_refresh)(window, cx)),
                        )
                        .child(
                            div()
                                .id("browser-import-run")
                                .when(self.importing || self.selected_profile.is_none(), |el| {
                                    el.opacity(0.4)
                                })
                                .cursor_pointer()
                                .rounded(px(6.0))
                                .bg(theme.accent)
                                .px(px(12.0))
                                .py(px(6.0))
                                .text_size(px(12.0))
                                .font_weight(FontWeight::MEDIUM)
                                .text_color(theme.on_inverse)
                                .when(!self.importing && self.selected_profile.is_some(), |el| {
                                    el.hover(|s| s.opacity(0.9))
                                })
                                .child(if self.importing {
                                    "Importing…"
                                } else {
                                    "Import"
                                })
                                .tooltip(Tooltip::text(
                                    "Import history and cookies from the selected profile",
                                ))
                                .on_click(move |_event, window, cx| {
                                    if !(self.importing || self.selected_profile.is_none()) {
                                        (on_import)(window, cx);
                                    }
                                }),
                        ),
                ),
        );

        // Status line and the consent note.
        if let Some(status) = &self.status {
            container = container.child(
                div()
                    .p(px(10.0))
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.canvas)
                    .text_size(px(12.0))
                    .text_color(theme.text_secondary)
                    .child(status.clone()),
            );
        }

        container = container.child(
            div()
                .text_size(px(11.5))
                .text_color(theme.text_tertiary)
                .child(
                    "macOS asks once to let Console read the Chrome cookie encryption key. Cookies decrypt in memory and go straight into Console's webview — they are never written to disk, and imported history stays on this machine.",
                ),
        );

        container
    }
}
