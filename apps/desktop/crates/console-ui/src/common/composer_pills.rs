//! The floating row of status pills just above the composer: changed files,
//! active tasks, running subagents, and background shells. The row is an
//! overlay on the transcript's bottom edge, so chat scrolls behind it the way
//! it does on mobile instead of the pills claiming their own band. Each pill
//! is a compact left-aligned summary that opens the surface holding the full
//! list; multiple kinds sit side by side, never stacked.

use std::cell::Cell;
use std::rc::Rc;

use console_core::{Automation, SubagentInfo, TodoItem, WebhookSubscription};
use gpui::{
    App, Div, ElementId, Entity, FontWeight, ParentElement, ScrollHandle, Stateful,
    StatefulInteractiveElement, Styled, Window, div, linear_color_stop, linear_gradient,
    prelude::*, px,
};

use crate::chat::change_chips::ChangesSummary;
use crate::common::pill_popovers::{
    automations_popover, shells_popover, subagents_popover, tasks_popover,
};
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, popover};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ClickHandler = Rc<dyn Fn(&mut Window, &mut App) + 'static>;
pub(crate) type StopHandler = Rc<dyn Fn(String, &mut Window, &mut App) + 'static>;
pub(crate) type ActionHandler = Rc<dyn Fn(String, &mut Window, &mut App) + 'static>;
/// Every pill in the row, status and suggestion alike, is this tall so they
/// read as one set and match the 31px scroll controls.
const PILL_HEIGHT: f32 = 31.0;
/// Hover widens a suggestion chip so its full text is readable.
const SUGGESTION_COLLAPSED_WIDTH: f32 = 260.0;
const SUGGESTION_EXPANDED_WIDTH: f32 = 420.0;
type PickHandler = Rc<dyn Fn(String, &mut Window, &mut App) + 'static>;

/// Follow-up prompts that fill the rest of the row and scroll sideways.
pub struct SuggestionsPill {
    pub items: Vec<String>,
    pub on_pick: PickHandler,
}

/// The active task list plus the menu handle that anchors its popover to the
/// leading pill.
pub struct TasksPill {
    pub completed: usize,
    pub total: usize,
    pub menu: ContextMenuHandle,
    pub items: Rc<Vec<TodoItem>>,
}

/// The session's subagents plus the menu handle that anchors their popover.
pub struct SubagentsPill {
    pub running: bool,
    pub menu: ContextMenuHandle,
    pub items: Rc<Vec<SubagentInfo>>,
}

/// One running detached shell in the shells popover.
pub struct ShellEntry {
    pub id: String,
    pub command: String,
}

/// The session's running shells, their popover anchor, and the stop action.
pub struct ShellsPill {
    pub menu: ContextMenuHandle,
    pub shells: Rc<Vec<ShellEntry>>,
    pub on_stop: StopHandler,
}

/// The session's active automations and webhook subscriptions.
pub struct AutomationsPill {
    pub menu: ContextMenuHandle,
    pub automations: Rc<Vec<Automation>>,
    pub webhooks: Rc<Vec<WebhookSubscription>>,
    pub on_pause: ActionHandler,
    pub on_resume: ActionHandler,
    pub on_delete_automation: ActionHandler,
    pub on_delete_webhook: ActionHandler,
}

