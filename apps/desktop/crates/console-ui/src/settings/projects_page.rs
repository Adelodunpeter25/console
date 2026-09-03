use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use console_core::types::ProjectInfo;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    Styled, Window, div, prelude::*, px,
};
use std::rc::Rc;

#[derive(IntoElement)]
pub struct ProjectsPage {
    pub projects: Rc<Vec<ProjectInfo>>,
    pub on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_remove_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl RenderOnce for ProjectsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_add = self.on_add_project.clone();
        let on_remove = self.on_remove_project.clone();

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
                                    .child("Active Projects"),
                            )
                            .child(
                                div()
                                    .text_size(px(12.5))
                                    .text_color(theme.text_secondary)
                                    .child(
                                        "Working directories accessible to local tools and agents.",
                                    ),
                            ),
                    )
                    .child(
                        div()
                            .id("btn-add-project")
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
                                (on_add)(window, cx);
                            })
                            .child(app_icon(IconName::Folder, 13.0, theme.on_inverse))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(gpui::FontWeight::MEDIUM)
                                    .text_color(theme.on_inverse)
                                    .child("Open Project"),
                            ),
                    ),
            )
            .child(if self.projects.is_empty() {
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
                    .child(app_icon(IconName::Folder, 24.0, theme.text_ghost))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .text_color(theme.text_secondary)
                            .child("No active projects found."),
                    )
            } else {
                div()
                    .flex()
                    .flex_col()
                    .gap(px(8.0))
                    .children(self.projects.iter().map(|proj| {
                        let proj_id = proj.id.clone();
                        let on_rem = on_remove.clone();

                        div()
                            .id(ElementId::from(format!("proj-row-{}", proj.id)))
                            .p(px(12.0))
                            .rounded(px(8.0))
                            .border_1()
                            .border_color(theme.border)
                            .bg(theme.surface)
                            .flex()
                            .items_center()
                            .justify_between()
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(10.0))
                                    .child(app_icon(IconName::Folder, 16.0, theme.accent))
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
                                                    .child(proj.name.clone()),
                                            )
                                            .child(
                                                div()
                                                    .text_size(px(11.5))
                                                    .text_color(theme.text_tertiary)
                                                    .child(proj.path.clone()),
                                            ),
                                    ),
                            )
                            .child(
                                div()
                                    .id(ElementId::from(format!("btn-remove-proj-{}", proj_id)))
                                    .px(px(6.0))
                                    .py(px(4.0))
                                    .rounded(px(5.0))
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay_strong))
                                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                        cx.stop_propagation();
                                        (on_rem)(proj_id.clone(), window, cx);
                                    })
                                    .child(app_icon(
                                        IconName::TrashBinMinimalistic,
                                        13.0,
                                        theme.text_ghost,
                                    )),
                            )
                    }))
            })
    }
}
