//! "View all" session changes review tab: every changed file in a scope,
//! stacked in one scrollable column with per-file mark-as-reviewed state
//! (§6 of `docs/session-changes-turn-diff-plan.md`).

use std::rc::Rc;

use console_core::types::{ChangesScope, SessionFileChange};
use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::chat::diff_view::DiffView;
use crate::markdown::render::MONO_FAMILY;
use crate::primitives::icons::{IconName, app_icon};
use crate::primitives::{base_name, file_type_icon};
use crate::theme::Theme;
use crate::utils::short_parent_dir;

#[derive(IntoElement)]
pub struct ReviewTab {
    scope: ChangesScope,
    changes: Rc<Vec<SessionFileChange>>,
    collapsed: std::collections::HashSet<String>,
    on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_toggle_reviewed: Rc<dyn Fn(String, u64, &mut Window, &mut App) + 'static>,
}

impl ReviewTab {
    pub fn new(
        scope: ChangesScope,
        changes: Rc<Vec<SessionFileChange>>,
        collapsed: std::collections::HashSet<String>,
        on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_toggle_reviewed: Rc<dyn Fn(String, u64, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            scope,
            changes,
            collapsed,
            on_toggle_collapsed,
            on_toggle_reviewed,
        }
    }
}

impl RenderOnce for ReviewTab {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let scoped = console_core::types::filter_changes_for_scope(&self.changes, self.scope);
        let total_additions: u64 = scoped.iter().map(|c| c.additions).sum();
        let total_deletions: u64 = scoped.iter().map(|c| c.deletions).sum();
        let file_count = scoped.len();
        let on_toggle_collapsed = self.on_toggle_collapsed;
        let on_toggle_reviewed = self.on_toggle_reviewed;

