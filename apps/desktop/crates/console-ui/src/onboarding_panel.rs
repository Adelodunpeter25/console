use crate::theme::Theme;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

/// Live setup status behind the three steps. Mirrors
/// `crate::state::onboarding::SetupStatus`, which cannot be named from here —
/// `console-ui` is a library crate that must not depend on the app binary.
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

/// The first-run setup panel for one server.
///
/// Shown once per server, on boot or on switching to a server that has never
/// had it, and dismissed forever after. Everything it displays is read live
/// from the server, so it reflects the server's actual state at the moment it
/// opens rather than anything remembered.
///
/// Non-blocking by construction: there is no backdrop that swallows input and
/// no way to be trapped here. A server with no project yet is a legitimate
/// state, not an error, so the panel points at the three setup steps and gets
/// out of the way.
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

/// One setup step: a label, live done/not-done state, and the button that does
/// it. Rendered by [`step_row`].
struct Step {
    title: &'static str,
    detail: &'static str,
    done: bool,
    action: &'static str,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for OnboardingPanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let dismiss = self.on_dismiss.clone();

        let steps = vec![
            Step {
                title: "Add a project",
                detail: "A project is the folder Console works in.",
                done: self.status.has_project,
                action: "Add project",
                on_click: self.on_add_project.clone(),
            },
            Step {
                title: "Sign in to an AI provider",
                detail: "Needed before a run can talk to a model.",
                done: self.status.has_provider,
                action: "Open providers",
                on_click: self.on_open_providers.clone(),
            },
            Step {
                title: "Add a GitHub key",
                detail: "Lets the server clone your private repositories.",
                done: self.status.has_github,
                action: "Open account",
                on_click: self.on_open_github.clone(),
            },
        ];

        let all_done = self.status.is_complete();

        div()
            .absolute()
            .top_0()
            .left_0()
            .right_0()
            .bottom_0()
            .flex()
            .justify_center()
            .items_center()
            .p(px(24.0))
            .child(
                div()
                    .w(px(520.0))
                    .max_w(px(520.0))
                    .rounded(px(12.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .p(px(20.0))
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("Set up this server"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child(format!(
                                "{} — none of these are required to get started.",
                                self.server_name
                            )),
                    )
                    .child(div().h(px(10.0)))
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(8.0))
                            .children(steps.into_iter().map(|step| {
                                step_row(step, self.loading, &theme)
                            })),
                    )
                    .child(div().h(px(14.0)))
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .justify_between()
                            // When every step is already satisfied there is
                            // nothing to do, so the panel says so rather than
                            // showing three "do this" buttons for work already
                            // done.
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .text_color(theme.text_tertiary)
                                    .child(if all_done {
                                        "This server is ready to use."
                                    } else {
                                        "Skip for now — you can do this any time in Settings."
                                    }),
                            )
                            .child({
                                let on_dismiss = dismiss.clone();
                                div()
                                    .id(ElementId::from("onboarding-dismiss"))
                                    .px(px(10.0))
                                    .py(px(5.0))
                                    .rounded(px(6.0))
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.raised))
                                    .child(
                                        div()
                                            .text_size(px(12.0))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.text)
                                            .child("Got it"),
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

/// One row: status dot, title, detail, and the action button.
///
/// A completed step's button is replaced by a quiet "Done" rather than left
/// clickable — re-running "add a project" from here would be a no-op the user
/// would have to discover for themselves.
fn step_row(step: Step, loading: bool, theme: &Theme) -> impl IntoElement {
    let on_click = step.on_click.clone();
    div()
        .p(px(10.0))
        .rounded(px(8.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.canvas)
        .flex()
        .items_center()
        .gap(px(10.0))
        .child(
            div()
                .w(px(8.0))
                .h(px(8.0))
                .flex_shrink_0()
                .rounded_full()
                .bg(if step.done { theme.success } else { theme.text_ghost }),
        )
        .child(
            div()
                .flex_1()
                .flex()
                .flex_col()
                .gap(px(1.0))
                .child(
                    div()
                        .text_size(px(13.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text)
                        .child(step.title),
                )
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_tertiary)
                        .child(step.detail),
                ),
        )
        // While the three checks are in flight the state is unknown, not false. A
        // grey dot and a live button here would read as "not done" for a server
        // that may well be fully set up, and invite a click that does nothing
        // while the answer is already on its way.
        .child(match (step.done, loading) {
            (true, _) => div()
                .flex_shrink_0()
                .text_size(px(11.5))
                .text_color(theme.success)
                .child("Done")
                .into_any_element(),
            (false, true) => div()
                .flex_shrink_0()
                .text_size(px(11.5))
                .text_color(theme.text_ghost)
                .child("Checking…")
                .into_any_element(),
            (false, false) => div()
                .id(ElementId::from(format!("onboarding-step-{}", step.title)))
                .flex_shrink_0()
                .px(px(10.0))
                .py(px(5.0))
                .rounded(px(6.0))
                .border_1()
                .border_color(theme.border_strong)
                .bg(theme.raised)
                .cursor_pointer()
                .hover(|s| s.bg(theme.overlay))
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text)
                        .child(step.action),
                )
                .on_mouse_down(MouseButton::Left, move |_e, w, cx| {
                    cx.stop_propagation();
                    (on_click)(w, cx);
                })
                .into_any_element(),
        })
        .into_any_element()
}