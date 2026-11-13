//! The floating row of status pills just above the composer: changed files,
//! active tasks, running subagents, and background shells. The row is an
//! overlay on the transcript's bottom edge, so chat scrolls behind it the way
//! it does on mobile instead of the pills claiming their own band. Each pill
//! is a compact left-aligned summary that opens the surface holding the full
//! list; multiple kinds sit side by side, never stacked.

use std::rc::Rc;

use console_core::{SubagentInfo, TodoItem};
use gpui::{
    App, Div, ElementId, FontWeight, ParentElement, Stateful, StatefulInteractiveElement, Styled,
    Window, div, prelude::*, px,
};

use crate::chat::change_chips::ChangesSummary;
use crate::common::pill_popovers::{shells_popover, subagents_popover, tasks_popover};
use crate::primitives::menu::{ContextMenuHandle, MenuAlign, popover};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ClickHandler = Rc<dyn Fn(&mut Window, &mut App) + 'static>;
pub(crate) type StopHandler = Rc<dyn Fn(String, &mut Window, &mut App) + 'static>;
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

/// The pills row, or `None` when the session has nothing to summarise.
///
/// `changes` carries the session's change totals plus the handler that opens
/// the changes view. The subagents and shells pills open their own popovers,
/// the way the tasks pill does.
#[allow(clippy::type_complexity)]
pub fn composer_pills(
    key: &str,
    tasks: Option<TasksPill>,
    subagents: Option<SubagentsPill>,
    shells: Option<ShellsPill>,
    changes: Option<(ChangesSummary, ClickHandler)>,
    suggestions: Option<SuggestionsPill>,
    theme: Theme,
) -> Option<Div> {
    if tasks.is_none()
        && subagents.is_none()
        && shells.is_none()
        && changes.is_none()
        && suggestions.is_none()
    {
        return None;
    }

    let mut row = div()
        .w_full()
        .max_w(px(768.0))
        .flex()
        .flex_row()
        .items_center()
        .gap(px(8.0));

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

    // The status pills keep their size on the left; the chips take whatever
    // room remains and scroll sideways. Kept outside the scroller so the
    // status popovers are never clipped by it.
    if let Some(suggestions) = suggestions {
        let on_pick = suggestions.on_pick;
        row = row.child(
            div()
                .id(ElementId::Name(format!("{key}-suggestions").into()))
                .flex_1()
                .min_w(px(0.0))
                .flex()
                .flex_row()
                .items_center()
                .gap(px(8.0))
                .overflow_x_scroll()
                .children(
                    suggestions
                        .items
                        .into_iter()
                        .enumerate()
                        .map(|(index, text)| {
                            suggestion_pill(
                                format!("{key}-suggestion-{index}"),
                                text,
                                on_pick.clone(),
                                theme,
                            )
                        }),
                ),
        );
    }

    Some(
        div()
            .absolute()
            .left_0()
            .right_0()
            .bottom(px(8.0))
            .w_full()
            .px(px(20.0))
            .flex()
            .justify_center()
            .child(row),
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
        .px(px(10.0))
        .py(px(5.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
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
        .px(px(10.0))
        .py(px(5.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
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
fn suggestion_pill(id: String, text: String, on_pick: PickHandler, theme: Theme) -> Stateful<Div> {
    let picked = text.clone();
    div()
        .id(ElementId::Name(id.into()))
        .flex_none()
        .max_w(px(260.0))
        .flex()
        .items_center()
        .gap(px(7.0))
        .pl(px(4.0))
        .pr(px(10.0))
        .py(px(3.0))
        .rounded_full()
        .bg(theme.raised)
        .cursor_pointer()
        .hover(|style| style.bg(theme.overlay))
        .active(|style| style.bg(theme.overlay_strong))
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
