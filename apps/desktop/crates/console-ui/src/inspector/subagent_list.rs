//! Subagent List & Activity Timeline for the Right Inspector Panel.

use std::cell::RefCell;
use std::collections::HashSet;
use std::rc::Rc;

use console_core::types::SubagentInfo;
use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::chat::message_bubble::render_selectable_markdown;
use crate::markdown::render::{
    Ctx as MarkdownCtx, MarkdownView, Metrics, Palette, TranscriptSelection,
};
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct SubagentListView {
    subagents: Rc<Vec<SubagentInfo>>,
    expanded_subagents: HashSet<String>,
    selection: TranscriptSelection,
    on_toggle_subagent: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_copy_summary: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl SubagentListView {
    pub fn new(
        subagents: Rc<Vec<SubagentInfo>>,
        expanded_subagents: HashSet<String>,
        on_toggle_subagent: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_copy_summary: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            subagents,
            expanded_subagents,
            selection: TranscriptSelection::default(),
            on_toggle_subagent,
            on_copy_summary,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = selection;
        self
    }
}

impl RenderOnce for SubagentListView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let palette = Palette::from_theme(&theme);
        let on_toggle = self.on_toggle_subagent;
        let on_copy = self.on_copy_summary;

        div()
            .id("subagent-list-container")
            .flex_1()
            .w_full()
            .overflow_y_scroll()
            .p(px(8.0))
            .child(if self.subagents.is_empty() {
                div()
                    .flex_1()
                    .flex()
                    .flex_col()
                    .items_center()
                    .justify_center()
                    .py(px(48.0))
                    .px(px(16.0))
                    .gap(px(10.0))
                    .child(app_icon(IconName::Bot, 28.0, theme.text_ghost))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child("No Subagents Spawned"),
                    )
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_tertiary)
                            .text_align(gpui::TextAlign::Center)
                            .child("Subagents spawned by the assistant will stream activity here in real time"),
                    )
                    .into_any_element()
            } else {
                div()
                    .flex()
                    .flex_col()
                    .gap(px(8.0))
                    .children(self.subagents.iter().map(|subagent| {
                        let is_expanded = self.expanded_subagents.contains(&subagent.subagent_id);
                        let is_running = subagent.status == "running";
                        let is_error = subagent.status == "error" || subagent.status == "aborted";
                        let is_completed = subagent.status == "completed";

                        let subagent_id = subagent.subagent_id.clone();
                        let on_toggle = on_toggle.clone();

                        let status_label = if is_running {
                            "Running".to_string()
                        } else if is_completed {
                            "Done".to_string()
                        } else if subagent.status == "aborted" {
                            "Aborted".to_string()
                        } else {
                            "Failed".to_string()
                        };

                        let status_color = if is_running {
                            theme.accent
                        } else if is_completed {
                            theme.success
                        } else if subagent.status == "aborted" {
                            theme.warning
                        } else {
                            theme.danger
                        };

                        let mut card = div()
                            .id(format!("subagent-card-{}", subagent.subagent_id))
                            .w_full()
                            .flex()
                            .flex_col()
                            .rounded(px(6.0))
                            .bg(theme.surface)
                            .border_1()
                            .border_color(if is_running {
                                theme.accent.opacity(0.3)
                            } else {
                                theme.sidebar_border
                            });

                        // Header row (Role as title, clean status text without dot badge)
                        card = card.child(
                            div()
                                .id(format!("subagent-header-{}", subagent.subagent_id))
                                .flex()
                                .flex_col()
                                .gap(px(4.0))
                                .p(px(8.0))
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay))
                                .on_click(move |_, window, cx| {
                                    (on_toggle)(subagent_id.clone(), window, cx);
                                })
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .justify_between()
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(6.0))
                                                .min_w_0()
                                                .child(app_icon(IconName::Bot, 14.0, if is_running { theme.accent } else { theme.text_secondary }))
                                                .child(
                                                    div()
                                                        .text_size(px(12.0))
                                                        .font_weight(FontWeight::SEMIBOLD)
                                                        .text_color(theme.text)
                                                        .truncate()
                                                        .child(subagent.role.clone()),
                                                ),
                                        )
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(6.0))
                                                .when(!is_running, |el| {
                                                    let icon = if is_completed {
                                                        IconName::CircleCheck
                                                    } else {
                                                        IconName::Alert
                                                    };
                                                    el.child(app_icon(icon, 11.0, status_color))
                                                })
                                                .child(
                                                    div()
                                                        .text_size(px(10.5))
                                                        .font_weight(FontWeight::MEDIUM)
                                                        .text_color(status_color)
                                                        .child(status_label),
                                                )
                                                .child(app_icon(
                                                    if is_expanded {
                                                        IconName::ChevronDown
                                                    } else {
                                                        IconName::ChevronRight
                                                    },
                                                    12.0,
                                                    theme.text_tertiary,
                                                )),
                                        ),
                                )
                                .child(
                                    div()
                                        .text_size(px(11.0))
                                        .text_color(theme.text_tertiary)
                                        .truncate()
                                        .child(format!("\"{}\"", subagent.prompt)),
                                ),
                        );

                        // Expanded Details Body
                        if is_expanded {
                            let summary_text = subagent.summary.clone();
                            let on_copy_summary = on_copy.clone();
                            let summary_to_copy = summary_text.clone().unwrap_or_default();

                            card = card.child(
                                div()
                                    .flex()
                                    .flex_col()
                                    .gap(px(8.0))
                                    .p(px(8.0))
                                    .border_t_1()
                                    .border_color(theme.sidebar_border)
                                    .bg(theme.sidebar)
                                    // 1. Mission Prompt (Clean text display)
                                    .child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .gap(px(2.0))
                                            .child(
                                                div()
                                                    .text_size(px(10.0))
                                                    .font_weight(FontWeight::BOLD)
                                                    .text_color(theme.text_tertiary)
                                                    .child("MISSION PROMPT"),
                                            )
                                            .child(
                                                div()
                                                    .p(px(6.0))
                                                    .rounded(px(4.0))
                                                    .bg(theme.surface)
                                                    .text_size(px(11.5))
                                                    .text_color(theme.text_secondary)
                                                    .child(subagent.prompt.clone()),
                                            ),
                                    )
                                    // 2. Activity Timeline (Fast, clean native GPUI elements)
                                    .child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .gap(px(4.0))
                                            .child(
                                                div()
                                                    .text_size(px(10.0))
                                                    .font_weight(FontWeight::BOLD)
                                                    .text_color(theme.text_tertiary)
                                                    .child(format!("ACTIVITY TIMELINE ({})", subagent.activities.len())),
                                            )
                                            .child(if subagent.activities.is_empty() {
                                                div()
                                                    .py(px(4.0))
                                                    .text_size(px(11.0))
                                                    .text_color(theme.text_tertiary)
                                                    .child("No tool actions executed yet...")
                                                    .into_any_element()
                                            } else {
                                                div()
                                                    .flex()
                                                    .flex_col()
                                                    .gap(px(4.0))
                                                    .children(subagent.activities.iter().map(|act| {
                                                        let act_running = act.status == "running";
                                                        let act_completed = act.status == "completed";

                                                        let args_summary = act.args.as_ref().and_then(|v| {
                                                            let raw_str = if let Some(obj) = v.as_object() {
                                                                if let Some(cmd) = obj.get("command").or_else(|| obj.get("CommandLine")).and_then(|c| c.as_str()) {
                                                                    Some(cmd.to_string())
                                                                } else if let Some(path) = obj.get("path")
                                                                    .or_else(|| obj.get("filePath"))
                                                                    .or_else(|| obj.get("targetFile"))
                                                                    .or_else(|| obj.get("TargetFile"))
                                                                    .or_else(|| obj.get("absolutePath"))
                                                                    .or_else(|| obj.get("AbsolutePath"))
                                                                    .or_else(|| obj.get("SearchDirectory"))
                                                                    .or_else(|| obj.get("directory"))
                                                                    .and_then(|p| p.as_str()) {
                                                                    Some(path.to_string())
                                                                } else if let Some(pattern) = obj.get("pattern")
                                                                    .or_else(|| obj.get("Pattern"))
                                                                    .or_else(|| obj.get("Query"))
                                                                    .or_else(|| obj.get("query"))
                                                                    .and_then(|p| p.as_str()) {
                                                                    Some(pattern.to_string())
                                                                } else if let Some(prompt) = obj.get("Prompt")
                                                                    .or_else(|| obj.get("prompt"))
                                                                    .and_then(|p| p.as_str()) {
                                                                    Some(prompt.to_string())
                                                                } else if let Some(url) = obj.get("url")
                                                                    .or_else(|| obj.get("Url"))
                                                                    .and_then(|p| p.as_str()) {
                                                                    Some(url.to_string())
                                                                } else if let Some((first_key, first_val)) = obj.iter().next() {
                                                                    if let Some(s) = first_val.as_str() {
                                                                        Some(format!("{}: {}", first_key, s))
                                                                    } else {
                                                                        Some(format!("{}: {}", first_key, first_val))
                                                                    }
                                                                } else {
                                                                    None
                                                                }
                                                            } else if let Some(s) = v.as_str() {
                                                                Some(s.to_string())
                                                            } else {
                                                                None
                                                            };

                                                            raw_str.map(|s| {
                                                                if s.len() > 60 {
                                                                    format!("{}…", &s[..57])
                                                                } else {
                                                                    s
                                                                }
                                                            })
                                                        });

                                                        let (icon, color) = if act_running {
                                                            (IconName::Bot, theme.accent)
                                                        } else if act_completed {
                                                            (IconName::CircleCheck, theme.success)
                                                        } else {
                                                            (IconName::Alert, theme.danger)
                                                        };

                                                        div()
                                                            .flex()
                                                            .items_center()
                                                            .gap(px(6.0))
                                                            .px(px(6.0))
                                                            .py(px(4.0))
                                                            .rounded(px(4.0))
                                                            .bg(theme.surface)
                                                            .child(app_icon(icon, 12.0, color))
                                                            .child(
                                                                div()
                                                                    .px(px(4.0))
                                                                    .py(px(1.0))
                                                                    .rounded(px(3.0))
                                                                    .bg(theme.overlay)
                                                                    .text_size(px(11.0))
                                                                    .font_weight(FontWeight::MEDIUM)
                                                                    .font_family(crate::markdown::render::MONO_FAMILY)
                                                                    .text_color(theme.text_secondary)
                                                                    .child(act.tool_name.clone()),
                                                            )
                                                            .when_some(args_summary, |el, args| {
                                                                el.child(
                                                                    div()
                                                                        .flex_1()
                                                                        .min_w_0()
                                                                        .truncate()
                                                                        .text_size(px(11.0))
                                                                        .font_family(crate::markdown::render::MONO_FAMILY)
                                                                        .text_color(theme.text_tertiary)
                                                                        .child(args),
                                                                )
                                                            })
                                                            .when_some(act.error.clone(), |el, err| {
                                                                el.child(
                                                                    div()
                                                                        .text_size(px(10.5))
                                                                        .text_color(theme.danger)
                                                                        .truncate()
                                                                        .child(err),
                                                                )
                                                            })
                                                    }))
                                                    .into_any_element()
                                            }),
                                    )
                                    // 3. Summary Section (Only Summary uses Markdown renderer)
                                    .child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .gap(px(4.0))
                                            .child(
                                                div()
                                                    .flex()
                                                    .items_center()
                                                    .justify_between()
                                                    .child(
                                                        div()
                                                            .text_size(px(10.0))
                                                            .font_weight(FontWeight::BOLD)
                                                            .text_color(theme.text_tertiary)
                                                            .child("SUMMARY"),
                                                    )
                                                    .when(summary_text.is_some(), |el| {
                                                        let summary_to_copy = summary_to_copy.clone();
                                                        let on_copy_summary = on_copy_summary.clone();
                                                        el.child(
                                                            div()
                                                                .id("copy-summary-btn")
                                                                .flex()
                                                                .items_center()
                                                                .gap(px(4.0))
                                                                .px(px(6.0))
                                                                .py(px(2.0))
                                                                .rounded(px(3.0))
                                                                .bg(theme.overlay)
                                                                .cursor_pointer()
                                                                .hover(|s| s.bg(theme.overlay_strong))
                                                                .on_click(move |_, window, cx| {
                                                                    (on_copy_summary)(summary_to_copy.clone(), window, cx);
                                                                })
                                                                .child(app_icon(IconName::Copy, 10.0, theme.text_secondary))
                                                                .child(
                                                                    div()
                                                                        .text_size(px(10.0))
                                                                        .font_weight(FontWeight::MEDIUM)
                                                                        .text_color(theme.text_secondary)
                                                                        .child("Copy"),
                                                                ),
                                                        )
                                                    }),
                                            )
                                            .child(if let Some(ref sum) = summary_text {
                                                let sum_view = Rc::new(RefCell::new(MarkdownView::new()));
                                                let sum_ctx = MarkdownCtx::new(
                                                    format!("subagent-summary-{}", subagent.subagent_id),
                                                    &palette,
                                                    Metrics::BODY,
                                                    self.selection.clone(),
                                                );

                                                div()
                                                    .p(px(6.0))
                                                    .rounded(px(4.0))
                                                    .bg(theme.surface)
                                                    .child(render_selectable_markdown(
                                                        sum,
                                                        Some(&sum_view),
                                                        &sum_ctx,
                                                        is_running,
                                                    ))
                                                    .into_any_element()
                                            } else if is_running {
                                                div()
                                                    .py(px(4.0))
                                                    .text_size(px(11.0))
                                                    .text_color(theme.text_tertiary)
                                                    .child("(Awaiting subagent completion...)")
                                                    .into_any_element()
                                            } else if is_error {
                                                div()
                                                    .p(px(6.0))
                                                    .rounded(px(4.0))
                                                    .bg(theme.surface)
                                                    .text_size(px(11.0))
                                                    .text_color(theme.danger)
                                                    .child(subagent.error.clone().unwrap_or_else(|| "Subagent encountered an error".to_string()))
                                                    .into_any_element()
                                            } else {
                                                div().into_any_element()
                                            }),
                                    ),
                            );
                        }

                        card.into_any_element()
                    }))
                    .into_any_element()
            })
    }
}
