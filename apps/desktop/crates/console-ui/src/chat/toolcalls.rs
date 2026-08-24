//! Grouped tool-call activity for the transcript.
//!
//! This is the GPUI equivalent of the desktop `RunActivity` component: one
//! collapsible run summary, consecutive calls grouped by tool name, and
//! independently expandable arguments/results. Results are always matched by
//! `tool_call_id`, never by their position in the transcript.

use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use console_core::{
    ActivityEvent, ToolCall, ToolCallEntry, ToolResult, diff_lines, extract_edit_args,
};
use gpui::{
    AnyElement, App, ElementId, FontWeight, IntoElement, ParentElement, RenderOnce, Styled, Window,
    div, prelude::*, px, transparent_black,
};

use crate::chat::message_bubble::render_selectable_markdown;
use crate::chat::{DiffView, ThinkingBlock, WorkingIndicator};
use crate::markdown::render::{
    Ctx as MarkdownCtx, MarkdownView, Metrics, Palette, TranscriptSelection, plain_text,
};
use crate::primitives::{IconName, activity_icon, app_icon, icon};
use crate::theme::Theme;
use crate::utils::time::normalize_unix_timestamp;

/// Persistent disclosure state owned by a transcript entity.
#[derive(Clone, Default)]
pub struct ToolCallsState {
    pub expanded_runs: HashSet<String>,
    pub collapsed_runs: HashSet<String>,
    pub expanded_calls: HashSet<String>,
    /// IDs of thinking blocks the user has expanded inside a run activity.
    /// Thinking is collapsed by default, like the desktop app.
    pub thinking_expanded: HashSet<String>,
    /// Markdown views for run-activity thinking/text events, keyed by event
    /// id. Owned by the transcript so virtualization does not reset a
    /// streaming render when a row remounts.
    pub markdown_views: HashMap<String, Rc<RefCell<MarkdownView>>>,
    /// Memoized diffs for edit-file tool calls, keyed by call id.
    pub diff_cache: HashMap<String, (serde_json::Value, Option<console_core::DiffResult>)>,
}

impl ToolCallsState {
    pub fn clear(&mut self) {
        self.expanded_runs.clear();
        self.collapsed_runs.clear();
        self.expanded_calls.clear();
        self.thinking_expanded.clear();
        self.markdown_views.clear();
        self.diff_cache.clear();
    }
}

/// Actions emitted by the component so the owning transcript can update its
/// entity and trigger a repaint.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ToolCallsAction {
    ToggleRun { run_id: String, expanded: bool },
    ToggleCall { call_id: String, expanded: bool },
    ToggleThinking { id: String, expanded: bool },
}

/// Join separately persisted tool results to their calls by stable backend ID.
pub fn attach_results_by_id(
    entries: Vec<ToolCallEntry>,
    results: impl IntoIterator<Item = ToolResult>,
) -> Vec<ToolCallEntry> {
    let results = results
        .into_iter()
        .map(|result| (result.tool_call_id.clone(), result))
        .collect::<std::collections::HashMap<_, _>>();
    entries
        .into_iter()
        .map(|mut entry| {
            entry.result = results.get(&entry.call.id).cloned();
            entry
        })
        .collect()
}

#[derive(Clone)]
struct ToolGroup {
    name: String,
    entries: Vec<ToolCallEntry>,
}

/// A grouped, collapsible run activity surface.
#[derive(IntoElement)]
pub struct ToolCalls {
    run_id: String,
    events: Vec<ActivityEvent>,
    working: bool,
    started_at: Option<i64>,
    elapsed_seconds: u64,
    state: Rc<RefCell<ToolCallsState>>,
    /// Shared transcript selection so run-activity text, arguments, and
    /// results join the same selection state as the surrounding messages.
    selection: TranscriptSelection,
    on_action: Option<Rc<dyn Fn(ToolCallsAction, &mut Window, &mut App) + 'static>>,
}

