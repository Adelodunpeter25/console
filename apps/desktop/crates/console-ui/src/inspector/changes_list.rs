//! Git & Session Changes List with Diff Badges for the Right Inspector.

use std::rc::Rc;

use console_core::types::{GitFileEntry, SessionFileChange};
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct ChangesListView {
    working_changes: Vec<GitFileEntry>,
    session_changes: Vec<SessionFileChange>,
    selected_path: Option<String>,
    on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl ChangesListView {
    pub fn new(
        working_changes: Vec<GitFileEntry>,
        session_changes: Vec<SessionFileChange>,
        selected_path: Option<String>,
        on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            working_changes,
            session_changes,
            selected_path,
            on_select_file,
        }
    }
}

impl RenderOnce for ChangesListView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_select = self.on_select_file;
        let selected_path = self.selected_path;

        let has_changes = !self.working_changes.is_empty() || !self.session_changes.is_empty();

        div()
            .id("changes-list-container")
            .flex_1()
            .w_full()
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
                    .children(self.working_changes.into_iter().map(|entry| {
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

                        let file_name = std::path::Path::new(&entry.path)
                            .file_name()
                            .and_then(|n| n.to_str())
                            .unwrap_or(&entry.path)
                            .to_string();

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
                                            .truncate()
                                            .text_size(px(12.0))
                                            .text_color(if is_selected {
                                                theme.text
                                            } else {
                                                theme.text_secondary
                                            })
                                            .child(file_name),
                                    ),
                            )
                            .child(
                                div()
                                    .flex()
                                    .items_center()
                                    .gap(px(4.0))
                                    .flex_none()
                                    .when_some(entry.additions.filter(|&a| a > 0), |el, adds| {
                                        el.child(
                                            div()
                                                .text_size(px(10.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.success)
                                                .child(format!("+{}", adds)),
                                        )
                                    })
                                    .when_some(entry.deletions.filter(|&d| d > 0), |el, dels| {
                                        el.child(
                                            div()
                                                .text_size(px(10.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.danger)
                                                .child(format!("-{}", dels)),
                                        )
                                    }),
                            )
                    }))
                    .into_any_element()
            })
    }
}
