//! Git & Session Changes List with Diff Badges for the Right Inspector.

use std::rc::Rc;

use console_core::types::{ChangesScope, GitFileEntry, SessionFileChange};
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::primitives::file_icons::file_icon_for_name;
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{
    ContextMenuHandle, MenuAlign, MenuChip, MenuItem, base_name, dropdown_menu, file_icon,
};
use crate::theme::Theme;
use crate::utils::short_parent_dir;

#[derive(IntoElement)]
pub struct ChangesListView {
    working_changes: Rc<Vec<GitFileEntry>>,
    session_changes: Rc<Vec<SessionFileChange>>,
    scope: ChangesScope,
    scope_menu: ContextMenuHandle,
    selected_path: Option<String>,
    on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_select_scope: Rc<dyn Fn(ChangesScope, &mut Window, &mut App) + 'static>,
    on_view_all: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ChangesListView {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        working_changes: Rc<Vec<GitFileEntry>>,
        session_changes: Rc<Vec<SessionFileChange>>,
        scope: ChangesScope,
        scope_menu: ContextMenuHandle,
        selected_path: Option<String>,
        on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_select_scope: Rc<dyn Fn(ChangesScope, &mut Window, &mut App) + 'static>,
        on_view_all: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            working_changes,
            session_changes,
            scope,
            scope_menu,
            selected_path,
            on_select_file,
            on_select_scope,
            on_view_all,
        }
    }
}