impl ToolCalls {
    pub fn new(
        run_id: impl Into<String>,
        events: Vec<ActivityEvent>,
        working: bool,
        started_at: Option<i64>,
        elapsed_seconds: u64,
        state: Rc<RefCell<ToolCallsState>>,
    ) -> Self {
        Self {
            run_id: run_id.into(),
            events,
            working,
            started_at,
            elapsed_seconds,
            state,
            selection: TranscriptSelection::default(),
            on_action: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = selection;
        self
    }

    pub fn on_action(
        mut self,
        handler: impl Fn(ToolCallsAction, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_action = Some(Rc::new(handler));
        self
    }

    fn groups(entries: &[ToolCallEntry]) -> Vec<ToolGroup> {
        let mut groups: Vec<ToolGroup> = Vec::new();
        for entry in entries {
            if let Some(group) = groups.last_mut()
                && group.name == entry.call.name
            {
                group.entries.push(entry.clone());
                continue;
            }
            groups.push(ToolGroup {
                name: entry.call.name.clone(),
                entries: vec![entry.clone()],
            });
        }
        groups
    }

    fn call_label(name: &str) -> String {
        match name {
            "readFile" | "read_file" => "Read File".into(),
            "writeFile" | "write_file" => "Write File".into(),
            "batchWrite" | "batch_write" => "Batch Write".into(),
            "editFile" | "edit_file" | "str_replace" => "Edit File".into(),
            "bash" | "shell" | "command" => "Run Command".into(),
            "grep" | "search_files" => "Search Code".into(),
            "glob" | "list_files" => "Find Files".into(),
            "listDir" | "list_dir" | "ls" => "List Directory".into(),
            "fetch" => "Fetch URL".into(),
            "webSearch" | "web_search" => "Web Search".into(),
            "subagent" => "Subagent".into(),
            "ask" => "Ask Question".into(),
            "todo" => "Todo".into(),
            name => name.to_owned(),
        }
    }

    fn argument_summary(call: &ToolCall) -> Option<String> {
        let object = call.arguments.as_object()?;
        for key in [
            "path",
            "filePath",
            "command",
            "pattern",
            "query",
            "url",
            "directory",
        ] {
            if let Some(value) = object.get(key).and_then(|value| value.as_str()) {
                return Some(truncate(value, 72));
            }
        }
        if let Some(paths) = object.get("paths").and_then(|value| value.as_array()) {
            return Some(format!("{} files", paths.len()));
        }
        if let Some(operations) = object.get("operations").and_then(|value| value.as_array()) {
            return Some(format!("{} operations", operations.len()));
        }
        None
    }

    fn formatted_result(result: &ToolResult) -> String {
        let mut content = result.content.clone();
        while let Some(inner) = content
            .as_object()
            .and_then(|object| object.get("content"))
            .cloned()
        {
            content = inner;
        }
        if let Some(text) = content.as_str() {
            text.to_owned()
        } else if let Some(text) = content
            .as_object()
            .and_then(|object| object.get("text"))
            .and_then(|value| value.as_str())
        {
            text.to_owned()
        } else if let Some(items) = content.as_array() {
            items
                .iter()
                .filter_map(|item| {
                    item.as_str().map(str::to_owned).or_else(|| {
                        item.as_object()
                            .and_then(|object| object.get("text"))
                            .and_then(|value| value.as_str())
                            .map(str::to_owned)
                    })
                })
                .collect::<Vec<_>>()
                .join("\n")
        } else {
            serde_json::to_string_pretty(&content).unwrap_or_else(|_| content.to_string())
        }
    }

    fn status_icon(entry: &ToolCallEntry, theme: &Theme) -> (IconName, gpui::Hsla) {
        match entry.result.as_ref() {
            Some(result) if result.is_error.unwrap_or(false) => (IconName::Alert, theme.danger),
            Some(_) => (IconName::CircleCheck, theme.success),
            None => (IconName::LoaderCircle, theme.text_tertiary),
        }
    }

    fn group_failed(group: &ToolGroup) -> bool {
        group.entries.iter().any(|entry| {
            entry
                .result
                .as_ref()
                .is_some_and(|result| result.is_error.unwrap_or(false))
        })
    }

    fn group_complete(group: &ToolGroup) -> bool {
        group.entries.iter().all(|entry| entry.result.is_some())
    }

    fn call_row(
        &self,
        entry: ToolCallEntry,
        theme: Theme,
        first: bool,
        on_action: Option<Rc<dyn Fn(ToolCallsAction, &mut Window, &mut App) + 'static>>,
    ) -> AnyElement {
        let call_id = entry.call.id.clone();
        let open = self.state.borrow().expanded_calls.contains(&call_id);
        let summary = Self::argument_summary(&entry.call);
        let (status_icon, status_color) = Self::status_icon(&entry, &theme);
        let call_id_for_action = call_id.clone();
        let is_edit = is_edit_file(&entry.call.name);
        let diff = if is_edit {
            let mut cache = self.state.borrow_mut();
            if let Some((cached_args, cached_diff)) = cache.diff_cache.get(&call_id) {
                if cached_args == &entry.call.arguments {
                    cached_diff.clone()
                } else {
                    let computed = extract_edit_args(&entry.call.arguments)
                        .map(|(old, new)| diff_lines(old, new, 3));
                    cache
                        .diff_cache
                        .insert(call_id.clone(), (entry.call.arguments.clone(), computed.clone()));
                    computed
                }
            } else {
                let computed = extract_edit_args(&entry.call.arguments)
                    .map(|(old, new)| diff_lines(old, new, 3));
                cache
                    .diff_cache
                    .insert(call_id.clone(), (entry.call.arguments.clone(), computed.clone()));
                computed
            }
        } else {
            None
        };
        let has_diff = diff.is_some();
        let mut row = div()
            .id(ElementId::Name(format!("tool-call-{call_id}").into()))
            .w_full()
            .flex()
            .flex_col()
            .when(!first, |element| {
                element.border_t_1().border_color(theme.border)
            })
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .px(px(10.0))
                    .py(px(6.0))
                    .cursor_pointer()
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(transparent_black())
                    .id(ElementId::Name(
                        format!("tool-call-header-{call_id}").into(),
                    ))
                    .on_click(move |_, window, cx| {
                        if let Some(on_action) = &on_action {
                            on_action(
                                ToolCallsAction::ToggleCall {
                                    call_id: call_id_for_action.clone(),
                                    expanded: !open,
                                },
                                window,
                                cx,
                            );
                        }
                    })
                    .child(icon(
                        activity_icon(&entry.call.name),
                        13.0,
                        theme.text_tertiary,
                    ))
                    .child(
                        div()
                            .flex_none()
                            .text_size(px(12.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child(Self::call_label(&entry.call.name)),
                    )
                    .when_some(summary, |element, summary| {
                        element.child(
                            div()
                                .min_w_0()
                                .truncate()
                                .font_family("GeistMono")
                                .text_size(px(11.5))
                                .text_color(theme.text_tertiary)
                                .child(summary),
                        )
                    })
                    .when_some(diff.as_ref(), |element, d| {
                        element.child(
                            div()
                                .flex()
                                .items_center()
                                .gap(px(4.0))
                                .text_size(px(10.0))
                                .font_weight(FontWeight::SEMIBOLD)
                                .child(div().text_color(theme.success).child(format!("+{}", d.added)))
                                .child(div().text_color(theme.danger).child(format!("-{}", d.removed))),
                        )
                    })
                    .child(
                        div()
                            .ml_auto()
                            .flex_none()
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(
                                status_icon,
                                13.0,
                                status_color,
                            ))
                            // Reserve the chevron's width so this row's status
                            // icon aligns with group rows that show a chevron
                            // after the status icon.
                            .child(div().w(px(12.0))),
                    ),
            );

        if open {
            row = row.child(
                div()
                    .px(px(10.0))
                    .pb(px(8.0))
                    .flex()
                    .flex_col()
                    .gap(px(6.0))
                    .when_some(diff, |element, d| {
                        element.child(DiffView::new(call_id.clone(), d))
                    })
                    .when(!has_diff, |element| {
                        let arguments = serde_json::to_string_pretty(&entry.call.arguments)
                            .unwrap_or_else(|_| entry.call.arguments.to_string());
                        element.child(self.section(&call_id, "Arguments", arguments, theme))
                    })
                    .when_some(entry.result, |element, result| {
                        element.child(self.section(
                            &call_id,
                            "Result",
                            Self::formatted_result(&result),
                            theme,
                        ))
                    }),
            );
        }
        row.into_any_element()
    }

    fn group_row(
        &self,
        group: ToolGroup,
        theme: Theme,
        on_action: Option<Rc<dyn Fn(ToolCallsAction, &mut Window, &mut App) + 'static>>,
    ) -> AnyElement {
        let group_key = format!("group:{}", group.entries[0].call.id);
        let open = self.state.borrow().expanded_calls.contains(&group_key);
        let failed = Self::group_failed(&group);
        let complete = Self::group_complete(&group);
        let label = Self::call_label(&group.name);
        let action_id = group_key.clone();
        let header_action = on_action.clone();
        let status = if failed {
            (IconName::Alert, theme.danger)
        } else if complete {
            (IconName::CircleCheck, theme.success)
        } else {
            (IconName::LoaderCircle, theme.text_tertiary)
        };
        let mut row = div()
            .id(ElementId::Name(format!("tool-group-{group_key}").into()))
            .w_full()
            .flex()
            .flex_col()
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .px(px(10.0))
                    .py(px(6.0))
                    .cursor_pointer()
                    .rounded(px(6.0))
                    .border_1()
                    .border_color(transparent_black())
                    .id(ElementId::Name(
                        format!("tool-group-header-{group_key}").into(),
                    ))
                    .on_click(move |_, window, cx| {
                        if let Some(on_action) = &header_action {
                            on_action(
                                ToolCallsAction::ToggleCall {
                                    call_id: action_id.clone(),
                                    expanded: !open,
                                },
                                window,
                                cx,
                            );
                        }
                    })
                    .child(icon(activity_icon(&group.name), 13.0, theme.text_tertiary))
                    .child(
                        div()
                            .flex_none()
                            .text_size(px(12.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child(label),
                    )
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_tertiary)
                            .child(format!("· {} calls", group.entries.len())),
                    )
                    .child(
                        div()
                            .ml_auto()
                            .flex_none()
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(status.0, 13.0, status.1))
                            .child(app_icon(
                                if open {
                                    IconName::ChevronUp
                                } else {
                                    IconName::ChevronRight
                                },
                                12.0,
                                theme.text_ghost,
                            )),
                    ),
            );

        if open {
            row = row.child(
                div()
                    .border_t_1()
                    .border_color(theme.border)
                    .pl(px(10.0))
                    .children(group.entries.into_iter().enumerate().map(|(index, entry)| {
                        self.call_row(entry, theme, index == 0, on_action.clone())
                    })),
            );
        }
        row.into_any_element()
    }

