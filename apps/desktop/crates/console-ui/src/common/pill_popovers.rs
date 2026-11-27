//! Popover content for the composer's pill row: the card the tasks,
//! subagents, and shells pills open above themselves. The pills live in
//! `composer_pills`; this module only builds what shows inside the popover.

use std::rc::Rc;

use console_core::{Automation, SubagentInfo, TodoItem, WebhookSubscription};
use gpui::{AnyElement, ElementId, FontWeight, div, prelude::*, px};

use crate::common::composer_pills::{ActionHandler, ShellEntry, StopHandler};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// The floating task list that opens above the tasks pill: a compact card
/// with one truncated title per row and a status marker, capped so a long
/// checklist scrolls instead of covering the chat.
pub(crate) fn tasks_popover(items: Rc<[TodoItem]>, theme: Theme) -> AnyElement {
    let total = items.len();
    let completed = items
        .iter()
        .filter(|item| {
            matches!(
                item.status.to_ascii_lowercase().as_str(),
                "done" | "completed" | "complete"
            )
        })
        .count();

    div()
        .id("composer-tasks-popover")
        .w(px(300.0))
        .max_h(px(340.0))
        .rounded(px(12.0))
        .bg(theme.raised)
        .border_1()
        .border_color(theme.border_strong)
        .shadow_xl()
        .overflow_hidden()
        .flex()
        .flex_col()
        .child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .px(px(11.0))
                .py(px(8.0))
                .border_b_1()
                .border_color(theme.border)
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .child(app_icon(IconName::Checklist, 12.0, theme.text_secondary))
                        .child(
                            div()
                                .text_size(px(10.5))
                                .font_weight(FontWeight::BOLD)
                                .text_color(theme.text_secondary)
                                .child("TASKS"),
                        ),
                )
                .child(
                    div()
                        .text_size(px(10.5))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.text_tertiary)
                        .child(format!("{completed}/{total}")),
                ),
        )
        .child(
            div()
                .id("composer-tasks-popover-scroll")
                .max_h(px(300.0))
                .overflow_y_scroll()
                .on_scroll(|_, _, cx| cx.stop_propagation())
                .p(px(5.0))
                .flex()
                .flex_col()
                .gap(px(1.0))
                .children(items.iter().enumerate().map(|(index, item)| {
                    let status = item.status.to_ascii_lowercase();
                    let done = matches!(status.as_str(), "done" | "completed" | "complete");
                    let in_progress =
                        matches!(status.as_str(), "in_progress" | "started" | "running");

                    div()
                        .id(format!("composer-task-row-{index}"))
                        .w_full()
                        .min_w_0()
                        .flex()
                        .items_center()
                        .gap(px(8.0))
                        .rounded(px(6.0))
                        .px(px(6.0))
                        .py(px(6.0))
                        .hover(|style| style.bg(theme.overlay))
                        .child(
                            div()
                                .size(px(15.0))
                                .flex_none()
                                .rounded_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .border_1()
                                .when(done, |marker| {
                                    marker
                                        .bg(theme.success.opacity(0.16))
                                        .border_color(theme.success)
                                        .child(app_icon(IconName::Check, 9.0, theme.success))
                                })
                                .when(in_progress, |marker| {
                                    marker
                                        .bg(theme.accent.opacity(0.12))
                                        .border_color(theme.accent)
                                        .child(div().size(px(5.0)).rounded_full().bg(theme.accent))
                                })
                                .when(!done && !in_progress, |marker| {
                                    marker.border_color(theme.border_strong)
                                }),
                        )
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .truncate()
                                .text_size(px(12.5))
                                .text_color(if done {
                                    theme.text_tertiary
                                } else {
                                    theme.text
                                })
                                .child(item.content.clone()),
                        )
                })),
        )
        .into_any_element()
}

/// Shared chrome for the pill popovers: the tasks card's width, radius, and
/// header row with an icon-labelled title on the left and a counter on the
/// right.
pub(crate) fn pill_popover_card(
    id: &str,
    icon: IconName,
    title: &str,
    counter: String,
    scroll_id: &str,
    theme: Theme,
    rows: impl IntoIterator<Item = impl IntoElement>,
) -> AnyElement {
    div()
        .id(ElementId::Name(id.into()))
        .w(px(300.0))
        .max_h(px(340.0))
        .rounded(px(12.0))
        .bg(theme.raised)
        .border_1()
        .border_color(theme.border_strong)
        .shadow_xl()
        .overflow_hidden()
        .flex()
        .flex_col()
        .child(
            div()
                .flex()
                .items_center()
                .justify_between()
                .px(px(11.0))
                .py(px(8.0))
                .border_b_1()
                .border_color(theme.border)
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap(px(6.0))
                        .child(app_icon(icon, 12.0, theme.text_secondary))
                        .child(
                            div()
                                .text_size(px(10.5))
                                .font_weight(FontWeight::BOLD)
                                .text_color(theme.text_secondary)
                                .child(title.to_string()),
                        ),
                )
                .child(
                    div()
                        .text_size(px(10.5))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.text_tertiary)
                        .child(counter),
                ),
        )
        .child(
            div()
                .id(ElementId::Name(scroll_id.into()))
                .max_h(px(300.0))
                .overflow_y_scroll()
                .on_scroll(|_, _, cx| cx.stop_propagation())
                .p(px(5.0))
                .flex()
                .flex_col()
                .gap(px(1.0))
                .children(rows),
        )
        .into_any_element()
}