        div()
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.canvas)
            .child(
                div()
                    .flex_none()
                    .flex()
                    .items_center()
                    .gap(px(10.0))
                    .px(px(16.0))
                    .py(px(12.0))
                    .border_b_1()
                    .border_color(theme.sidebar_border)
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child(format!(
                                "{} — {} file{} changed",
                                self.scope.label(),
                                file_count,
                                if file_count == 1 { "" } else { "s" }
                            )),
                    )
                    .when(total_additions > 0, |el| {
                        el.child(
                            div()
                                .text_size(px(12.0))
                                .font_weight(FontWeight::SEMIBOLD)
                                .text_color(theme.success)
                                .child(format!("+{}", total_additions)),
                        )
                    })
                    .when(total_deletions > 0, |el| {
                        el.child(
                            div()
                                .text_size(px(12.0))
                                .font_weight(FontWeight::SEMIBOLD)
                                .text_color(theme.danger)
                                .child(format!("-{}", total_deletions)),
                        )
                    }),
            )
            .child(
                div()
                    .id("changes-review-scroll")
                    .flex_1()
                    .min_h_0()
                    .overflow_y_scroll()
                    .child(if scoped.is_empty() {
                        div()
                            .flex()
                            .items_center()
                            .justify_center()
                            .py(px(48.0))
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .child("No changes in this scope")
                            .into_any_element()
                    } else {
                        div()
                            .flex()
                            .flex_col()
                            .children(scoped.iter().map(|entry| {
                                let is_collapsed = self.collapsed.contains(&entry.path);
                                let file_name = base_name(&entry.path).to_string();
                                let short_dir = short_parent_dir(&entry.path);
                                let path_for_toggle = entry.path.clone();
                                let path_for_reviewed = entry.path.clone();
                                let turn_index = entry.turn_index;
                                let on_toggle_collapsed = on_toggle_collapsed.clone();
                                let on_toggle_reviewed = on_toggle_reviewed.clone();
                                let reviewed = entry.reviewed;

                                let diff_result = entry
                                    .diff_text
                                    .as_deref()
                                    .map(console_core::utils::diff::parse_unified_diff)
                                    .unwrap_or_default();

                                div()
                                    .flex()
                                    .flex_col()
                                    .border_b_1()
                                    .border_color(theme.sidebar_border)
                                    .when(reviewed, |el| el.opacity(0.6))
                                    .child(
                                        div()
                                            .id(gpui::ElementId::Name(
                                                format!(
                                                    "review-header-{}-{}",
                                                    entry.path, entry.turn_index
                                                )
                                                .into(),
                                            ))
                                            .flex()
                                            .items_center()
                                            .justify_between()
                                            .gap(px(8.0))
                                            .px(px(16.0))
                                            .py(px(10.0))
                                            .cursor_pointer()
                                            .hover(|s| s.bg(theme.overlay))
                                            .on_click(move |_, window, cx| {
                                                (on_toggle_collapsed)(
                                                    path_for_toggle.clone(),
                                                    window,
                                                    cx,
                                                );
                                            })
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_center()
                                                    .gap(px(8.0))
                                                    .min_w_0()
                                                    .flex_1()
                                                    .child(app_icon(
                                                        if is_collapsed {
                                                            IconName::ChevronRight
                                                        } else {
                                                            IconName::ChevronDown
                                                        },
                                                        11.0,
                                                        theme.text_tertiary,
                                                    ))
                                                    .child(file_type_icon(&entry.path, 14.0))
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
                                                                    .text_size(px(12.5))
                                                                    .font_weight(
                                                                        FontWeight::MEDIUM,
                                                                    )
                                                                    .text_color(theme.text)
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
                                                                        .text_color(
                                                                            theme.text_ghost,
                                                                        )
                                                                        .child(short_dir.clone()),
                                                                )
                                                            }),
                                                    ),
                                            )
                                            .child(
                                                div()
                                                    .id(gpui::ElementId::Name(
                                                        format!(
                                                            "review-mark-{}-{}",
                                                            entry.path, entry.turn_index
                                                        )
                                                        .into(),
                                                    ))
                                                    .flex()
                                                    .items_center()
                                                    .gap(px(6.0))
                                                    .px(px(8.0))
                                                    .h(px(24.0))
                                                    .rounded(px(6.0))
                                                    .cursor_pointer()
                                                    .when(reviewed, |el| {
                                                        el.bg(theme.overlay_strong)
                                                    })
                                                    .when(!reviewed, |el| {
                                                        el.border_1()
                                                            .border_color(theme.border_strong)
                                                    })
                                                    .hover(|s| s.bg(theme.overlay))
                                                    .on_click(move |event, window, cx| {
                                                        cx.stop_propagation();
                                                        let _ = event;
                                                        (on_toggle_reviewed)(
                                                            path_for_reviewed.clone(),
                                                            turn_index,
                                                            window,
                                                            cx,
                                                        );
                                                    })
                                                    .child(app_icon(
                                                        IconName::Check,
                                                        10.0,
                                                        if reviewed {
                                                            theme.success
                                                        } else {
                                                            theme.text_tertiary
                                                        },
                                                    ))
                                                    .child(
                                                        div()
                                                            .text_size(px(10.5))
                                                            .font_weight(FontWeight::MEDIUM)
                                                            .text_color(if reviewed {
                                                                theme.success
                                                            } else {
                                                                theme.text_tertiary
                                                            })
                                                            .child(if reviewed {
                                                                "Reviewed"
                                                            } else {
                                                                "Mark as reviewed"
                                                            }),
                                                    ),
                                            ),
                                    )
                                    .when(!is_collapsed, |el| {
                                        el.child(
                                            div()
                                                .px(px(16.0))
                                                .pb(px(12.0))
                                                .child(
                                                    DiffView::new(
                                                        format!(
                                                            "review-{}-{}",
                                                            entry.path, entry.turn_index
                                                        ),
                                                        diff_result,
                                                    )
                                                    .file_path(entry.path.clone()),
                                                ),
                                        )
                                    })
                            }))
                            .into_any_element()
                    }),
            )
    }
}
