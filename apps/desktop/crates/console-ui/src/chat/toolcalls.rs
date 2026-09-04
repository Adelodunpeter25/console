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

use crate::chat::markdown_helpers::{assistant_ctx, render_selectable_markdown};
use crate::chat::{DiffView, ThinkingBlock, WorkingIndicator};
use crate::markdown::render::{
    LinkHandler, MarkdownView, Palette, TranscriptSelection, plain_text,
};
use crate::markdown::{highlight, render as markdown_render};
use crate::primitives::{IconName, activity_icon, app_icon, file_type_icon, icon};
use crate::theme::Theme;
use crate::utils::time::{format_working_elapsed, normalize_unix_timestamp};
// `format_elapsed` was unified into `format_working_elapsed` (C); kept
// re-exported so older imports keep resolving.
pub use crate::utils::time::format_working_elapsed as format_elapsed;

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
    ViewSubagentInPanel { call_id: String },
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
    /// Session working directory, so absolute tool paths render relative.
    cwd: Option<String>,
    state: Rc<RefCell<ToolCallsState>>,
    /// Shared transcript selection so run-activity text, arguments, and
    /// results join the same selection state as the surrounding messages.
    selection: TranscriptSelection,
    on_action: Option<Rc<dyn Fn(ToolCallsAction, &mut Window, &mut App) + 'static>>,
    link_handler: Option<LinkHandler>,
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
            cwd: None,
            state,
            selection: TranscriptSelection::default(),
            on_action: None,
            link_handler: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection) -> Self {
        self.selection = selection;
        self
    }

    /// The session working directory, for relative path display.
    pub fn cwd(mut self, cwd: Option<String>) -> Self {
        self.cwd = cwd.filter(|cwd| !cwd.is_empty());
        self
    }

    pub fn on_action(
        mut self,
        handler: impl Fn(ToolCallsAction, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_action = Some(Rc::new(handler));
        self
    }

    pub fn link_handler(mut self, handler: Option<LinkHandler>) -> Self {
        self.link_handler = handler;
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

    fn argument_summary(&self, call: &ToolCall) -> Option<String> {
        let object = call.arguments.as_object()?;
        let cwd = self.cwd.as_deref();
        if let Some(path) = argument_path(call) {
            return Some(truncate(&to_relative_path(path, cwd), 72));
        }
        for key in ["command", "pattern", "query", "url", "directory"] {
            if let Some(value) = object.get(key).and_then(|value| value.as_str()) {
                let value = if key == "directory" {
                    to_relative_path(value, cwd)
                } else {
                    value.to_owned()
                };
                return Some(truncate(&value, 72));
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
        let summary = self.argument_summary(&entry.call);
        let (status_icon, status_color) = Self::status_icon(&entry, &theme);
        let call_id_for_action = call_id.clone();
        let is_edit = is_edit_file(&entry.call.name);
        let file_path = argument_path(&entry.call).map(str::to_owned);
        let diff = if is_edit {
            let mut cache = self.state.borrow_mut();
            if let Some((cached_args, cached_diff)) = cache.diff_cache.get(&call_id) {
                if cached_args == &entry.call.arguments {
                    cached_diff.clone()
                } else {
                    let computed = extract_edit_args(&entry.call.arguments)
                        .map(|(old, new)| diff_lines(old, new, 3));
                    cache.diff_cache.insert(
                        call_id.clone(),
                        (entry.call.arguments.clone(), computed.clone()),
                    );
                    computed
                }
            } else {
                let computed = extract_edit_args(&entry.call.arguments)
                    .map(|(old, new)| diff_lines(old, new, 3));
                cache.diff_cache.insert(
                    call_id.clone(),
                    (entry.call.arguments.clone(), computed.clone()),
                );
                computed
            }
        } else {
            None
        };
        let has_diff = diff.is_some();
        let header_action = on_action.clone();
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
                    .bg(theme.inset)
                    .border_1()
                    .border_color(transparent_black())
                    .id(ElementId::Name(
                        format!("tool-call-header-{call_id}").into(),
                    ))
                    .on_click(move |_, window, cx| {
                        if let Some(on_action) = &header_action {
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
                    .when_some(file_path.clone(), |element, path| {
                        // File-targeting calls show the file's real type icon
                        // (multicolor, like mobile) instead of the generic
                        // monochrome tool glyph.
                        element.child(file_type_icon(&path, 13.0))
                    })
                    .when_none(&file_path, |element| {
                        element.child(icon(
                            activity_icon(&entry.call.name),
                            13.0,
                            theme.text_tertiary,
                        ))
                    })
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
                                .font_family(markdown_render::MONO_FAMILY)
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
                                .child(
                                    div()
                                        .text_color(theme.success)
                                        .child(format!("+{}", d.added)),
                                )
                                .child(
                                    div()
                                        .text_color(theme.danger)
                                        .child(format!("-{}", d.removed)),
                                ),
                        )
                    })
                    .when(entry.call.name == "subagent", |element| {
                        let on_action = on_action.clone();
                        let call_id = call_id.clone();
                        element.child(
                            div()
                                .id(format!("view-subagent-btn-{}", call_id))
                                .flex()
                                .items_center()
                                .gap(px(4.0))
                                .px(px(6.0))
                                .py(px(2.0))
                                .rounded(px(4.0))
                                .bg(theme.overlay)
                                .cursor_pointer()
                                .hover(|s| s.bg(theme.overlay_strong))
                                .on_click(move |_, window, cx| {
                                    cx.stop_propagation();
                                    if let Some(on_action) = &on_action {
                                        on_action(
                                            ToolCallsAction::ViewSubagentInPanel {
                                                call_id: call_id.clone(),
                                            },
                                            window,
                                            cx,
                                        );
                                    }
                                })
                                .child(
                                    div()
                                        .text_size(px(10.5))
                                        .font_weight(FontWeight::MEDIUM)
                                        .text_color(theme.accent)
                                        .child("View in Panel →"),
                                ),
                        )
                    })
                    .child(
                        div()
                            .ml_auto()
                            .flex_none()
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(status_icon, 13.0, status_color))
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
                        let mut view = DiffView::new(call_id.clone(), d);
                        if let Some(path) = &file_path {
                            view = view.file_path(path.clone());
                        }
                        element.child(view)
                    })
                    .when(!has_diff, |element| {
                        let arguments = serde_json::to_string_pretty(&entry.call.arguments)
                            .unwrap_or_else(|_| entry.call.arguments.to_string());
                        element.child(self.section(&call_id, "Arguments", arguments, theme))
                    })
                    .when_some(entry.result, |element, result| {
                        let raw = Self::formatted_result(&result);
                        if is_read_file(&entry.call.name) {
                            element.child(self.read_file_section(
                                &call_id,
                                raw,
                                file_path.as_deref(),
                                theme,
                            ))
                        } else {
                            element.child(self.section(&call_id, "Result", raw, theme))
                        }
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
                    .bg(theme.inset)
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
        let link_handler = self.link_handler.clone();
        let id = id.to_owned();
        div()
            .px(px(10.0))
            .py(px(6.0))
            .child(
                ThinkingBlock::new(id.clone(), text.to_owned(), !open)
                    .markdown_view(view)
                    .selection(self.selection.clone())
                    .streaming(is_streaming)
                    .link_handler(link_handler)
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
        let ctx = assistant_ctx(
            id.to_owned(),
            &palette,
            self.selection.clone(),
            self.link_handler.clone(),
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
        let elapsed = format_working_elapsed(self.elapsed_seconds);
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
    /// The Result block for readFile calls: the file's code, syntax
    /// highlighted through the markdown lexer when its extension maps to a
    /// language. Mirrors mobile's `ReadFileResult` normalization — a leading
    /// `File:` header is dropped, and line numbers are stripped when most
    /// lines carry them.
    fn read_file_section(
        &self,
        call_id: &str,
        raw: String,
        path: Option<&str>,
        theme: Theme,
    ) -> AnyElement {
        let lang_tag = path.and_then(highlight::lang_tag_for_path);
        let Some(lang_tag) = lang_tag else {
            return self
                .section(call_id, "Result", raw, theme)
                .into_any_element();
        };

        let palette = Palette::from_theme(&theme);
        let ctx = crate::chat::markdown_helpers::compact_ctx(
            format!("tool-{call_id}-result"),
            &palette,
            self.selection.clone(),
            None,
        );
        div()
            .flex()
            .flex_col()
            .gap(px(3.0))
            .child(
                div()
                    .text_size(px(10.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.text_ghost)
                    .child("Result"),
            )
            .child(
                div()
                    .id(ElementId::Name(
                        format!("tool-output-{call_id}-result").into(),
                    ))
                    .max_h(px(240.0))
                    .overflow_y_scroll()
                    .rounded(px(5.0))
                    .bg(theme.inset)
                    .px(px(8.0))
                    .py(px(6.0))
                    .font_family(markdown_render::MONO_FAMILY)
                    .text_size(px(12.0))
                    .line_height(px(17.0))
                    .text_color(theme.text_tertiary)
                    .child(markdown_render::highlighted_code(
                        normalize_read_file_output(&raw),
                        lang_tag,
                        &ctx,
                    )),
            )
            .into_any_element()
    }

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
        let ctx = crate::chat::markdown_helpers::compact_ctx(
            row,
            &palette,
            self.selection.clone(),
            None,
        );
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
                    .id(ElementId::Name(
                        format!("tool-output-{call_id}-{label}").into(),
                    ))
                    .max_h(px(160.0))
                    .overflow_y_scroll()
                    .rounded(px(5.0))
                    .bg(theme.inset)
                    .px(px(8.0))
                    .py(px(6.0))
                    .font_family(markdown_render::MONO_FAMILY)
                    .text_size(px(12.0))
                    .line_height(px(17.0))
                    .text_color(theme.text_tertiary)
                    .child(plain_text(
                        content,
                        markdown_render::MONO_FAMILY,
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

/// Whether a tool-call name reads file content into its result.
fn is_read_file(name: &str) -> bool {
    matches!(name, "readFile" | "read_file" | "view" | "Read")
}

/// Byte length of the `\s*\d+:\s?` line-number prefix, if present.
fn line_number_prefix_len(line: &str) -> Option<usize> {
    let bytes = line.as_bytes();
    let mut index = line.len() - line.trim_start().len();
    let digits_start = index;
    while index < line.len() && bytes[index].is_ascii_digit() {
        index += 1;
    }
    if index == digits_start || bytes.get(index) != Some(&b':') {
        return None;
    }
    index += 1;
    if bytes.get(index) == Some(&b' ') {
        index += 1;
    }
    Some(index)
}

/// Normalize readFile output to its bare code body, mirroring mobile's
/// `ReadFileResult`: drop the `File:` header block (when short and followed by
/// a blank line) and strip per-line numbers when most lines carry them.
fn normalize_read_file_output(raw: &str) -> String {
    let lines: Vec<&str> = raw.lines().collect();
    let header_end = lines
        .iter()
        .skip(1)
        .position(|line| line.is_empty())
        .filter(|&end| end > 0 && end <= 6)
        .filter(|_| {
            lines
                .first()
                .is_some_and(|first| first.starts_with("File:"))
        })
        .map(|end| end + 1);
    let code: &[&str] = match header_end {
        Some(end) => &lines[end..],
        None => &lines[..],
    };
    let numbered = code
        .iter()
        .filter(|line| line_number_prefix_len(line).is_some())
        .count();
    if numbered * 2 > code.len() {
        code.iter()
            .map(|line| line_number_prefix_len(line).map_or(*line, |prefix| &line[prefix..]))
            .collect::<Vec<_>>()
            .join("\n")
    } else {
        code.join("\n")
    }
}

/// The target path of a file-oriented tool call, when its arguments carry one.
fn argument_path(call: &ToolCall) -> Option<&str> {
    let object = call.arguments.as_object()?;
    ["path", "filePath", "targetFile", "absolutePath"]
        .into_iter()
        .find_map(|key| object.get(key).and_then(|value| value.as_str()))
}

/// Format `path` relative to the session working directory, so
/// `/Users/me/repo/apps/server/index.ts` renders as `apps/server/index.ts`.
/// Paths outside the directory are left untouched.
fn to_relative_path(path: &str, cwd: Option<&str>) -> String {
    let normalized_path = path.replace('\\', "/");
    let Some(cwd) = cwd.map(|cwd| cwd.replace('\\', "/")) else {
        return normalized_path;
    };
    let cwd = cwd.trim_end_matches('/');
    if !cwd.is_empty()
        && let Some(rest) = normalized_path.strip_prefix(cwd)
    {
        return rest.trim_start_matches('/').to_owned();
    }
    normalized_path
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

pub fn elapsed_seconds(started_at: Option<i64>, ended_at: Option<i64>) -> u64 {
    let Some(started_at) = started_at.and_then(normalize_unix_timestamp) else {
        return 0;
    };
    let Some(ended_at) = ended_at.and_then(normalize_unix_timestamp) else {
        return 0;
    };
    ended_at.saturating_sub(started_at) as u64
}