impl RenderOnce for ChangesListView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_select = self.on_select_file;
        let selected_path = self.selected_path;

        let scoped_session_changes =
            console_core::types::filter_changes_for_scope(&self.session_changes, self.scope);
        let has_session_changes = !scoped_session_changes.is_empty();
        let has_changes = !self.working_changes.is_empty() || has_session_changes;

        let total_additions: u64 = scoped_session_changes.iter().map(|c| c.additions).sum();
        let total_deletions: u64 = scoped_session_changes.iter().map(|c| c.deletions).sum();
        let file_count = scoped_session_changes.len();

        let scope = self.scope;
        let scope_menu = self.scope_menu;
        let on_select_scope = self.on_select_scope;
        let on_view_all = self.on_view_all;

        div()
            .id("changes-list-container")
            .flex_1()
            .w_full()
            .h_full()
            .min_h_0()
            .flex()
            .flex_col()
            .overflow_hidden()
            .when(has_session_changes, |el| {
                el.child(
                    div()
                        .flex_none()
                        .flex()
                        .items_center()
                        .justify_between()
                        .gap(px(6.0))
                        .px(px(8.0))
                        .py(px(6.0))
                        .border_b_1()
                        .border_color(theme.sidebar_border)
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .gap(px(6.0))
                                .min_w_0()
                                .child(
                                    div()
                                        .text_size(px(11.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text_secondary)
                                        .child(format!(
                                            "{} file{} changed",
                                            file_count,
                                            if file_count == 1 { "" } else { "s" }
                                        )),
                                )
                                .when(total_additions > 0, |el| {
                                    el.child(
                                        div()
                                            .text_size(px(11.0))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.success)
                                            .child(format!("+{}", total_additions)),
                                    )
                                })
                                .when(total_deletions > 0, |el| {
                                    el.child(
                                        div()
                                            .text_size(px(11.0))
                                            .font_weight(gpui::FontWeight::MEDIUM)
                                            .text_color(theme.danger)
                                            .child(format!("-{}", total_deletions)),
                                    )
                                }),
                        )
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .gap(px(6.0))
                                .flex_none()
                                .child(dropdown_menu(
                                    MenuChip::new("changes-scope-trigger").label(scope.label()),
                                    "changes-scope-menu",
                                    &scope_menu,
                                    MenuAlign::BelowRight,
                                    move |_cx| {
                                        ChangesScope::ALL
                                            .into_iter()
                                            .map(|s| {
                                                let on_select_scope = on_select_scope.clone();
                                                MenuItem::new(s.label(), move |window, cx| {
                                                    (on_select_scope)(s, window, cx);
                                                })
                                                .selected(s == scope)
                                            })
                                            .collect()
                                    },
                                ))
                                .child(
                                    div()
                                        .id("changes-view-all-btn")
                                        .px(px(8.0))
                                        .h(px(24.0))
                                        .rounded(px(6.0))
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .text_size(px(11.0))
                                        .font_weight(gpui::FontWeight::MEDIUM)
                                        .text_color(theme.text_secondary)
                                        .cursor_pointer()
                                        .hover(|s| s.bg(theme.overlay))
                                        .on_click(move |_, window, cx| (on_view_all)(window, cx))
                                        .child("View all"),
                                ),
                        ),
                )
            })
            .child(
                div()
                    .id("changes-list-scroll")
                    .flex_1()
                    .w_full()
                    .min_h_0()
                    .overflow_y_scroll()
                    .p(px(6.0))
                    .child(if !has_changes {
                        div()
                            .flex_1()
                            .flex()
                            .items_center()
                            .justify_center()
                            .py(px(32.0))
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .child("No working tree changes")
                            .into_any_element()
                    } else {
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(2.0))
                            .children(self.working_changes.iter().map(|entry| {
                                let on_select = on_select.clone();
                                let path = entry.path.clone();
                                let is_selected = selected_path.as_deref() == Some(&path);

                                let (status_label, status_color) = match entry.status.as_str() {
                                    "M" | "modified" => ("M", theme.warning),
                                    "A" | "added" => ("A", theme.success),
                                    "D" | "deleted" => ("D", theme.danger),
                                    "R" => ("R", theme.accent),
                                    _ => ("?", theme.text_tertiary),
                                };

                                let file_name = base_name(&entry.path).to_string();
                                let short_dir = short_parent_dir(&entry.path);
                                let tooltip_path = entry.path.clone();

                                div()
                                    .id(format!("change-row-{}", entry.path))
                                    .flex()
                                    .items_center()
                                    .justify_between()
                                    .h(px(28.0))
                                    .w_full()
                                    .px(px(8.0))
                                    .rounded(px(4.0))
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .when(is_selected, |s| s.bg(theme.overlay_strong))
                                    .tooltip(Tooltip::text(tooltip_path))
                                    .on_click(move |_, window, cx| {
                                        (on_select)(path.clone(), window, cx);
                                    })
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(6.0))
                                            .min_w_0()
                                            .flex_1()
                                            .child(
                                                div()
                                                    .text_size(px(11.0))
                                                    .font_weight(gpui::FontWeight::BOLD)
                                                    .text_color(status_color)
                                                    .w(px(14.0))
                                                    .flex_none()
                                                    .child(status_label),
                                            )
                                            .child(file_icon(file_icon_for_name(&file_name), 14.0))
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_baseline()
                                                    .gap(px(6.0))
                                                    .min_w_0()
                                                    .flex_1()
                                                    .child(
                                                        div()
                                                            .flex_none()
                                                            .text_size(px(12.0))
                                                            .text_color(if is_selected {
                                                                theme.text
                                                            } else {
                                                                theme.text_secondary
                                                            })
                                                            .child(file_name),
                                                    )
                                                    .when(!short_dir.is_empty(), |el| {
                                                        el.child(
                                                            div()
                                                                .truncate()
                                                                .min_w_0()
                                                                .flex_1()
                                                                .text_size(px(10.5))
                                                                .font_family(MONO_FAMILY)
                                                                .text_color(theme.text_ghost)
                                                                .child(short_dir.clone()),
                                                        )
                                                    }),
                                            ),
                                    )
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(4.0))
                                            .flex_none()
                                            .when_some(
                                                entry.additions.filter(|&a| a > 0),
                                                |el, adds| {
                                                    el.child(
                                                        div()
                                                            .text_size(px(10.0))
                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                            .text_color(theme.success)
                                                            .child(format!("+{}", adds)),
                                                    )
                                                },
                                            )
                                            .when_some(
                                                entry.deletions.filter(|&d| d > 0),
                                                |el, dels| {
                                                    el.child(
                                                        div()
                                                            .text_size(px(10.0))
                                                            .font_weight(gpui::FontWeight::MEDIUM)
                                                            .text_color(theme.danger)
                                                            .child(format!("-{}", dels)),
                                                    )
                                                },
                                            ),
                                    )
                            }))
                            .children(scoped_session_changes.iter().map(|entry| {
                                let on_select = on_select.clone();
                                let path = entry.path.clone();
                                let is_selected = selected_path.as_deref() == Some(&path);

                                let (status_label, status_color) = match entry.status.as_str() {
                                    "modified" => ("M", theme.warning),
                                    "added" => ("A", theme.success),
                                    "deleted" => ("D", theme.danger),
                                    _ => ("?", theme.text_tertiary),
                                };

                                let file_name = base_name(&entry.path).to_string();
                                let short_dir = short_parent_dir(&entry.path);
                                let tooltip_path = entry.path.clone();

                                div()
                                    .id(format!(
                                        "session-change-row-{}-{}",
                                        entry.path, entry.turn_index
                                    ))
                                    .flex()
                                    .items_center()
                                    .justify_between()
                                    .h(px(28.0))
                                    .w_full()
                                    .px(px(8.0))
                                    .rounded(px(4.0))
                                    .cursor_pointer()
                                    .hover(|s| s.bg(theme.overlay))
                                    .when(is_selected, |s| s.bg(theme.overlay_strong))
                                    .tooltip(Tooltip::text(tooltip_path))
                                    .on_click(move |_, window, cx| {
                                        (on_select)(path.clone(), window, cx);
                                    })
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(6.0))
                                            .min_w_0()
                                            .flex_1()
                                            .child(
                                                div()
                                                    .text_size(px(11.0))
                                                    .font_weight(gpui::FontWeight::BOLD)
                                                    .text_color(status_color)
                                                    .w(px(14.0))
                                                    .flex_none()
                                                    .child(status_label),
                                            )
                                            .child(file_icon(file_icon_for_name(&file_name), 14.0))
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_baseline()
                                                    .gap(px(6.0))
                                                    .min_w_0()
                                                    .flex_1()
                                                    .child(
                                                        div()
                                                            .flex_none()
                                                            .text_size(px(12.0))
                                                            .text_color(if is_selected {
                                                                theme.text
                                                            } else {
                                                                theme.text_secondary
                                                            })
                                                            .child(file_name),
                                                    )
                                                    .when(!short_dir.is_empty(), |el| {
                                                        el.child(
                                                            div()
                                                                .truncate()
                                                                .min_w_0()
                                                                .flex_1()
                                                                .text_size(px(10.5))
                                                                .font_family(MONO_FAMILY)
                                                                .text_color(theme.text_ghost)
                                                                .child(short_dir.clone()),
                                                        )
                                                    }),
                                            ),
                                    )
                                    .child(
                                        div()
                                            .flex()
                                            .items_center()
                                            .gap(px(4.0))
                                            .flex_none()
                                            .when(entry.reviewed, |el| {
                                                el.child(crate::primitives::icons::app_icon(
                                                    crate::primitives::icons::IconName::Check,
                                                    11.0,
                                                    theme.success,
                                                ))
                                            })
                                            .when(entry.additions > 0, |el| {
                                                el.child(
                                                    div()
                                                        .text_size(px(10.0))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.success)
                                                        .child(format!("+{}", entry.additions)),
                                                )
                                            })
                                            .when(entry.deletions > 0, |el| {
                                                el.child(
                                                    div()
                                                        .text_size(px(10.0))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.danger)
                                                        .child(format!("-{}", entry.deletions)),
                                                )
                                            }),
                                    )
                            }))
                            .into_any_element()
                    }),
            )
    }
}
