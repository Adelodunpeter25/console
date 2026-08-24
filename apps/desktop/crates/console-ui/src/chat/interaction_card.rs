use crate::input::ComposerInput;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use console_core::{AskQuestionRequest, PermissionRequest};
use gpui::{
    App, ElementId, Entity, FontWeight, IntoElement, MouseButton, ParentElement, RenderOnce,
    Styled, Window, div, prelude::*, px,
};
use std::collections::HashSet;
use std::rc::Rc;

#[derive(IntoElement)]
pub struct PermissionInteractionCard {
    pub request: PermissionRequest,
    pub submitting: bool,
    on_approve: Rc<dyn Fn(bool, &mut Window, &mut App) + 'static>,
}

impl PermissionInteractionCard {
    pub fn new(
        request: PermissionRequest,
        submitting: bool,
        on_approve: impl Fn(bool, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            request,
            submitting,
            on_approve: Rc::new(on_approve),
        }
    }
}

impl RenderOnce for PermissionInteractionCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let req = self.request.clone();
        let requires_upgrade = req.requires_upgrade.unwrap_or(false);
        let on_app = self.on_approve;

        div()
            .w_full()
            .max_w(px(768.0))
            .p(px(14.0))
            .rounded(px(12.0))
            .bg(theme.surface)
            .border_1()
            .border_color(theme.warning.opacity(0.4))
            .shadow_lg()
            .flex()
            .flex_col()
            .gap(px(10.0))
            // Header
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child(app_icon(IconName::TriangleAlert, 14.0, theme.warning))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child(if requires_upgrade {
                                format!("Upgrade required: {}", req.tool_name)
                            } else {
                                format!("Permission required: {}", req.tool_name)
                            }),
                    ),
            )
            // Optional reason
            .when_some(req.reason.clone(), |el, reason| {
                el.child(
                    div()
                        .pl(px(24.0))
                        .text_size(px(12.0))
                        .text_color(theme.text_secondary)
                        .child(reason),
                )
            })
            // Tool args JSON preview
            .child(
                div()
                    .id("permission-args-preview")
                    .ml(px(24.0))
                    .max_h(px(140.0))
                    .p(px(8.0))
                    .rounded(px(6.0))
                    .bg(theme.inset)
                    .border_1()
                    .border_color(theme.border)
                    .overflow_y_scroll()
                    .font_family("GeistMono")
                    .text_size(px(11.0))
                    .text_color(theme.text_tertiary)
                    .child(serde_json::to_string_pretty(&req.args).unwrap_or_default()),
            )
            // Actions
            .child(
                div()
                    .pl(px(24.0))
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    // Allow button
                    .child({
                        let on_a = on_app.clone();
                        div()
                            .id("permission-allow")
                            .h(px(28.0))
                            .px(px(12.0))
                            .rounded(px(6.0))
                            .bg(theme.success.opacity(0.18))
                            .border_1()
                            .border_color(theme.success.opacity(0.35))
                            .cursor_default()
                            .hover(|s| s.bg(theme.success.opacity(0.28)))
                            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                (on_a)(true, window, cx);
                            })
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(IconName::Check, 11.0, theme.success))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(theme.success)
                                    .child(if requires_upgrade {
                                        "Allow once"
                                    } else {
                                        "Allow"
                                    }),
                            )
                    })
                    // Deny button
                    .child({
                        let on_a = on_app;
                        div()
                            .id("permission-deny")
                            .h(px(28.0))
                            .px(px(12.0))
                            .rounded(px(6.0))
                            .bg(theme.danger_soft)
                            .border_1()
                            .border_color(theme.danger.opacity(0.35))
                            .cursor_default()
                            .hover(|s| s.bg(theme.danger.opacity(0.22)))
                            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                (on_a)(false, window, cx);
                            })
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(IconName::X, 11.0, theme.danger))
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(theme.danger)
                                    .child("Deny"),
                            )
                    }),
            )
    }
}

// ──────────────────────────────────────────────────────────────────────────────

#[derive(IntoElement)]
pub struct QuestionInteractionCard {
    pub request: AskQuestionRequest,
    pub selected: HashSet<String>,
    pub custom_answer: String,
    pub custom_input: Option<Entity<ComposerInput>>,
    pub is_last: bool,
    pub submitting: bool,
    on_answer: Rc<dyn Fn(serde_json::Value, &mut Window, &mut App) + 'static>,
    on_select: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_skip: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl QuestionInteractionCard {
    pub fn new(
        request: AskQuestionRequest,
        is_last: bool,
        submitting: bool,
        on_answer: impl Fn(serde_json::Value, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            request,
            selected: HashSet::new(),
            custom_answer: String::new(),
            custom_input: None,
            is_last,
            submitting,
            on_answer: Rc::new(on_answer),
            on_select: Rc::new(|_, _, _| {}),
            on_skip: None,
        }
    }

    pub fn selected(mut self, selected: HashSet<String>) -> Self {
        self.selected = selected;
        self
    }

    pub fn on_select(mut self, handler: impl Fn(String, &mut Window, &mut App) + 'static) -> Self {
        self.on_select = Rc::new(handler);
        self
    }

    pub fn custom_input(mut self, input: Entity<ComposerInput>) -> Self {
        self.custom_input = Some(input);
        self
    }

    pub fn on_skip(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_skip = Some(Rc::new(handler));
        self
    }
}

impl RenderOnce for QuestionInteractionCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let req = self.request.clone();
        let is_multi = req.is_multi_select.unwrap_or(false);
        let options = req.options.clone().unwrap_or_default();
        let on_select = self.on_select.clone();
        let custom_input = self.custom_input.clone();
        let custom_answer = custom_input
            .as_ref()
            .map(|input| input.read(cx).content().trim().to_owned())
            .filter(|answer| !answer.is_empty())
            .unwrap_or_else(|| self.custom_answer.trim().to_owned());
        let has_answer = !custom_answer.is_empty() || !self.selected.is_empty();