    fn thinking_row(&self, id: &str, text: &str, _theme: Theme, is_streaming: bool) -> AnyElement {
        let open = self.state.borrow().thinking_expanded.contains(id);
        let view = self.markdown_view_for(id);
        let on_action = self.on_action.clone();
        let id = id.to_owned();
        div()
            .px(px(10.0))
            .py(px(6.0))
            .child(
                ThinkingBlock::new(id.clone(), text.to_owned(), !open)
                    .markdown_view(view)
                    .selection(self.selection.clone())
                    .streaming(is_streaming)
                    .on_toggle(move |expanded, window, cx| {
                        if let Some(on_action) = &on_action {
                            on_action(
                                ToolCallsAction::ToggleThinking {
                                    id: id.clone(),
                                    expanded,
                                },
                                window,
                                cx,
                            );
                        }
                    }),
            )
            .into_any_element()
    }

    fn text_row(&self, id: &str, text: &str, theme: Theme, is_streaming: bool) -> AnyElement {
        let view = self.markdown_view_for(id);
        let palette = Palette::from_theme(&theme);
        let ctx = MarkdownCtx::new(
            id.to_owned(),
            &palette,
            Metrics::BODY,
            self.selection.clone(),
        );
        div()
            .px(px(10.0))
            .py(px(6.0))
            .child(render_selectable_markdown(
                text,
                Some(&view),
                &ctx,
                is_streaming,
            ))
            .into_any_element()
    }

