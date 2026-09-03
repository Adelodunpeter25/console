use super::{EnvironmentRow, ProbeState};
use crate::input::ComposerInput;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct ConnectionPage {
    pub environments: Vec<EnvironmentRow>,
    pub is_adding: bool,
    pub is_editing: bool,
    pub name_input: Option<Entity<ComposerInput>>,
    pub url_input: Option<Entity<ComposerInput>>,
    pub new_probe_state: ProbeState,
    pub on_activate: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_probe: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_edit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_remove: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_toggle_add: Rc<dyn Fn(bool, &mut Window, &mut App) + 'static>,
    pub on_probe_new: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_save_new: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for ConnectionPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_activate = self.on_activate.clone();
        let on_probe = self.on_probe.clone();
        let on_edit = self.on_edit.clone();
        let on_remove = self.on_remove.clone();
        let on_toggle_add = self.on_toggle_add.clone();
        let on_probe_new = self.on_probe_new.clone();
        let on_save_new = self.on_save_new.clone();

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .items_center()
                    .justify_between()
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
                                    .child("Server Environments"),
                            )
                            .child(
                                div()
                                    .text_size(px(12.5))
                                    .text_color(theme.text_secondary)
                                    .child("Configure backend servers, local daemons, and endpoints."),
                            ),
                    )
                    .when(!self.is_adding, |el| {
                        let on_add = on_toggle_add.clone();
                        el.child(
                            div()
                                .id("btn-add-environment")
                                .px(px(10.0))
                                .py(px(5.0))
                                .rounded(px(6.0))
                                .bg(theme.accent)
                                .flex()
                                .items_center()
                                .gap(px(6.0))
                                .cursor_pointer()
                                .hover(|s| s.opacity(0.9))
                                .active(|s| s.opacity(0.75))
                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                    cx.stop_propagation();
                                    (on_add)(true, window, cx);
                                })
                                .child(app_icon(IconName::Plus, 13.0, theme.on_inverse))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.on_inverse)
                                        .child("Add Server"),
                                ),
                        )
                    }),
            )
            // Inline add/edit form
            .when(self.is_adding, |el| {
                let (probe_text, probe_color) = match self.new_probe_state {
                    ProbeState::Ok => ("Connected successfully", theme.accent),
                    ProbeState::Failed => ("Could not reach server", theme.danger),
                    ProbeState::Probing => ("Testing connection...", theme.accent),
                    ProbeState::Unknown => ("Click Test to verify connection", theme.text_ghost),
                };

                let on_cancel = on_toggle_add.clone();
                let on_prb = on_probe_new.clone();
                let on_save = on_save_new.clone();

                el.child(
                    div()
                        .p(px(14.0))
                        .rounded(px(8.0))
                        .border_1()
                        .border_color(theme.accent)
                        .bg(theme.surface)
                        .flex()
                        .flex_col()
                        .gap(px(12.0))
                        .child(
                            div()
                                .text_size(px(13.5))
                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                .text_color(theme.text)
                                .child(if self.is_editing {
                                    "Edit Server Environment"
                                } else {
                                    "Add New Server Environment"
                                }),
                        )
                        .child(
                            div()
                                .flex()
                                .flex_col()
                                .gap(px(8.0))
                                .child(
                                    div()
                                        .flex()
                                        .flex_col()
                                        .gap(px(4.0))
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text_secondary)
                                                .child("Environment Name"),
                                        )
                                        .when_some(self.name_input, |d, input| {
                                            d.child(
                                                div()
                                                    .p(px(6.0))
                                                    .rounded(px(6.0))
                                                    .border_1()
                                                    .border_color(theme.border)
                                                    .bg(theme.canvas)
                                                    .child(input),
                                            )
                                        }),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .flex_col()
                                        .gap(px(4.0))
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text_secondary)
                                                .child("Server URL (e.g. http://localhost:3000 or http://192.168.1.50:3000)"),
                                        )
                                        .when_some(self.url_input, |d, input| {
                                            d.child(
                                                div()
                                                    .p(px(6.0))
                                                    .rounded(px(6.0))
                                                    .border_1()
                                                    .border_color(theme.border)
                                                    .bg(theme.canvas)
                                                    .child(input),
                                            )
                                        }),
                                )
                        )
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .justify_between()
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .gap(px(6.0))
                                        .child(
                                            div()
                                                .w(px(7.0))
                                                .h(px(7.0))
                                                .rounded_full()
                                                .bg(probe_color),
                                        )
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .text_color(probe_color)
                                                .child(probe_text),
                                        ),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .gap(px(8.0))
                                        .child(
                                            div()
                                                .px(px(8.0))
                                                .py(px(4.0))
                                                .rounded(px(5.0))
                                                .border_1()
                                                .border_color(theme.border_strong)
                                                .bg(theme.raised)
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_prb)(window, cx);
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .text_color(theme.text)
                                                        .child("Test Connection"),
                                                ),
                                        )
                                        .child(
                                            div()
                                                .px(px(8.0))
                                                .py(px(4.0))
                                                .rounded(px(5.0))
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_cancel)(false, window, cx);
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .text_color(theme.text_secondary)
                                                        .child("Cancel"),
                                                ),
                                        )
                                        .child(
                                            div()
                                                .px(px(10.0))
                                                .py(px(4.0))
                                                .rounded(px(5.0))
                                                .bg(theme.accent)
                                                .cursor_pointer()
                                                .hover(|s| s.opacity(0.9))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_save)(window, cx);
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.on_inverse)
                                                        .child("Save"),
                                                ),
                                        ),
                                ),
                        ),
                )
            })
            // Existing environments list
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(10.0))
                    .children(self.environments.into_iter().map(|env| {
                        let env_id_for_activate = env.id.clone();
                        let env_id_for_probe = env.id.clone();
                        let env_id_for_edit = env.id.clone();
                        let env_id_for_remove = env.id.clone();
                        let on_act = on_activate.clone();
                        let on_prb = on_probe.clone();
                        let on_edt = on_edit.clone();
                        let on_rem = on_remove.clone();

                        let (probe_text, probe_color) = match env.probe_state {
                            ProbeState::Ok => ("Reachable", theme.accent),
                            ProbeState::Failed => ("Unreachable", theme.danger),
                            ProbeState::Probing => ("Checking...", theme.accent),
                            ProbeState::Unknown => ("Not tested", theme.text_ghost),
                        };

                        div()
                            .id(ElementId::from(format!("env-row-{}", env.id)))
                            .p(px(12.0))
                            .rounded(px(8.0))
                            .border_1()
                            .border_color(if env.is_active { theme.accent } else { theme.border })
                            .bg(theme.surface)
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
                                            .bg(probe_color),
                                    )
                                    .child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .gap(px(2.0))
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_center()
                                                    .gap(px(8.0))
                                                    .child(
                                                        div()
                                                            .text_size(px(13.5))
                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                            .text_color(theme.text)
                                                            .child(env.name),
                                                    )
                                                    .when(env.is_active, |el| {
                                                        el.child(
                                                            div()
                                                                .px(px(6.0))
                                                                .py(px(1.0))
                                                                .rounded(px(4.0))
                                                                .bg(theme.accent)
                                                                .text_size(px(10.5))
                                                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                                                .text_color(theme.on_inverse)
                                                                .child("ACTIVE"),
                                                        )
                                                    }),
                                            )
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_center()
                                                    .gap(px(8.0))
                                                    .child(
                                                        div()
                                                            .text_size(px(11.5))
                                                            .text_color(theme.text_tertiary)
                                                            .child(env.url),
                                                    )
                                                    .child(
                                                        div()
                                                            .text_size(px(11.5))
                                                            .text_color(probe_color)
                                                            .child(format!("· {probe_text}")),
                                                    ),
                                            ),
                                    ),
                            )
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(6.0))
                                    .child(
                                        div()
                                            .id(ElementId::from(format!("btn-probe-{}", env_id_for_probe)))
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(5.0))
                                            .border_1()
                                            .border_color(theme.border_strong)
                                            .bg(theme.raised)
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay))
                                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                cx.stop_propagation();
                                                (on_prb)(env_id_for_probe.clone(), window, cx);
                                            })
                                            .child(
                                                div()
                                                    .text_size(px(11.5))
                                                    .text_color(theme.text_secondary)
                                                    .child("Test"),
                                            ),
                                    )
                                    .child(
                                        div()
                                            .id(ElementId::from(format!("btn-edit-{}", env_id_for_edit)))
                                            .px(px(8.0))
                                            .py(px(4.0))
                                            .rounded(px(5.0))
                                            .border_1()
                                            .border_color(theme.border_strong)
                                            .bg(theme.raised)
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay))
                                            .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                cx.stop_propagation();
                                                (on_edt)(env_id_for_edit.clone(), window, cx);
                                            })
                                            .child(
                                                div()
                                                    .text_size(px(11.5))
                                                    .text_color(theme.text_secondary)
                                                    .child("Edit"),
                                            ),
                                    )
                                    .when(!env.is_active, |el| {
                                        let env_id_activate = env_id_for_activate.clone();
                                        let on_act_click = on_act.clone();
                                        el.child(
                                            div()
                                                .id(ElementId::from(format!("btn-activate-{}", env_id_activate)))
                                                .px(px(8.0))
                                                .py(px(4.0))
                                                .rounded(px(5.0))
                                                .border_1()
                                                .border_color(theme.border_strong)
                                                .bg(theme.raised)
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_act_click)(env_id_activate.clone(), window, cx);
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .text_color(theme.text)
                                                        .child("Use"),
                                                ),
                                        )
                                    })
                                    .when(!env.is_active, |el| {
                                        let env_id_remove = env_id_for_remove.clone();
                                        let on_rem_click = on_rem.clone();
                                        el.child(
                                            div()
                                                .id(ElementId::from(format!("btn-remove-{}", env_id_remove)))
                                                .px(px(6.0))
                                                .py(px(4.0))
                                                .rounded(px(5.0))
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay_strong))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_rem_click)(env_id_remove.clone(), window, cx);
                                                })
                                                .child(app_icon(IconName::TrashBinMinimalistic, 13.0, theme.text_ghost)),
                                        )
                                    }),
                            )
                    })),
            )
    }
}