/// The pills row, or `None` when the session has nothing to summarise.
///
/// `changes` carries the session's change totals plus the handler that opens
/// the changes view. The subagents, shells, and automations pills open their
/// own popovers, the way the tasks pill does.
#[allow(clippy::type_complexity)]
pub fn composer_pills(
    key: &str,
    tasks: Option<TasksPill>,
    subagents: Option<SubagentsPill>,
    shells: Option<ShellsPill>,
    automations: Option<AutomationsPill>,
    changes: Option<(ChangesSummary, ClickHandler)>,
    suggestions: Option<SuggestionsPill>,
    scroll: &ScrollHandle,
    theme: Theme,
    cx: &mut App,
) -> Option<Div> {
    if tasks.is_none()
        && subagents.is_none()
        && shells.is_none()
        && automations.is_none()
        && changes.is_none()
        && suggestions.is_none()
    {
        return None;
    }

    // One sideways scroller holds every pill, so a narrow pane scrolls
    // instead of cutting anything off. Popovers draw deferred, outside this
    // scroller's clip, so they still open above their pill.
    let mut row = div()
        .id(ElementId::Name(format!("{key}-scroll").into()))
        .w_full()
        .flex()
        .flex_row()
        .items_center()
        .gap(px(8.0))
        .overflow_x_scroll()
        .restrict_scroll_to_axis()
        .track_scroll(scroll);

    if let Some((summary, on_changes)) = changes {
        row = row.child(changes_pill(
            format!("{key}-changes"),
            summary,
            Some(on_changes),
            theme,
        ));
    }

    if let Some(tasks) = tasks {
        let trigger = pill(
            format!("{key}-tasks"),
            IconName::Checklist,
            format!("{}/{} tasks", tasks.completed, tasks.total),
            false,
            None,
            theme,
        );
        let items = tasks.items;
        row = row.child(popover(
            trigger,
            &tasks.menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, cx| tasks_popover(items.clone(), Theme::current(cx)),
        ));
    }
    if let Some(subagents) = subagents {
        let trigger = pill(
            format!("{key}-subagents"),
            IconName::Bot,
            format!(
                "{} {}",
                subagents.items.len(),
                if subagents.items.len() == 1 {
                    "subagent"
                } else {
                    "subagents"
                }
            ),
            subagents.running,
            None,
            theme,
        );
        let items = subagents.items;
        row = row.child(popover(
            trigger,
            &subagents.menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, cx| subagents_popover(items.clone(), Theme::current(cx)),
        ));
    }
    if let Some(shells) = shells {
        let count = shells.shells.len();
        let trigger = pill(
            format!("{key}-shells"),
            IconName::Terminal,
            format!("{count} {}", if count == 1 { "shell" } else { "shells" }),
            true,
            None,
            theme,
        );
        let shells_list = shells.shells;
        let on_stop = shells.on_stop;
        row = row.child(popover(
            trigger,
            &shells.menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, cx| {
                shells_popover(shells_list.clone(), on_stop.clone(), Theme::current(cx))
            },
        ));
    }

    if let Some(automations) = automations {
        let auto_count = automations.automations.len();
        let hook_count = automations.webhooks.len();
        let label = match (auto_count, hook_count) {
            (a, 0) => format!("{a} {}", if a == 1 { "schedule" } else { "schedules" }),
            (0, w) => format!("{w} {}", if w == 1 { "webhook" } else { "webhooks" }),
            (a, w) => format!("{a} sched · {w} hook"),
        };
        let active = automations.automations.iter().any(|a| a.status == "active")
            || automations.webhooks.iter().any(|w| w.status == "active");
        let icon = if auto_count > 0 {
            IconName::Calendar
        } else {
            IconName::Zap
        };
        let trigger = pill(
            format!("{key}-automations"),
            icon,
            label,
            active,
            None,
            theme,
        );
        let autos = automations.automations;
        let hooks = automations.webhooks;
        let on_pause = automations.on_pause;
        let on_resume = automations.on_resume;
        let on_del_auto = automations.on_delete_automation;
        let on_del_hook = automations.on_delete_webhook;
        row = row.child(popover(
            trigger,
            &automations.menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, cx| {
                automations_popover(
                    autos.clone(),
                    hooks.clone(),
                    on_pause.clone(),
                    on_resume.clone(),
                    on_del_auto.clone(),
                    on_del_hook.clone(),
                    Theme::current(cx),
                )
            },
        ));
    }

    if let Some(suggestions) = suggestions {
        let on_pick = suggestions.on_pick;
        row = row.children(
            suggestions
                .items
                .into_iter()
                .enumerate()
                .map(|(index, text)| {
                    let hover = cx.new(|_| Cell::new(false));
                    suggestion_pill(
                        format!("{key}-suggestion-{index}"),
                        text,
                        on_pick.clone(),
                        hover,
                        theme,
                        cx,
                    )
                }),
        );
    }

    let offset = scroll.offset().x;
    let max = scroll.max_offset().x;
    // Offsets run negative as the row scrolls right.
    let more_left = offset < px(-0.5);
    let more_right = max > px(0.5) && -offset < max - px(0.5);
    let clear = theme.chat_canvas.opacity(0.0);
    let fade = |angle: f32| {
        linear_gradient(
            angle,
            linear_color_stop(theme.chat_canvas, 0.0),
            linear_color_stop(clear, 1.0),
        )
    };

    Some(
        div()
            .absolute()
            .left_0()
            .right_0()
            // Same vertical alignment as the transcript's jump / scroll-to-bottom
            // pill (31px tall, 10px up), centred on the transcript column.
            .bottom(px(10.0))
            .h(px(31.0))
            .w_full()
            .px(px(20.0))
            .flex()
            .items_center()
            .justify_center()
            .child(
                div()
                    .relative()
                    .w_full()
                    .max_w(px(768.0))
                    .h(px(31.0))
                    .flex()
                    .items_center()
                    // The transcript's jump / scroll-to-bottom pill floats at the
                    // bottom right (~67px wide, 16px in); keep pills inside the
                    // max-w column from sliding under it.
                    .pr(px(96.0))
                    .child(row)
                    // Soft edges so a pill fades out instead of being sliced;
                    // shown only where there is more to scroll to.
                    .when(more_left, |el| {
                        el.child(
                            div()
                                .absolute()
                                .left_0()
                                .top_0()
                                .bottom_0()
                                .w(px(28.0))
                                .bg(fade(90.0)),
                        )
                    })
                    .when(more_right, |el| {
                        el.child(
                            div()
                                .absolute()
                                .right_0()
                                .top_0()
                                .bottom_0()
                                .w(px(28.0))
                                .bg(fade(270.0)),
                        )
                    }),
            ),
    )
}