    fn markdown_view_for(&self, id: &str) -> Rc<RefCell<MarkdownView>> {
        self.state
            .borrow_mut()
            .markdown_views
            .entry(id.to_owned())
            .or_insert_with(|| Rc::new(RefCell::new(MarkdownView::new())))
            .clone()
    }

    /// Build the expanded timeline: thinking/text events render individually,
    /// and consecutive tool calls of the same name collapse into one group.
    /// Mirrors the desktop app's `groupEvents` pass.
    fn timeline_children(&self, theme: Theme) -> Vec<AnyElement> {
        let is_streaming = self.working;
        let mut children: Vec<AnyElement> = Vec::new();
        let mut pending: Vec<ToolCallEntry> = Vec::new();
        for event in &self.events {
            match event {
                ActivityEvent::Thinking { id, text } => {
                    if !pending.is_empty() {
                        children.extend(self.flush_tool_groups(&mut pending, theme));
                    }
                    children.push(self.thinking_row(id, text, theme, is_streaming));
                }
                ActivityEvent::Text { id, text } => {
                    if !pending.is_empty() {
                        children.extend(self.flush_tool_groups(&mut pending, theme));
                    }
                    children.push(self.text_row(id, text, theme, is_streaming));
                }
                ActivityEvent::ToolCall(entry) => pending.push(entry.clone()),
            }
        }
        children.extend(self.flush_tool_groups(&mut pending, theme));
        children
    }

