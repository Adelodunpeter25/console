use gpui::{
    App, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement, KeyDownEvent, ParentElement,
    SharedString, StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use crate::utils::{SessionDateGroup, SidebarSortMode};

/// Toggle between date and project sectioning for the sidebar session list.
/// Shows the current mode label; clicking switches to the other axis.
pub fn sort_mode_toggle(
    theme: Theme,
    sort_mode: SidebarSortMode,
    on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    let (label, tooltip) = match sort_mode {
        SidebarSortMode::Date => ("Date", "Sort by project"),
        SidebarSortMode::Project => ("Project", "Sort by date"),
    };
    div()
        .id("btn-toggle-sort")
        .h(px(20.0))
        .px(px(6.0))
        .rounded(px(6.0))
        .flex()
        .items_center()
        .gap(px(4.0))
        .cursor_default()
        .hover(|s| s.bg(theme.overlay))
        .active(|s| s.bg(theme.overlay_strong))
        .tooltip(Tooltip::text(tooltip))
        .on_click(move |_, window, cx| (on_toggle)(window, cx))
        .child(app_icon(IconName::SortVertical, 12.0, theme.text_ghost))
        .child(
            div()
                .text_size(px(11.0))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(theme.text_tertiary)
                .child(label),
        )
}

type ToggleFn = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// Every section header (date group, project, Pinned, Drafts, Settled) is this
/// one layout: a clickable, keyboard-toggleable label with a hover-revealed
/// chevron, and an optional add-project button on the right edge.
fn section_header(
    theme: Theme,
    group_name: impl Into<SharedString>,
    toggle_id: ElementId,
    label: impl Into<SharedString>,
    label_color: Hsla,
    collapsed: bool,
    on_add: Option<ToggleFn>,
    on_toggle: ToggleFn,
) -> impl IntoElement {
    let group_name: SharedString = group_name.into();
    let chevron = app_icon(IconName::ChevronDown, 11.0, theme.text_ghost)
        .when(collapsed, |icon| {
            icon.with_transformation(gpui::Transformation::rotate(gpui::percentage(0.75)))
        })
        .invisible()
        .group_hover(group_name.clone(), |icon| icon.visible());
    let toggle_on_click = on_toggle.clone();
    let toggle_on_key = on_toggle;

    div()
        .h(px(28.0))
        .flex()
        .items_center()
        .justify_between()
        .group(group_name)
        .child(
            div()
                .id(toggle_id)
                .tab_index(0)
                .h(px(22.0))
                .flex()
                .items_center()
                .gap(px(4.0))
                .cursor_default()
                .hover(|s| s.bg(theme.overlay))
                .text_size(px(14.5))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(label_color)
                .child(label.into())
                .child(chevron)
                .on_click(move |_, window, cx| (toggle_on_click)(window, cx))
                .on_key_down(move |event: &KeyDownEvent, window, cx| {
                    if matches!(event.keystroke.key.as_str(), "enter" | "space") {
                        (toggle_on_key)(window, cx);
                        cx.stop_propagation();
                    }
                }),
        )
        .when_some(on_add, |header, on_add| {
            header.child(
                div()
                    .id("btn-add-project")
                    .w(px(20.0))
                    .h(px(20.0))
                    .rounded(px(6.0))
                    .flex()
                    .items_center()
                    .justify_center()
                    .cursor_default()
                    .hover(|s| s.bg(theme.overlay))
                    .active(|s| s.bg(theme.overlay_strong))
                    .on_click(move |_, window, cx| (on_add)(window, cx))
                    .child(app_icon(IconName::FolderNew, 15.0, theme.text_ghost)),
            )
        })
}

/// A project section header for the project-sorted session list, keyed by
/// project id (or "none") instead of a calendar bucket.
pub fn project_header(
    theme: Theme,
    name: String,
    key: String,
    collapsed: bool,
    with_add_button: bool,
    on_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    let toggle_key = key.clone();
    section_header(
        theme,
        format!("sidebar-project-header-{key}"),
        ElementId::Name(format!("sidebar-project-toggle-{key}").into()),
        name,
        theme.text,
        collapsed,
        with_add_button.then_some(on_add),
        Rc::new(move |window, cx| (on_toggle)(toggle_key.clone(), window, cx)),
    )
}

/// A calendar-period section header for the grouped session list. Clicking the
/// label collapses or expands the group (a chevron reveals on hover). The
/// first group's header also hosts the add-project button on its right edge,
/// so the label and the folder+ icon sit on the same line.
#[allow(clippy::too_many_arguments)]
pub fn group_header(
    theme: Theme,
    label: &'static str,
    collapsed: bool,
    with_add_button: bool,
    on_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(SessionDateGroup, &mut Window, &mut App) + 'static>,
    group: SessionDateGroup,
) -> impl IntoElement {
    section_header(
        theme,
        format!("sidebar-group-header-{}", group.index()),
        ElementId::Name(format!("sidebar-group-toggle-{}", group.index()).into()),
        label,
        theme.text,
        collapsed,
        with_add_button.then_some(on_add),
        Rc::new(move |window, cx| (on_toggle)(group, window, cx)),
    )
}

/// Collapsible section header for the Pinned sessions group.
pub fn pinned_group_header(
    theme: Theme,
    collapsed: bool,
    with_add_button: bool,
    on_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-pinned",
        ElementId::Name("sidebar-group-toggle-pinned".into()),
        "Pinned",
        theme.text,
        collapsed,
        with_add_button.then_some(on_add),
        on_toggle,
    )
}

/// Collapsible section header for the Drafts group.
pub fn drafts_group_header(
    theme: Theme,
    collapsed: bool,
    with_add_button: bool,
    on_add: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-drafts",
        ElementId::Name("sidebar-group-toggle-drafts".into()),
        "Drafts",
        theme.text,
        collapsed,
        with_add_button.then_some(on_add),
        on_toggle,
    )
}

/// Collapsible section header for the Settled chats group. The section always
/// sits at the bottom of the list, so it never hosts the add button, and its
/// label is dimmed to match the archived rows below it.
pub fn settled_group_header(
    theme: Theme,
    collapsed: bool,
    on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-settled",
        ElementId::Name("sidebar-group-toggle-settled".into()),
        "Settled",
        theme.text_tertiary,
        collapsed,
        None,
        on_toggle,
    )
}