/// The changed-files pill: icon, "N files changed", and the colored `+N -N`
/// totals. Clicking opens the changes review for the session.
fn changes_pill(
    id: String,
    summary: ChangesSummary,
    on_click: Option<ClickHandler>,
    theme: Theme,
) -> Stateful<Div> {
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .flex()
        .items_center()
        .gap(px(6.0))
        .h(px(PILL_HEIGHT))
        .px(px(10.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        // Same hover tint as the suggestion chips: overlay is a ~5% tint
        // meant to sit on a surface, so blend it onto raised.
        .hover(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .active(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .when_some(on_click, |element, handler| {
            element.on_click(move |_, window, cx| (handler)(window, cx))
        })
        .child(app_icon(IconName::FileDiff, 13.0, theme.text_tertiary))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(format!(
                    "{} file{} changed",
                    summary.files,
                    if summary.files == 1 { "" } else { "s" }
                )),
        )
        .when(summary.additions > 0, |element| {
            element.child(
                div()
                    .text_size(px(12.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.success)
                    .child(format!("+{}", summary.additions)),
            )
        })
        .when(summary.deletions > 0, |element| {
            element.child(
                div()
                    .text_size(px(12.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.danger)
                    .child(format!("-{}", summary.deletions)),
            )
        })
}

fn pill(
    id: String,
    icon: IconName,
    text: String,
    running: bool,
    on_click: Option<ClickHandler>,
    theme: Theme,
) -> Stateful<Div> {
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .flex()
        .items_center()
        .gap(px(6.0))
        .h(px(PILL_HEIGHT))
        .px(px(10.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        // Same hover tint as the suggestion chips: overlay is a ~5% tint
        // meant to sit on a surface, so blend it onto raised.
        .hover(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .active(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .when_some(on_click, |element, handler| {
            element.on_click(move |_, window, cx| (handler)(window, cx))
        })
        .child(app_icon(
            icon,
            13.0,
            if running {
                theme.accent
            } else {
                theme.text_tertiary
            },
        ))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(text),
        )
}

/// One follow-up chip: the pill shape of the status row, with an accent
/// send button so it reads as clickable rather than informational.
fn suggestion_pill(
    id: String,
    text: String,
    on_pick: PickHandler,
    hover: Entity<Cell<bool>>,
    theme: Theme,
    cx: &App,
) -> Stateful<Div> {
    let picked = text.clone();
    let width_for_hover = hover.clone();
    let set_hover = hover.clone();
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .max_w(px(if width_for_hover.read(cx).get() {
            SUGGESTION_EXPANDED_WIDTH
        } else {
            SUGGESTION_COLLAPSED_WIDTH
        }))
        .flex()
        .items_center()
        .gap(px(7.0))
        .h(px(PILL_HEIGHT))
        .pl(px(4.0))
        .pr(px(10.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        // `overlay` is a ~5% tint meant to sit on top of a surface, so it has
        // to be blended onto `raised` or the chip turns see-through on hover.
        .hover(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .active(|style| style.bg(theme.raised.blend(theme.overlay_strong)))
        .on_hover(move |is_hovered, _, cx| {
            let changed = set_hover.update(cx, |cell, _| cell.replace(*is_hovered) != *is_hovered);
            if changed {
                cx.notify(set_hover.entity_id());
            }
        })
        .on_click(move |_, window, cx| (on_pick)(picked.clone(), window, cx))
        .child(
            div()
                .flex_none()
                .size(px(20.0))
                .rounded_full()
                .bg(theme.accent)
                .flex()
                .items_center()
                .justify_center()
                .child(app_icon(IconName::ArrowUp, 12.0, theme.on_inverse)),
        )
        .child(
            div()
                .min_w(px(0.0))
                .text_size(px(12.0))
                .font_weight(FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .truncate()
                .child(text),
        )
}