    fn flush_tool_groups(&self, pending: &mut Vec<ToolCallEntry>, theme: Theme) -> Vec<AnyElement> {
        if pending.is_empty() {
            return Vec::new();
        }
        let groups = Self::groups(pending);
        let children = groups
            .into_iter()
            .map(|group| {
                if group.entries.len() == 1 {
                    self.call_row(
                        group.entries.into_iter().next().unwrap(),
                        theme,
                        true,
                        self.on_action.clone(),
                    )
                } else {
                    self.group_row(group, theme, self.on_action.clone())
                }
            })
            .collect::<Vec<_>>();
        pending.clear();
        children
    }
}

impl RenderOnce for ToolCalls {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        if self.events.is_empty() {
            return div().into_any_element();
        }

        let theme = Theme::current(cx);
        let run_id = self.run_id.clone();
        let state = self.state.borrow();
        let expanded = if self.working && !state.collapsed_runs.contains(&run_id) {
            true
        } else {
            state.expanded_runs.contains(&run_id)
        };
        drop(state);
        let action_id = run_id.clone();
        let on_action = self.on_action.clone();
        let elapsed = format_elapsed(self.elapsed_seconds);
        let summary = if self.working {
            None
        } else {
            // Tool call errors are already shown per-row with the alert icon.
            // The run-level summary should reflect the run's overall duration,
            // not declare failure just because one tool errored — the agent may
            // have recovered and completed successfully.
            Some(format!("Worked for {elapsed}"))
        };

        let mut header = div()
            .id(ElementId::Name(format!("tool-run-{run_id}").into()))
            .w_full()
            .flex()
            .items_center()
            .gap(px(7.0))
            .px(px(10.0))
            .py(px(5.0))
            .cursor_pointer()
            .rounded(px(6.0))
            .border_1()
            .border_color(transparent_black())
            .on_click(move |_, window, cx| {
                if let Some(on_action) = &on_action {
                    on_action(
                        ToolCallsAction::ToggleRun {
                            run_id: action_id.clone(),
                            expanded: !expanded,
                        },
                        window,
                        cx,
                    );
                }
            });

        if self.working {
            header = header.child(WorkingIndicator::new(
                self.started_at
                    .unwrap_or_else(|| chrono::Utc::now().timestamp()),
                theme,
            ));
        } else if let Some(summary) = summary {
            header = header
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_tertiary)
                        .child(summary),
                )
                .child(app_icon(
                    if expanded {
                        IconName::ChevronUp
                    } else {
                        IconName::ChevronRight
                    },
                    12.0,
                    theme.text_ghost,
                ))
                .child(div().ml_auto());
        } else {
            header = header.child(div().ml_auto());
        }

        let mut container = div()
            .w_full()
            .border_b_1()
            .border_color(theme.border)
            .children([header.into_any_element()]);

        if expanded {
            container = container.child(
                div()
                    .overflow_hidden()
                    .children(self.timeline_children(theme)),
            );
        }

        container.into_any_element()
    }
}

