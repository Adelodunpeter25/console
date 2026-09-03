use super::SettingsTab;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct SettingsShell {
    pub active_tab: SettingsTab,
    pub on_select_tab: Rc<dyn Fn(SettingsTab, &mut Window, &mut App) + 'static>,
    pub content: AnyElement,
}

impl SettingsShell {
    pub fn new(
        active_tab: SettingsTab,
        on_select_tab: Rc<dyn Fn(SettingsTab, &mut Window, &mut App) + 'static>,
        content: impl IntoElement,
    ) -> Self {
        Self {
            active_tab,
            on_select_tab,
            content: content.into_any_element(),
        }
    }
}

impl RenderOnce for SettingsShell {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_select = self.on_select_tab.clone();
        let tabs = [
            (SettingsTab::Accounts, IconName::User, "Accounts"),
            (SettingsTab::Connection, IconName::Server, "Connection"),
            (SettingsTab::Usage, IconName::ChartColumn, "Usage"),
            (SettingsTab::Projects, IconName::Folder, "Projects"),
            (
                SettingsTab::DeletedChats,
                IconName::TrashBinMinimalistic,
                "Deleted chats",
            ),
        ];

        div()
            .flex()
            .size_full()
            .bg(theme.canvas)
            .text_color(theme.text)
            // Left sidebar for navigation
            .child(
                div()
                    .w(px(210.0))
                    .h_full()
                    .border_r_1()
                    .border_color(theme.border)
                    .bg(theme.sidebar)
                    .flex()
                    .flex_col()
                    .justify_between()
                    .pt(px(42.0))
                    .pb(px(16.0))
                    .px(px(12.0))
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(10.0))
                            .child(
                                div()
                                    .px(px(8.0))
                                    .py(px(4.0))
                                    .text_size(px(14.0))
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .text_color(theme.text)
                                    .child("Settings"),
                            )
                            .child(div().flex().flex_col().gap(px(2.0)).children(
                                tabs.into_iter().map(|(tab, icon_name, label)| {
                                    let is_active = self.active_tab == tab;
                                    let on_tab_click = on_select.clone();
                                    let tab_id = format!("settings-tab-{:?}", tab);

                                    div()
                                        .id(ElementId::from(tab_id))
                                        .px(px(8.0))
                                        .py(px(6.0))
                                        .rounded(px(6.0))
                                        .cursor_pointer()
                                        .bg(if is_active {
                                            theme.accent
                                        } else {
                                            gpui::transparent_black()
                                        })
                                        .hover(|s| if !is_active { s.bg(theme.overlay) } else { s })
                                        .active(|s| {
                                            if !is_active {
                                                s.bg(theme.overlay_strong)
                                            } else {
                                                s
                                            }
                                        })
                                        .on_mouse_down(
                                            MouseButton::Left,
                                            move |_event, window, cx| {
                                                cx.stop_propagation();
                                                (on_tab_click)(tab, window, cx);
                                            },
                                        )
                                        .flex()
                                        .items_center()
                                        .gap(px(8.0))
                                        .child(app_icon(
                                            icon_name,
                                            14.0,
                                            if is_active {
                                                theme.on_inverse
                                            } else {
                                                theme.text_secondary
                                            },
                                        ))
                                        .child(
                                            div()
                                                .text_size(px(13.0))
                                                .font_weight(if is_active {
                                                    gpui::FontWeight::MEDIUM
                                                } else {
                                                    gpui::FontWeight::NORMAL
                                                })
                                                .text_color(if is_active {
                                                    theme.on_inverse
                                                } else {
                                                    theme.text
                                                })
                                                .child(label),
                                        )
                                }),
                            )),
                    ),
            )
            // Right content container
            .child(
                div()
                    .id("settings-content-scroll")
                    .flex_1()
                    .h_full()
                    .overflow_y_scroll()
                    .pt(px(42.0))
                    .pb(px(24.0))
                    .px(px(24.0))
                    .child(self.content),
            )
    }
}
