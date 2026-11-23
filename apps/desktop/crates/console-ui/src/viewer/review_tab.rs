//! "View all" session changes review tab: one row per changed file, expanded
//! into an editor diff with the same comment gutter as the full diff tab.

use std::rc::Rc;

use console_core::types::{ChangesScope, SessionFileChange};
use editor_ui::DiffCommentTarget;
use gpui::{
    App, Entity, FontWeight, InteractiveElement, IntoElement, ListState, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, list, prelude::FluentBuilder, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::primitives::icons::{IconName, app_icon};
use crate::primitives::{base_name, file_type_icon};
use crate::theme::Theme;
use crate::utils::short_parent_dir;
use crate::viewer::diff_viewer::{DiffViewer, EditorDiff, build_editor_diff_capped_sized};

/// Code size and row height of a file's diff in this view.
const REVIEW_FONT_SIZE: f32 = 11.0;
const REVIEW_LINE_HEIGHT: f32 = 18.0;
/// Height of a collapsed file row.
const REVIEW_ROW_HEIGHT: f32 = 34.0;

/// Longest a file's diff grows inside the review tab before it scrolls
/// internally. The editor diff view lays out every row it is given room for,
/// so an unbounded height would render a huge diff's rows all at once.
const REVIEW_DIFF_MAX_HEIGHT: f32 = 800.0;

/// Initial diff lines to load before offering a clickable "Show N more lines"
/// toggle. Keeps tree-sitter highlighting and widest-line scans bounded on
/// initial open for huge files without permanently truncating them.
const REVIEW_DIFF_PREVIEW_LINES: usize = 1000;

pub type ReviewCommentTargetHandler = Rc<dyn Fn(String, DiffCommentTarget) + 'static>;
pub type ReviewSyncCommentsHandler =
    Rc<dyn Fn(String, &Entity<editor_ui::DiffView>, &mut Window, &mut App) -> u32 + 'static>;

#[derive(IntoElement)]
pub struct ReviewTab {
    scope: ChangesScope,
    changes: Rc<[SessionFileChange]>,
    expanded: std::collections::HashSet<String>,
    on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_comment_target: ReviewCommentTargetHandler,
    sync_comments: ReviewSyncCommentsHandler,
    list_state: Option<ListState>,
}

impl ReviewTab {
    pub fn new(
        scope: ChangesScope,
        changes: Rc<[SessionFileChange]>,
        expanded: std::collections::HashSet<String>,
        on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_comment_target: ReviewCommentTargetHandler,
        sync_comments: ReviewSyncCommentsHandler,
    ) -> Self {
        Self {
            scope,
            changes,
            expanded,
            on_toggle_collapsed,
            on_comment_target,
            sync_comments,
            list_state: None,
        }
    }

    pub fn list_state(mut self, list_state: ListState) -> Self {
        self.list_state = Some(list_state);
        self
    }
}

impl RenderOnce for ReviewTab {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let scoped = console_core::types::filter_changes_for_scope(&self.changes, self.scope);
        let total_additions: u32 = scoped.iter().map(|c| c.additions).sum();
        let total_deletions: u32 = scoped.iter().map(|c| c.deletions).sum();
        let file_count = scoped.len();
        let on_toggle_collapsed = self.on_toggle_collapsed;
        let on_comment_target = self.on_comment_target;
        let sync_comments = self.sync_comments;

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
                                "{} · {} file{} changed",
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
            .child(if scoped.is_empty() {
                div()
                    .id("changes-review-scroll")
                    .flex_1()
                    .min_h_0()
                    .overflow_y_scroll()
                    .child(
                        div()
                            .flex()
                            .items_center()
                            .justify_center()
                            .py(px(48.0))
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .child("No changes in this scope"),
                    )
                    .into_any_element()
            } else if let Some(list_state) = self.list_state {
                let scoped = Rc::new(scoped);
                let expanded = Rc::new(self.expanded);
                let on_toggle_collapsed = on_toggle_collapsed.clone();
                let on_comment_target = on_comment_target.clone();
                let sync_comments = sync_comments.clone();

                let list_state_for_rows = list_state.clone();
                div()
                    .id("changes-review-scroll")
                    .flex_1()
                    .min_h_0()
                    .size_full()
                    .child(
                        list(list_state, move |ix, window, cx| {
                            let entry = &scoped[ix];
                            let is_collapsed = !expanded.contains(&entry.path);
                            render_review_entry(
                                ix,
                                entry,
                                is_collapsed,
                                Some(&list_state_for_rows),
                                &on_toggle_collapsed,
                                &on_comment_target,
                                &sync_comments,
                                window,
                                cx,
                            )
                        })
                        .size_full(),
                    )
                    .into_any_element()
            } else {
                div()
                    .id("changes-review-scroll")
                    .flex_1()
                    .min_h_0()
                    .overflow_y_scroll()
                    .child(
                        div()
                            .flex()
                            .flex_col()
                            .children(scoped.iter().enumerate().map(|(ix, entry)| {
                                let is_collapsed = !self.expanded.contains(&entry.path);
                                render_review_entry(
                                    ix,
                                    entry,
                                    is_collapsed,
                                    None,
                                    &on_toggle_collapsed,
                                    &on_comment_target,
                                    &sync_comments,
                                    window,
                                    cx,
                                )
                            })),
                    )
                    .into_any_element()
            })
    }
}