impl ToolCalls {
    /// A labelled, scrollable, selectable block of tool output (arguments or
    /// results). The `call_id` namespaces the selection row so each call's
    /// Arguments/Result get distinct registry keys instead of colliding with
    /// every other call's Arguments/Result in the same frame.
    fn section(
        &self,
        call_id: &str,
        label: &'static str,
        content: String,
        theme: Theme,
    ) -> impl IntoElement {
        let row = format!("tool-{call_id}-{label}");
        let palette = Palette::from_theme(&theme);
        let ctx = MarkdownCtx::new(row, &palette, Metrics::COMPACT, self.selection.clone());
        div()
            .flex()
            .flex_col()
            .gap(px(3.0))
            .child(
                div()
                    .text_size(px(10.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.text_ghost)
                    .child(label),
            )
            .child(
                div()
                    .id(ElementId::Name(format!("tool-output-{call_id}-{label}").into()))
                    .max_h(px(160.0))
                    .overflow_y_scroll()
                    .rounded(px(5.0))
                    .bg(theme.inset)
                    .px(px(8.0))
                    .py(px(6.0))
                    .font_family("GeistMono")
                    .text_size(px(10.5))
                    .line_height(px(15.0))
                    .text_color(theme.text_tertiary)
                    .child(plain_text(
                        content,
                        "GeistMono",
                        FontWeight::NORMAL,
                        theme.text_tertiary,
                        &ctx,
                    )),
            )
    }
}

/// Whether a tool-call name represents a file-edit operation whose
/// arguments carry `oldContent` / `newContent` for diffing.
fn is_edit_file(name: &str) -> bool {
    matches!(name, "editFile" | "edit_file" | "str_replace")
}

fn truncate(value: &str, max_chars: usize) -> String {
    let mut chars = value.chars();
    let truncated: String = chars.by_ref().take(max_chars).collect();
    if chars.next().is_some() {
        format!("{truncated}…")
    } else {
        truncated
    }
}

pub fn format_elapsed(seconds: u64) -> String {
    match seconds {
        0..=59 => format!("{seconds}s"),
        60..=3_599 => format!("{}m {:02}s", seconds / 60, seconds % 60),
        _ => format!("{}h {:02}m", seconds / 3_600, (seconds % 3_600) / 60),
    }
}

pub fn elapsed_seconds(started_at: Option<i64>, ended_at: Option<i64>) -> u64 {
    let Some(started_at) = started_at.and_then(normalize_unix_timestamp) else {
        return 0;
    };
    let Some(ended_at) = ended_at.and_then(normalize_unix_timestamp) else {
        return 0;
    };
    ended_at.saturating_sub(started_at) as u64
}

#[cfg(test)]
mod tests {
    use super::{
        ToolCall, ToolCallEntry, ToolCalls, attach_results_by_id, elapsed_seconds, format_elapsed,
    };
    use serde_json::json;

    fn call(id: &str, name: &str) -> ToolCall {
        ToolCall {
            id: id.into(),
            name: name.into(),
            arguments: json!({"path": "src/main.rs"}),
            thought_signature: None,
        }
    }

    #[test]
    fn groups_only_consecutive_calls_with_the_same_name() {
        let entries = ["readFile", "readFile", "bash", "readFile"]
            .into_iter()
            .enumerate()
            .map(|(index, name)| ToolCallEntry {
                call: call(&index.to_string(), name),
                result: None,
            })
            .collect::<Vec<_>>();
        let groups = ToolCalls::groups(&entries);
        assert_eq!(
            groups
                .iter()
                .map(|group| group.entries.len())
                .collect::<Vec<_>>(),
            [2, 1, 1]
        );
        assert_eq!(groups[0].name, "readFile");
        assert_eq!(groups[2].name, "readFile");
    }

    #[test]
    fn results_match_calls_by_id_not_position() {
        let entries = vec![
            ToolCallEntry {
                call: call("first", "readFile"),
                result: None,
            },
            ToolCallEntry {
                call: call("second", "bash"),
                result: None,
            },
        ];
        let results = vec![console_core::ToolResult {
            tool_call_id: "second".into(),
            tool_name: Some("bash".into()),
            content: serde_json::json!("done"),
            is_error: Some(false),
        }];
        let joined = attach_results_by_id(entries, results);
        assert!(joined[0].result.is_none());
        assert_eq!(joined[1].result.as_ref().unwrap().tool_call_id, "second");
    }

    #[test]
    fn formats_elapsed_time_like_run_activity() {
        assert_eq!(format_elapsed(0), "0s");
        assert_eq!(format_elapsed(65), "1m 05s");
        assert_eq!(format_elapsed(3_725), "1h 02m");
    }

    #[test]
    fn matches_elapsed_timestamp_units() {
        assert_eq!(
            elapsed_seconds(Some(1_700_000_000), Some(1_700_000_065)),
            65
        );
        assert_eq!(
            elapsed_seconds(Some(1_700_000_000_000), Some(1_700_000_065_000)),
            65
        );
    }
}
