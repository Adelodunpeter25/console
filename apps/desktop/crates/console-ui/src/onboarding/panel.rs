use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

/// Live setup status behind the steps. Mirrors
/// `crate::state::onboarding::SetupStatus`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SetupStatus {
    pub has_project: bool,
    pub has_provider: bool,
    pub has_github: bool,
}

impl SetupStatus {
    pub fn is_complete(&self) -> bool {
        self.has_project && self.has_provider && self.has_github
    }
}

/// Full-window first-run setup for one server.
///
/// Fills the main window outright. The window renders this *instead of* the
/// workspace rather than on top of it, which is what keeps the sidebar and title
/// bar out of the way.
#[derive(IntoElement)]
pub struct OnboardingPanel {
    pub server_name: String,
    pub status: SetupStatus,
    pub loading: bool,
    pub on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_open_providers: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_open_github: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_dismiss: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

/// One setup step.
struct Step {
    icon: IconName,
    title: &'static str,
    detail: &'static str,
    done: bool,
    action: &'static str,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for OnboardingPanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let steps = vec![
            Step {
                icon: IconName::Folder,
                title: "Add a project",
                detail: "A project is the folder Console works in. Pick one to get going.",
                done: self.status.has_project,
                action: "Add project",
                on_click: self.on_add_project.clone(),
            },
            Step {
                icon: IconName::KeyMinimalistic,
                title: "Sign in to an AI provider",
                detail: "Needed before a run can talk to a model. Paste a key or connect an account.",
                done: self.status.has_provider,
                action: "Open providers",
                on_click: self.on_open_providers.clone(),
            },
            Step {
                icon: IconName::Check,
                title: "Add a GitHub key",
                detail: "Lets the server clone your private repositories on your behalf.",
                done: self.status.has_github,
                action: "Open account",
                on_click: self.on_open_github.clone(),
            },
        ];

        let done_count = [self.status.has_project, self.status.has_provider, self.status.has_github]
            .iter()
            .filter(|d| **d)
            .count();

        div()
            .id("onboarding-scroll")
            .size_full()
            .bg(theme.canvas)
            .flex()
            .justify_center()
            .items_start()
            .overflow_y_scroll()
            .child(
                div()
                    .w(px(560.0))
                    .max_w(px(560.0))
                    .px(px(32.0))
                    .py(px(72.0))
                    .flex()
                    .flex_col()
                    // Heading
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.accent)
                            .child("GETTING STARTED"),
                    )
                    .child(
                        div()
                            .mt(px(10.0))
                            .text_size(px(28.0))
                            .font_weight(gpui::FontWeight::BOLD)
                            .text_color(theme.text)
                            .child(format!("Set up {}", self.server_name)),
                    )
                    .child(
                        div()
                            .mt(px(10.0))
                            .text_size(px(14.0))
                            .text_color(theme.text_secondary)
                            .child(
                                "Three things make a server useful. None of them are required to \
                                 start working — skip anything you do not need yet.",
                            ),
                    )
                    // Progress, so a half-configured server is legible at a glance
                    .child(
                        div()
                            .mt(px(24.0))
                            .h(px(4.0))
                            .rounded_full()
                            .bg(theme.raised)
                            .child(
                                div()
                                    .h(px(4.0))
                                    .rounded_full()
                                    .bg(theme.accent)
                                    // 0/3 reads as no bar; never negative.
                                    .w(px(if done_count == 0 {
                                        0.0
                                    } else {
                                        186.0 * (done_count as f32 / 3.0)
                                    })),
                            ),
                    )
                    .child(
                        div()
                            .mt(px(8.0))
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .child(if self.loading {
                                "Checking what is already set up…".to_string()
                            } else if self.status.is_complete() {
                                "All set — this server is ready to use.".to_string()
                            } else {
                                format!("{} of 3 done", done_count)
                            }),
                    )
                    .child(div().h(px(28.0)))
                    // Steps
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(10.0))
                            .children(steps.into_iter().map(|step| step_card(step, self.loading, &theme))),
                    )
                    .child(div().h(px(28.0)))
                    // Footer
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .justify_end()
                            .child({
                                let on_dismiss = self.on_dismiss.clone();
                                div()
                                    .id(ElementId::from("onboarding-dismiss"))
                                    .px(px(18.0))
                                    .py(px(9.0))
                                    .rounded(px(8.0))
                                    .bg(theme.raised)
                                    .border_1()
                                    .border_color(theme.border_strong)
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .child(
                                        div()
                                            .text_size(px(13.0))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.text)
                                            .child("Skip setup"),
                                    )
                                    .on_mouse_down(MouseButton::Left, move |_e, w, cx| {
                                        cx.stop_propagation();
                                        (on_dismiss)(w, cx);
                                    })
                            }),
                    ),
            )
            .into_any_element()
    }
}

/// One step as a full-width card.
fn step_card(step: Step, loading: bool, theme: &Theme) -> impl IntoElement {
    let on_click = step.on_click.clone();
    // While the three checks are in flight the state is unknown, not false. A
    // greyed card plus a live button would read as "not done" for a server that
    // may already be fully configured.
    let actionable = !step.done && !loading;

    div()
        .flex()
        .items_center()
        .gap(px(14.0))
        .p(px(16.0))
        .rounded(px(10.0))
        .border_1()
        .border_color(if step.done { theme.border } else { theme.border_strong })
        .bg(theme.surface)
        .child(
            div()
                .flex_shrink_0()
                .w(px(36.0))
                .h(px(36.0))
                .flex()
                .items_center()
                .justify_center()
                .rounded(px(8.0))
                .bg(if step.done { theme.raised } else { theme.overlay })
                .child(app_icon(
                    if step.done { IconName::Check } else { step.icon },
                    17.0,
                    if step.done { theme.success } else { theme.text_secondary },
                )),
        )
        .child(
            div()
                .flex_1()
                .flex()
                .flex_col()
                .gap(px(3.0))
                .child(
                    div()
                        .text_size(px(14.5))
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        // Done rows recede; pending rows are what the eye lands on.
                        .text_color(if step.done { theme.text_tertiary } else { theme.text })
                        .child(step.title),
                )
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_tertiary)
                        .child(step.detail),
                ),
        )
        .child(if step.done {
            div()
                .flex_shrink_0()
                .text_size(px(12.5))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(theme.success)
                .child("Done")
                .into_any_element()
        } else if loading {
            div()
                .flex_shrink_0()
                .text_size(px(12.5))
                .text_color(theme.text_ghost)
                .child("Checking…")
                .into_any_element()
        } else {
            div()
                .id(ElementId::from(format!("onboarding-step-{}", step.title)))
                .flex_shrink_0()
                .px(px(14.0))
                .py(px(8.0))
                .rounded(px(7.0))
                .bg(theme.accent)
                .cursor_pointer()
                .hover(|s| s.opacity(0.9))
                .child(
                    div()
                        .text_size(px(12.5))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.on_inverse)
                        .child(step.action),
                )
                .on_mouse_down(MouseButton::Left, move |_e, w, cx| {
                    cx.stop_propagation();
                    if actionable {
                        (on_click)(w, cx);
                    }
                })
                .into_any_element()
        })
        .into_any_element()
}