/// The subagents popover: one truncated human-readable title per subagent,
/// its icon lit while the subagent is still running.
pub(crate) fn subagents_popover(items: Rc<[SubagentInfo]>, theme: Theme) -> AnyElement {
    let rows = items.iter().enumerate().map(|(index, sub)| {
        let running = sub.status == "running";
        div()
            .id(format!("composer-subagent-row-{index}"))
            .w_full()
            .min_w_0()
            .flex()
            .items_center()
            .gap(px(8.0))
            .rounded(px(6.0))
            .px(px(6.0))
            .py(px(6.0))
            .hover(|style| style.bg(theme.overlay))
            .child(app_icon(
                IconName::Bot,
                13.0,
                if running {
                    theme.accent
                } else {
                    theme.text_tertiary
                },
            ))
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .truncate()
                    .text_size(px(12.5))
                    .text_color(theme.text)
                    // The role is the human-readable name the sidebar
                    // list used ("Plan checker"); fall back to the raw
                    // name and then the id.
                    .child(if !sub.role.trim().is_empty() {
                        sub.role.clone()
                    } else if !sub.name.trim().is_empty() {
                        sub.name.clone()
                    } else {
                        sub.subagent_id.clone()
                    }),
            )
    });
    pill_popover_card(
        "composer-subagents-popover",
        IconName::Bot,
        "SUBAGENTS",
        format!("{}", items.len()),
        "composer-subagents-popover-scroll",
        theme,
        rows,
    )
}

/// The shells popover: one truncated command per running shell with a stop
/// button on the right.
pub(crate) fn shells_popover(
    shells: Rc<[ShellEntry]>,
    on_stop: StopHandler,
    theme: Theme,
) -> AnyElement {
    let rows = shells.iter().enumerate().map(|(index, shell)| {
        let stop_id = shell.id.clone();
        let stop = on_stop.clone();
        div()
            .id(format!("composer-shell-row-{index}"))
            .w_full()
            .min_w_0()
            .flex()
            .items_center()
            .gap(px(8.0))
            .rounded(px(6.0))
            .px(px(6.0))
            .py(px(6.0))
            .hover(|style| style.bg(theme.overlay))
            .child(app_icon(IconName::Terminal, 13.0, theme.text_tertiary))
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .truncate()
                    .text_size(px(12.5))
                    .font_family(crate::markdown::render::MONO_FAMILY)
                    .text_color(theme.text)
                    .child(shell.command.clone()),
            )
            .child(
                div()
                    .id(format!("composer-shell-stop-{index}"))
                    .size(px(24.0))
                    .flex_none()
                    .rounded_full()
                    .flex()
                    .items_center()
                    .justify_center()
                    .cursor_pointer()
                    .bg(theme.danger.opacity(0.16))
                    .hover(|s| s.bg(theme.danger.opacity(0.28)))
                    .on_click(move |_, window, cx| {
                        cx.stop_propagation();
                        stop(stop_id.clone(), window, cx);
                    })
                    .child(app_icon(IconName::Stop, 11.0, theme.danger)),
            )
    });
    pill_popover_card(
        "composer-shells-popover",
        IconName::Terminal,
        "SHELLS",
        format!("{}", shells.len()),
        "composer-shells-popover-scroll",
        theme,
        rows,
    )
}