        div()
            .w_full()
            .max_w(px(768.0))
            .p(px(14.0))
            .rounded(px(12.0))
            .bg(theme.surface)
            .border_1()
            .border_color(theme.accent.opacity(0.35))
            .shadow_lg()
            .flex()
            .flex_col()
            .gap(px(10.0))
            // Question header
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child(app_icon(IconName::Bot, 14.0, theme.accent))
                    .child(
                        div()
                            .text_size(px(13.5))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child(req.question.clone()),
                    ),
            )
            // Options
            .when(!options.is_empty(), |el| {
                el.child(div().pl(px(24.0)).flex().flex_col().gap(px(4.0)).children(
                    options.into_iter().enumerate().map(|(idx, opt)| {
                        let is_checked = self.selected.contains(&opt);
                        let option = opt.clone();
                        let on_select = on_select.clone();
                        div()
                            .id(ElementId::Integer(idx as u64))
                            .px(px(10.0))
                            .py(px(7.0))
                            .rounded(px(6.0))
                            .cursor_default()
                            .bg(if is_checked {
                                theme.accent.opacity(0.12)
                            } else {
                                theme.raised
                            })
                            .border_1()
                            .border_color(if is_checked {
                                theme.accent.opacity(0.4)
                            } else {
                                theme.border
                            })
                            .flex()
                            .items_center()
                            .gap(px(8.0))
                            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                (on_select)(option.clone(), window, cx);
                            })
                            .child(
                                div()
                                    .size(px(14.0))
                                    .rounded(if is_multi { px(3.0) } else { px(7.0) })
                                    .border_1()
                                    .border_color(if is_checked {
                                        theme.accent
                                    } else {
                                        theme.text_ghost
                                    })
                                    .bg(if is_checked {
                                        theme.accent
                                    } else {
                                        gpui::transparent_black()
                                    })
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .when(is_checked, |b| {
                                        b.child(app_icon(IconName::Check, 9.0, theme.on_inverse))
                                    }),
                            )
                            .child(div().text_size(px(12.5)).text_color(theme.text).child(opt))
                    }),
                ))
            })
            // A custom answer is always available, including when options are
            // present. This matches the desktop question panel and lets users
            // answer values that the model did not pre-populate.
            .when_some(custom_input, |el, input| {
                el.child(
                    div()
                        .pl(px(24.0))
                        .h(px(32.0))
                        .rounded(px(7.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.inset)
                        .px(px(8.0))
                        .child(input),
                )
            })
            // Footer actions
            .child(
                div()
                    .pl(px(24.0))
                    .pt(px(4.0))
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .child({
                        let on_ans = self.on_answer;
                        let answer_val = if !custom_answer.is_empty() {
                            serde_json::Value::String(custom_answer)
                        } else if is_multi {
                            serde_json::to_value(self.selected.into_iter().collect::<Vec<_>>())
                                .unwrap_or_default()
                        } else {
                            serde_json::to_value(
                                self.selected.into_iter().next().unwrap_or_default(),
                            )
                            .unwrap_or_default()
                        };
                        div()
                            .id("question-submit")
                            .h(px(28.0))
                            .px(px(14.0))
                            .rounded(px(6.0))
                            .bg(if has_answer && !self.submitting {
                                theme.inverse
                            } else {
                                theme.overlay_strong
                            })
                            .when(has_answer && !self.submitting, |el| {
                                el.cursor_default()
                                    .hover(|s| s.opacity(0.85))
                                    .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                        (on_ans)(answer_val.clone(), window, cx);
                                    })
                            })
                            .flex()
                            .items_center()
                            .justify_center()
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(FontWeight::SEMIBOLD)
                                    .text_color(theme.on_inverse)
                                    .child(if self.is_last { "Submit" } else { "Continue" }),
                            )
                    })
                    .when_some(self.on_skip, |el, on_skip| {
                        el.child(
                            div()
                                .id("question-skip")
                                .h(px(28.0))
                                .px(px(12.0))
                                .rounded(px(6.0))
                                .cursor_default()
                                .hover(|s| s.bg(theme.overlay))
                                .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                                    (on_skip)(window, cx)
                                })
                                .flex()
                                .items_center()
                                .justify_center()
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .text_color(theme.text_tertiary)
                                        .child("Skip"),
                                ),
                        )
                    }),
            )
    }
}
