//! The row of status pills just above the composer: active tasks, running
//! subagents, and background shells. Each pill is a compact left-aligned
//! summary that opens the surface holding the full list, mirroring the mobile
//! chat composer. Multiple kinds sit side by side, never stacked.

use std::rc::Rc;

use gpui::{
    App, Div, ElementId, FontWeight, ParentElement, Stateful, StatefulInteractiveElement, Styled,
    Window, div, prelude::*, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ClickHandler = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// The pills row, or `None` when the session has nothing to summarise.
///
/// `tasks` is `(completed, total)` and only present while unfinished work
/// remains; `subagents` is `(count, any running)`; `shells` is the live count.
#[allow(clippy::too_many_arguments)]
pub fn composer_pills(
    key: &str,
    tasks: Option<(usize, usize)>,
    subagents: Option<(usize, bool)>,
    shells: usize,
    on_tasks: Option<ClickHandler>,
    on_subagents: Option<ClickHandler>,
    on_shells: Option<ClickHandler>,
    theme: Theme,
) -> Option<Div> {
    if tasks.is_none() && subagents.is_none() && shells == 0 {
        return None;
    }

    let mut row = div()
        .w_full()
        .max_w(px(768.0))
        .flex()
        .flex_row()
        .items_center()
        .gap(px(8.0));

    if let Some((completed, total)) = tasks {
        row = row.child(pill(
            format!("{key}-tasks"),
            IconName::Checklist,
            format!("{completed}/{total} tasks"),
            false,
            on_tasks,
            theme,
        ));
    }
    if let Some((count, running)) = subagents {
        row = row.child(pill(
            format!("{key}-subagents"),
            IconName::Bot,
            format!(
                "{count} {}",
                if count == 1 { "subagent" } else { "subagents" }
            ),
            running,
            on_subagents,
            theme,
        ));
    }
    if shells > 0 {
        row = row.child(pill(
            format!("{key}-shells"),
            IconName::Terminal,
            format!("{shells} {}", if shells == 1 { "shell" } else { "shells" }),
            true,
            on_shells,
            theme,
        ));
    }

    Some(
        div()
            .w_full()
            .px(px(20.0))
            .pt(px(6.0))
            .pb(px(2.0))
            .flex()
            .justify_center()
            .child(row),
    )
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
        .when_some(on_click, |element, handler| {
            element
                .cursor_pointer()
                .hover(|style| style.bg(theme.overlay_strong))
                .on_click(move |_, window, cx| (handler)(window, cx))
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
