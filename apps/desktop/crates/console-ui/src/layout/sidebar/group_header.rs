use gpui::{
    App, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement, KeyDownEvent, ParentElement,
    SharedString, StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ToggleFn = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// Pinned, Drafts and Settled share this layout: a clickable,
/// keyboard-toggleable label with a hover-revealed chevron.
fn section_header(
    theme: Theme,
    group_name: impl Into<SharedString>,
    toggle_id: ElementId,
    label: impl Into<SharedString>,
    label_color: Hsla,
    collapsed: bool,
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
}

/// Muted "My workspaces" label shown above the project rows.
pub fn workspaces_label(theme: Theme) -> impl IntoElement {
    div()
        .h(px(30.0))
        .flex()
        .items_center()
        .text_size(px(12.5))
        .font_weight(FontWeight::SEMIBOLD)
        .text_color(theme.text_tertiary)
        .child("My workspaces")
}

/// A project row: a placeholder avatar, the bold project name, and a plain `+`
/// on the right that starts a new chat in that project. Clicking the row
/// collapses or expands it.
pub fn project_header(
    theme: Theme,
    name: String,
    key: String,
    collapsed: bool,
    on_new_chat: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    let group_name: SharedString = format!("sidebar-project-header-{key}").into();
    let initial = name
        .chars()
        .next()
        .map(|c| c.to_uppercase().to_string())
        .unwrap_or_default();
    let toggle_key = key.clone();
    let new_chat_key = key.clone();
    let chevron_rotation = if collapsed { 0.75 } else { 0.0 };

    div()
        .w_full()
        .h(px(34.0))
        .flex()
        .items_center()
        .justify_between()
        .group(group_name.clone())
        .child(
            div()
                .id(ElementId::Name(
                    format!("sidebar-project-toggle-{key}").into(),
                ))
                .tab_index(0)
                .flex_1()
                .min_w_0()
                .h(px(28.0))
                .flex()
                .items_center()
                .gap(px(10.0))
                .cursor_default()
                .on_click(move |_, window, cx| (on_toggle)(toggle_key.clone(), window, cx))
                .child(
                    div()
                        .size(px(26.0))
                        .flex_none()
                        .rounded_full()
                        .bg(theme.overlay_strong)
                        .flex()
                        .items_center()
                        .justify_center()
                        .text_size(px(11.0))
                        .font_weight(FontWeight::SEMIBOLD)
                        .text_color(theme.text_secondary)
                        .child(initial),
                )
                .child(
                    div()
                        .truncate()
                        .text_size(px(15.0))
                        .font_weight(FontWeight::SEMIBOLD)
                        .text_color(theme.text_secondary)
                        .child(name),
                )
                .child(
                    app_icon(IconName::ChevronDown, 11.0, theme.text_ghost)
                        .with_transformation(gpui::Transformation::rotate(gpui::percentage(
                            chevron_rotation,
                        )))
                        .invisible()
                        .group_hover(group_name, |icon| icon.visible()),
                ),
        )
        .child(
            div()
                .id(ElementId::Name(format!("btn-new-chat-{key}").into()))
                .size(px(24.0))
                .flex_none()
                .rounded(px(6.0))
                .flex()
                .items_center()
                .justify_center()
                .cursor_default()
                .hover(|s| s.bg(theme.overlay))
                .active(|s| s.bg(theme.overlay_strong))
                .tooltip(Tooltip::text("New chat"))
                .on_click(move |_, window, cx| (on_new_chat)(new_chat_key.clone(), window, cx))
                .child(app_icon(IconName::Plus, 18.0, theme.text_secondary)),
        )
}

/// Collapsible section header for the Pinned sessions group.
pub fn pinned_group_header(theme: Theme, collapsed: bool, on_toggle: ToggleFn) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-pinned",
        ElementId::Name("sidebar-group-toggle-pinned".into()),
        "Pinned",
        theme.text,
        collapsed,
        on_toggle,
    )
}

/// Collapsible section header for the Drafts group.
pub fn drafts_group_header(theme: Theme, collapsed: bool, on_toggle: ToggleFn) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-drafts",
        ElementId::Name("sidebar-group-toggle-drafts".into()),
        "Drafts",
        theme.text,
        collapsed,
        on_toggle,
    )
}

/// Collapsible section header for the Settled chats group. The section always
/// sits at the bottom of the list, and its label is dimmed to match the
/// archived rows below it.
pub fn settled_group_header(
    theme: Theme,
    collapsed: bool,
    on_toggle: ToggleFn,
) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-settled",
        ElementId::Name("sidebar-group-toggle-settled".into()),
        "Settled",
        theme.text_tertiary,
        collapsed,
        on_toggle,
    )
}

/// Section header for the search results list, using the same height and
/// typography as the other sidebar section headers.
pub fn search_results_group_header(
    theme: Theme,
    label: impl Into<SharedString>,
) -> impl IntoElement {
    div()
        .h(px(28.0))
        .flex()
        .items_center()
        .justify_between()
        .child(
            div()
                .h(px(22.0))
                .flex()
                .items_center()
                .gap(px(4.0))
                .text_size(px(14.5))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(theme.text)
                .child(label.into()),
        )
}