/// The automations & webhooks popover: lists active/paused schedules and
/// webhook subscriptions for the session with pause/resume and delete actions.
pub(crate) fn automations_popover(
    automations: Rc<[Automation]>,
    webhooks: Rc<[WebhookSubscription]>,
    on_pause: ActionHandler,
    on_resume: ActionHandler,
    on_delete_automation: ActionHandler,
    on_delete_webhook: ActionHandler,
    theme: Theme,
) -> AnyElement {
    let total = automations.len() + webhooks.len();
    let mut rows: Vec<AnyElement> = Vec::with_capacity(total);

    for (index, auto) in automations.iter().enumerate() {
        let is_paused = auto.status == "paused";
        let is_error = auto.status == "error";
        let toggle_id = auto.id.clone();
        let delete_id = auto.id.clone();
        let on_pause = on_pause.clone();
        let on_resume = on_resume.clone();
        let on_delete = on_delete_automation.clone();
        let subtitle = if auto.kind == "interval" {
            format!("Every {}s · {}", auto.interval_seconds, auto.status)
        } else {
            format!("One-shot · {}", auto.status)
        };
        rows.push(
            div()
                .id(format!("composer-automation-row-{index}"))
                .w_full()
                .min_w_0()
                .flex()
                .items_center()
                .gap(px(8.0))
                .rounded(px(6.0))
                .px(px(6.0))
                .py(px(6.0))
                .hover(|style| style.bg(theme.overlay))
                .child(app_icon(
                    IconName::Calendar,
                    13.0,
                    if is_error {
                        theme.danger
                    } else if is_paused {
                        theme.text_tertiary
                    } else {
                        theme.accent
                    },
                ))
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .flex()
                        .flex_col()
                        .child(
                            div()
                                .truncate()
                                .text_size(px(12.5))
                                .text_color(theme.text)
                                .child(auto.prompt.clone()),
                        )
                        .child(
                            div()
                                .truncate()
                                .text_size(px(10.5))
                                .text_color(theme.text_tertiary)
                                .child(subtitle),
                        ),
                )
                .child(
                    div()
                        .id(format!("composer-automation-toggle-{index}"))
                        .size(px(24.0))
                        .flex_none()
                        .rounded_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .cursor_pointer()
                        .bg(theme.overlay)
                        .hover(|s| s.bg(theme.overlay_strong))
                        .on_click(move |_, window, cx| {
                            cx.stop_propagation();
                            if is_paused || is_error {
                                on_resume(toggle_id.clone(), window, cx);
                            } else {
                                on_pause(toggle_id.clone(), window, cx);
                            }
                        })
                        .child(app_icon(
                            if is_paused || is_error {
                                IconName::Play
                            } else {
                                IconName::Pause
                            },
                            11.0,
                            theme.text_secondary,
                        )),
                )
                .child(
                    div()
                        .id(format!("composer-automation-delete-{index}"))
                        .size(px(24.0))
                        .flex_none()
                        .rounded_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .cursor_pointer()
                        .bg(theme.danger.opacity(0.16))
                        .hover(|s| s.bg(theme.danger.opacity(0.28)))
                        .on_click(move |_, window, cx| {
                            cx.stop_propagation();
                            on_delete(delete_id.clone(), window, cx);
                        })
                        .child(app_icon(IconName::Close, 11.0, theme.danger)),
                )
                .into_any_element(),
        );
    }

    for (index, hook) in webhooks.iter().enumerate() {
        let delete_id = hook.id.clone();
        let on_delete = on_delete_webhook.clone();
        let subtitle = format!("Webhook · {} · {}", hook.topic, hook.status);
        rows.push(
            div()
                .id(format!("composer-webhook-row-{index}"))
                .w_full()
                .min_w_0()
                .flex()
                .items_center()
                .gap(px(8.0))
                .rounded(px(6.0))
                .px(px(6.0))
                .py(px(6.0))
                .hover(|style| style.bg(theme.overlay))
                .child(app_icon(IconName::Zap, 13.0, theme.accent))
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .flex()
                        .flex_col()
                        .child(
                            div()
                                .truncate()
                                .text_size(px(12.5))
                                .text_color(theme.text)
                                .child(hook.prompt.clone()),
                        )
                        .child(
                            div()
                                .truncate()
                                .text_size(px(10.5))
                                .text_color(theme.text_tertiary)
                                .child(subtitle),
                        ),
                )
                .child(
                    div()
                        .id(format!("composer-webhook-delete-{index}"))
                        .size(px(24.0))
                        .flex_none()
                        .rounded_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .cursor_pointer()
                        .bg(theme.danger.opacity(0.16))
                        .hover(|s| s.bg(theme.danger.opacity(0.28)))
                        .on_click(move |_, window, cx| {
                            cx.stop_propagation();
                            on_delete(delete_id.clone(), window, cx);
                        })
                        .child(app_icon(IconName::Close, 11.0, theme.danger)),
                )
                .into_any_element(),
        );
    }

    pill_popover_card(
        "composer-automations-popover",
        IconName::Calendar,
        "AUTOMATIONS & WEBHOOKS",
        format!("{total}"),
        "composer-automations-popover-scroll",
        theme,
        rows,
    )
}
