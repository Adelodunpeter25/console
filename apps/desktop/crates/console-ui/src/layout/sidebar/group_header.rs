use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, KeyDownEvent, ParentElement,
    SharedString, StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type ToggleFn = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// The card every collapsible sidebar header shares: a soft rounded
/// background that shows on hover, a leading chevron that is always visible (rotated when the
/// section is collapsed), and the label.
fn header_card(theme: Theme, group_name: SharedString) -> gpui::Div {
    div()
        .w_full()
        .h(px(36.0))
        .mt(px(4.0))
        .px(px(8.0))
        .rounded(px(8.0))
        .flex()
        .items_center()
        .justify_between()
        .group_hover(group_name.clone(), |s| s.bg(theme.sidebar_item_background))
        .group(group_name)
}

fn header_chevron(theme: Theme, collapsed: bool) -> impl IntoElement {
    app_icon(IconName::ChevronDown, 12.0, theme.text_ghost).when(collapsed, |icon| {
        icon.with_transformation(gpui::Transformation::rotate(gpui::percentage(0.75)))
    })
}

fn header_label(theme: Theme, label: impl Into<SharedString>) -> gpui::Div {
    div()
        .truncate()
        .text_size(px(14.5))
        .font_weight(FontWeight::SEMIBOLD)
        .text_color(theme.text_secondary)
        .child(label.into())
}

/// Pinned, Drafts and Settled share this layout: a clickable,
/// keyboard-toggleable card.
fn section_header(
    theme: Theme,
    group_name: impl Into<SharedString>,
    toggle_id: ElementId,
    label: impl Into<SharedString>,
    collapsed: bool,
    on_toggle: ToggleFn,
) -> impl IntoElement {
    let toggle_on_click = on_toggle.clone();
    let toggle_on_key = on_toggle;

    header_card(theme, group_name.into()).child(
        div()
            .id(toggle_id)
            .tab_index(0)
            .flex_1()
            .min_w_0()
            .h_full()
            .flex()
            .items_center()
            .gap(px(10.0))
            .cursor_default()
            .child(header_chevron(theme, collapsed))
            .child(header_label(theme, label))
            .on_click(move |_, window, cx| (toggle_on_click)(window, cx))
            .on_key_down(move |event: &KeyDownEvent, window, cx| {
                if matches!(event.keystroke.key.as_str(), "enter" | "space") {
                    (toggle_on_key)(window, cx);
                    cx.stop_propagation();
                }
            }),
    )
}

/// A project card: chevron, the project name, and a plain `+` on the right
/// that starts a new chat in that project. Clicking the card collapses or
/// expands it.
pub fn project_header(
    theme: Theme,
    name: String,
    key: String,
    collapsed: bool,
    on_new_chat: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    let group_name: SharedString = format!("sidebar-project-header-{key}").into();
    let toggle_key = key.clone();
    let new_chat_key = key.clone();

    header_card(theme, group_name)
        .child(
            div()
                .id(ElementId::Name(
                    format!("sidebar-project-toggle-{key}").into(),
                ))
                .tab_index(0)
                .flex_1()
                .min_w_0()
                .h_full()
                .flex()
                .items_center()
                .gap(px(10.0))
                .cursor_default()
                .on_click(move |_, window, cx| (on_toggle)(toggle_key.clone(), window, cx))
                .child(header_chevron(theme, collapsed))
                .child(header_label(theme, name)),
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
                .child(app_icon(IconName::Plus, 16.0, theme.text_secondary)),
        )
}

/// Collapsible header card for the Pinned sessions group.
pub fn pinned_group_header(theme: Theme, collapsed: bool, on_toggle: ToggleFn) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-pinned",
        ElementId::Name("sidebar-group-toggle-pinned".into()),
        "Pinned",
        collapsed,
        on_toggle,
    )
}

/// Collapsible header card for the Drafts group.
pub fn drafts_group_header(theme: Theme, collapsed: bool, on_toggle: ToggleFn) -> impl IntoElement {
    section_header(
        theme,
        "sidebar-group-header-drafts",
        ElementId::Name("sidebar-group-toggle-drafts".into()),
        "Drafts",
        collapsed,
        on_toggle,
    )
}

/// Collapsible header card for the Settled chats group, which always sits at
/// the bottom of the list.
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