#[allow(clippy::too_many_arguments)]
fn render_review_entry(
    ix: usize,
    entry: &SessionFileChange,
    is_collapsed: bool,
    list_state: Option<&ListState>,
    on_toggle_collapsed: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_comment_target: &ReviewCommentTargetHandler,
    sync_comments: &ReviewSyncCommentsHandler,
    window: &mut Window,
    cx: &mut App,
) -> gpui::AnyElement {
    let theme = Theme::current(cx);
    let file_name = base_name(&entry.path);
    let short_dir = short_parent_dir(&entry.path);
    let path_for_toggle = entry.path.clone();
    let on_toggle_collapsed = on_toggle_collapsed.clone();
    let (status_label, status_color) = match entry.status.as_str() {
        "modified" => ("M", theme.warning),
        "added" => ("A", theme.success),
        "deleted" => ("D", theme.danger),
        _ => ("?", theme.text_tertiary),
    };
    let additions = entry.additions;
    let deletions = entry.deletions;

    // One cached editor diff view per file. The key carries the row's update
    // time and diff length, so a changed diff builds a fresh view.
    let diff_info: Option<(EditorDiff, usize)> = if is_collapsed {
        None
    } else {
        entry
            .diff_text
            .as_deref()
            .filter(|text| !text.trim().is_empty())
            .map(|text| {
                let key = format!(
                    "review-diff-{}-{}-{}",
                    entry.path,
                    entry.updated_at,
                    text.len()
                );
                let comment_path = entry.path.clone();
                let comment_handler = on_comment_target.clone();
                let state =
                    window.use_keyed_state(gpui::ElementId::Name(key.into()), cx, |_, cx| {
                        let (diff, total) = build_editor_diff_capped_sized(
                            text,
                            &entry.path,
                            &theme,
                            REVIEW_DIFF_PREVIEW_LINES,
                            REVIEW_FONT_SIZE,
                            REVIEW_LINE_HEIGHT,
                            cx,
                        );
                        diff.view.update(cx, |view, _| {
                            view.on_comment_target(move |target| {
                                comment_handler(comment_path.clone(), target);
                            });
                        });
                        (diff, total)
                    });
                state.read(cx).clone()
            })
    };

    div()
        .w_full()
        .flex()
        .flex_col()
        .child(
            div()
                .id(gpui::ElementId::Name(
                    format!("review-header-{}", entry.path).into(),
                ))
                .flex()
                .items_center()
                .justify_between()
                .gap(px(8.0))
                .px(px(14.0))
                .h(px(REVIEW_ROW_HEIGHT))
                .flex_none()
                .cursor_pointer()
                .hover(|s| s.bg(theme.overlay))
                .on_click(move |_, window, cx| {
                    (on_toggle_collapsed)(path_for_toggle.clone(), window, cx);
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
                                        .text_size(px(12.0))
                                        .font_weight(FontWeight::MEDIUM)
                                        .text_color(theme.text)
                                        .child(file_name.to_string()),
                                )
                                .when(!short_dir.is_empty(), |el| {
                                    el.child(
                                        div()
                                            .truncate()
                                            .min_w_0()
                                            .flex_1()
                                            .text_size(px(10.0))
                                            .font_family(MONO_FAMILY)
                                            .text_color(theme.text_tertiary)
                                            .child(short_dir),
                                    )
                                }),
                        ),
                )
                .child(
                    div()
                        .flex()
                        .flex_none()
                        .items_center()
                        .gap(px(8.0))
                        .when(additions > 0, |el| {
                            el.child(
                                div()
                                    .text_size(px(11.0))
                                    .font_family(MONO_FAMILY)
                                    .text_color(theme.success)
                                    .child(format!("+{additions}")),
                            )
                        })
                        .when(deletions > 0, |el| {
                            el.child(
                                div()
                                    .text_size(px(11.0))
                                    .font_family(MONO_FAMILY)
                                    .text_color(theme.danger)
                                    .child(format!("-{deletions}")),
                            )
                        })
                        .child(
                            div()
                                .flex()
                                .items_center()
                                .justify_center()
                                .w(px(18.0))
                                .h(px(18.0))
                                .rounded(px(4.0))
                                .border_1()
                                .border_color(status_color)
                                .text_size(px(10.0))
                                .font_weight(FontWeight::BOLD)
                                .text_color(status_color)
                                .child(status_label),
                        ),
                ),
        )
        .when_some(diff_info, |el, (diff, total_lines)| {
            let overlay_rows = sync_comments(entry.path.clone(), &diff.view, window, cx);
            let raw_height =
                (diff.line_count + overlay_rows as usize) as f32 * REVIEW_LINE_HEIGHT + 8.0;
            let is_vertically_scrollable = raw_height > REVIEW_DIFF_MAX_HEIGHT;
            let height = raw_height.min(REVIEW_DIFF_MAX_HEIGHT);
            let remaining = total_lines.saturating_sub(diff.line_count);
            let show_toggle = remaining > 0;
            let diff_box = div()
                .id(gpui::ElementId::Name(
                    format!("review-diff-box-{}", entry.path).into(),
                ))
                .w_full()
                .h(px(height))
                .overflow_hidden()
                .when(is_vertically_scrollable, |box_el| {
                    box_el.on_scroll(|_, _, cx| cx.stop_propagation())
                })
                .child(DiffViewer::new(diff.view));
            el.child(diff_box).when(show_toggle, |el| {
                let list_state = list_state.cloned();
                el.child(
                    div()
                        .id(gpui::ElementId::Name(
                            format!("review-diff-more-{}", entry.path).into(),
                        ))
                        .w_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .gap(px(4.0))
                        .py(px(8.0))
                        .cursor_pointer()
                        .hover(|s| s.bg(theme.overlay))
                        .on_click(move |_, _window, _cx| {
                            if let Some(list_state) = &list_state {
                                list_state.splice(ix..ix + 1, 1);
                            }
                        })
                        .child(app_icon(IconName::ChevronDown, 12.0, theme.text_secondary))
                        .child(
                            div()
                                .text_size(px(12.0))
                                .text_color(theme.text_secondary)
                                .child(format!("Show {remaining} more lines")),
                        ),
                )
            })
        })
        .into_any_element()
}
