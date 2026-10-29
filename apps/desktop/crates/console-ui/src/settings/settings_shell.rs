use super::SettingsTab;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

/// The width of the settings navigation column.
pub const SETTINGS_NAV_WIDTH: f32 = 220.0;

struct SettingsGroup {
    title: &'static str,
    tabs: &'static [(SettingsTab, IconName, &'static str)],
}

/// The settings are grouped the same way the phone app groups them, so the
/// two clients read as one product: the same names, in the same places.
const SETTINGS_GROUPS: &[SettingsGroup] = &[
    SettingsGroup {
        title: "Connections",
        tabs: &[
            (SettingsTab::Servers, IconName::Server, "Servers"),
            (SettingsTab::Providers, IconName::CircleUser, "Providers"),
            (SettingsTab::Usage, IconName::ChartColumn, "Usage"),
        ],
    },
    SettingsGroup {
        title: "AI & tools",
        tabs: &[
            (SettingsTab::Models, IconName::Brain, "Models"),
            (SettingsTab::Integrations, IconName::Package, "Integrations"),
            (SettingsTab::Mcp, IconName::Cpu, "MCP Servers"),
        ],
    },
    SettingsGroup {
        title: "Projects & chats",
        tabs: &[
            (SettingsTab::Projects, IconName::Folder, "Projects"),
            (
                SettingsTab::DeletedChats,
                IconName::TrashBinMinimalistic,
                "Deleted Chats",
            ),
        ],
    },
    // The phone has no keyboard, so it has no shortcuts page. The desktop
    // keeps it in a group of its own rather than filing it under one of the
    // others, where it would not belong.
    SettingsGroup {
        title: "Preferences",
        tabs: &[(SettingsTab::Keybindings, IconName::Keyboard, "Shortcuts")],
    },
];

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

        div()
            .flex()
            .size_full()
            .bg(theme.canvas)
            .text_color(theme.text)
            // Left navigation sidebar
            .child(
                div()
                    .w(px(SETTINGS_NAV_WIDTH))
                    .flex_shrink_0()
                    .h_full()
                    .border_r_1()
                    .border_color(theme.sidebar_border)
                    .bg(theme.sidebar)
                    .flex()
                    .flex_col()
                    .justify_between()
                    .pt(px(16.0))
                    .pb(px(16.0))
                    .px(px(12.0))
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(16.0))
                            // Sidebar Header
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(8.0))
                                    .px(px(8.0))
                                    .py(px(4.0))
                                    .child(app_icon(IconName::Settings, 16.0, theme.text))
                                    .child(
                                        div()
                                            .text_size(px(14.0))
                                            .font_weight(FontWeight::SEMIBOLD)
                                            .text_color(theme.text)
                                            .child("Settings"),
                                    ),
                            )
                            // Navigation Groups
                            .child(div().flex().flex_col().gap(px(12.0)).children(
                                SETTINGS_GROUPS.iter().map(|group| {
                                    div()
                                        .flex()
                                        .flex_col()
                                        .gap(px(2.0))
                                        .child(
                                            div()
                                                .px(px(8.0))
                                                .pt(px(4.0))
                                                .pb(px(2.0))
                                                .text_size(px(10.5))
                                                .font_weight(FontWeight::SEMIBOLD)
                                                .text_color(theme.text_ghost)
                                                .child(group.title),
                                        )
                                        .children(group.tabs.iter().map(
                                            |(tab, icon_name, label)| {
                                                let is_active = self.active_tab == *tab;
                                                let on_tab_click = on_select.clone();
                                                let tab_id = format!("settings-tab-{:?}", tab);
                                                let tab_val = *tab;

                                                div()
                                                    .id(ElementId::from(tab_id))
                                                    .px(px(8.0))
                                                    .py(px(6.0))
                                                    .rounded(px(6.0))
                                                    .cursor_pointer()
                                                    .bg(if is_active {
                                                        theme.surface
                                                    } else {
                                                        gpui::transparent_black()
                                                    })
                                                    .hover(|s| {
                                                        if !is_active {
                                                            s.bg(theme.overlay)
                                                        } else {
                                                            s
                                                        }
                                                    })
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
                                                            (on_tab_click)(tab_val, window, cx);
                                                        },
                                                    )
                                                    .flex()
                                                    .items_center()
                                                    .justify_between()
                                                    .child(
                                                        div()
                                                            .flex()
                                                            .items_center()
                                                            .gap(px(8.0))
                                                            .child(app_icon(
                                                                *icon_name,
                                                                13.5,
                                                                if is_active {
                                                                    theme.accent
                                                                } else {
                                                                    theme.text_secondary
                                                                },
                                                            ))
                                                            .child(
                                                                div()
                                                                    .text_size(px(12.5))
                                                                    .font_weight(if is_active {
                                                                        FontWeight::MEDIUM
                                                                    } else {
                                                                        FontWeight::NORMAL
                                                                    })
                                                                    .text_color(if is_active {
                                                                        theme.text
                                                                    } else {
                                                                        theme.text_secondary
                                                                    })
                                                                    .child(*label),
                                                            ),
                                                    )
                                                    .when(is_active, |el| {
                                                        el.child(
                                                            div()
                                                                .w(px(2.5))
                                                                .h(px(12.0))
                                                                .rounded(px(1.5))
                                                                .bg(theme.accent),
                                                        )
                                                    })
                                            },
                                        ))
                                }),
                            )),
                    ),
            )
            // Right content container with readable max-width and comfortable padding
            .child(
                div()
                    .id("settings-content-scroll")
                    .flex_1()
                    .min_w_0()
                    .h_full()
                    .overflow_y_scroll()
                    .child(
                        div()
                            .w_full()
                            .max_w(px(860.0))
                            .mx_auto()
                            .pt(px(24.0))
                            .pb(px(40.0))
                            .px(px(32.0))
                            .child(self.content),
                    ),
            )
    }
}
