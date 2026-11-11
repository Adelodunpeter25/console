//! Waku-style transcript surface backed by `console-core::AgentMessage`.

use console_core::types::SessionFileChange;
use console_core::{
    AgentMessage, AssistantContentPart, AssistantMessage, ImageAttachment, ToolResult,
};
use gpui::{
    App, Context, FocusHandle, FollowMode, IntoElement, KeyDownEvent, ListAlignment, ListOffset,
    ListState, Render, ScrollStrategy, SharedString, Task, UniformListScrollHandle, Window, div, list,
    prelude::*, px,
};
use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::rc::Rc;
use std::sync::Arc;
use std::time::Duration;

use crate::chat::change_chips::{ChangesSummary, TurnChanges};
use crate::chat::{ActivityEvent, ToolCallEntry, ToolCallsAction, ToolCallsState};
use crate::markdown::file_links::{is_file_link, is_http_url};
use crate::markdown::render::{self, LinkHandler, MarkdownView, TranscriptSelection};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::theme::Theme;

mod callbacks;
mod changes;
mod disclosure;
mod messages;
mod prompts;
mod rows;
mod scroll;
mod selection;
mod streaming;
mod tool_activity;

type MarkdownViewCache = Rc<RefCell<HashMap<(usize, usize), Rc<RefCell<MarkdownView>>>>>;

/// Most chat rows are short, while long Markdown rows are corrected after
/// their first measurement. A realistic estimate reduces initial scroll
/// correction versus the old one-thousand-pixel default.
const ESTIMATED_TRANSCRIPT_ROW_HEIGHT: f32 = 120.0;

#[derive(Clone)]
pub(crate) struct AssistantPresentation {
    pub(crate) content_parts: Rc<Vec<AssistantContentPart>>,
    pub(crate) copy_content: SharedString,
}

/// The id of the message in `row`, when it has one.
///
/// Tool results carry no id of their own — they belong to the tool call
/// above them.
pub fn message_id_at(messages: &[AgentMessage], row: usize) -> Option<String> {
    messages.get(row).and_then(|message| match message {
        AgentMessage::User { id, .. } | AgentMessage::Assistant { id, .. } => id.clone(),
        AgentMessage::ToolResult { .. } => None,
    })
}

/// The row holding the message with `id`, if it is in `messages`.
///
/// This is what makes a scroll anchor survive a reload: a row index counts
/// rows in the window that happens to be loaded, and the window moves as
/// older pages are prepended. The id names the message itself.
pub fn row_for_message_id(messages: &[AgentMessage], id: &str) -> Option<usize> {
    messages.iter().position(|message| match message {
        AgentMessage::User { id: message_id, .. }
        | AgentMessage::Assistant { id: message_id, .. } => message_id.as_deref() == Some(id),
        AgentMessage::ToolResult { .. } => false,
    })
}

/// Resolve an anchor to a row of `messages`.
///
/// A message id that is not loaded cannot be resolved: the fetch window
/// moved on. The row index is only a fallback for messages that have no id
/// at all — an optimistic user message, before the server has assigned one.
pub fn resolve_anchor_row(
    messages: &[AgentMessage],
    message_id: Option<&str>,
    row_index: usize,
) -> Option<usize> {
    match message_id {
        Some(id) => row_for_message_id(messages, id),
        None => Some(row_index.min(messages.len().saturating_sub(1))),
    }
}

/// The row an anchor points at, given the row at the top of the viewport.
///
/// Clamped to the last message: while a run streams the list carries a
/// trailing "Working…" row that is not a message, and an index pointing at
/// it would point past the end once that row is gone.
pub fn anchor_row(message_count: usize, top_row: usize) -> usize {
    top_row.min(message_count.saturating_sub(1))
}

/// Where a transcript was left, in logical list terms.
///
/// `row_index` counts rows of the window that happened to be loaded, so it
/// shifts when older pages are prepended and points past the end when the
/// window is shorter than it was. `message_id` names the message itself and
/// survives both; it is the anchor that is persisted.
#[derive(Clone, Debug, PartialEq)]
pub struct TranscriptScrollAnchor {
    pub row_index: usize,
    pub offset_in_row: f32,
    pub at_tail: bool,
    pub message_id: Option<String>,
}

/// Target destination for animated transcript scrolling.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ScrollTarget {
    Prompt(usize),
    Bottom,
}

pub struct TranscriptView {
    pub messages: Vec<AgentMessage>,
    /// Persistent Markdown state keyed by message index and content-part
    /// index. The parser and flattened text cache inside each view survive
    /// transcript row rebuilds, which is especially important while a stream
    /// appends to the final text part.
    markdown_cache: MarkdownViewCache,
    /// Immutable presentation snapshots for settled assistant rows. Scrolling
    /// can remount a row without cloning its full content vector or rebuilding
    /// its copy string; a revision changes only when that message mutates.
    presentation_cache: RefCell<HashMap<usize, (u64, AssistantPresentation)>>,
    content_revisions: Vec<u64>,
    pub(crate) list_state: ListState,
    scrollbar_state: std::rc::Rc<ScrollbarState>,
    pub(crate) prompts_open: bool,
    pub(crate) prompts_focus: FocusHandle,
    pub(crate) prompts: Rc<Vec<console_core::SessionPrompt>>,
    /// The pointer is over the transcript: the floating controls show only then.
    pub(crate) hovered: bool,
    pub(crate) prompts_scroll: UniformListScrollHandle,
    /// The fetched prompt list no longer matches the chat (new prompt, or a
    /// different chat); reopening the popup refetches only then.
    pub(crate) prompts_stale: bool,
    pub(crate) on_request_prompts: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    on_jump_to_prompt: Option<Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>>,
    focus_handle: FocusHandle,
    pub(crate) selection: TranscriptSelection,
    pub(crate) is_streaming: bool,
    /// Unix seconds when the current run started streaming; the live
    /// "Working for Ns" indicator ticks from this.
    pub(crate) streaming_started_at: Option<i64>,
    /// Invalidates deferred scroll restores from older chat switches.
    scroll_restore_generation: u64,
    /// In-flight smooth scroll animation task.
    scroll_animation_task: Option<Task<()>>,
    /// Disclosure state for grouped tool-call activity. This belongs to the
    /// transcript entity so virtualization does not reset it when rows remount.
    pub(crate) tool_calls_state: Rc<RefCell<ToolCallsState>>,
    /// IDs of thinking blocks the user has expanded. Thinking is collapsed by
    /// default, but the choice survives virtualized row remounts.
    pub(crate) thinking_expanded: Rc<RefCell<HashSet<String>>>,
    /// The active session's working directory, for relative tool paths.
    pub(crate) session_cwd: Option<String>,
    /// Cache for run activity derived from messages. Keyed by user_index,
    /// invalidated when messages or streaming state changes.
    tool_activity_cache:
        RefCell<HashMap<usize, (u64, Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)>)>>,
    tool_activity_cache_version: std::cell::Cell<u64>,
    /// Pagination for incremental loading: whether older messages exist and cursor.
    has_more: bool,
    next_cursor: Option<i64>,
    /// Whether a pagination fetch is in flight.
    loading_older: bool,
    on_load_older: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    /// Opens the image preview modal with the decoded image when the user
    /// clicks an image inside a message bubble.
    pub(crate) on_preview_image:
        Option<Rc<dyn Fn(Arc<gpui::Image>, &mut Window, &mut App) + 'static>>,
    on_view_subagent: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    on_open_file_raw: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    on_open_url_raw: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    pub(crate) link_handler: Option<LinkHandler>,
    /// The session's recorded file changes grouped by the user message that
    /// started each turn, built once per fetch. A turn's chip row under its
    /// final assistant message is a lookup here, never a scan or a copy.
    turn_changes: HashMap<String, Rc<TurnChanges>>,
    /// The raw session change list the grouped map was built from, kept for
    /// the whole-session totals in the pinned summary pill.
    session_changes: Rc<Vec<SessionFileChange>>,
    /// Opens the diff of one changed file of one turn:
    /// `(path, turn_index, diff_text)`.
    pub(crate) on_open_change:
        Option<Rc<dyn Fn(String, u32, Option<String>, &mut Window, &mut App) + 'static>>,
    /// Fetches a changed file's diff when its chip row came without diffs.
    pub(crate) load_diff: Option<crate::chat::change_chips::LoadDiffHandler>,
    /// Forks the session through one assistant message: `(message_id)`.
    pub(crate) on_fork: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    /// Window-space bounds of this transcript's container, recorded by a
    /// paint-time canvas probe each frame. The Cite popup's window-space
    /// anchor converts to container-relative coordinates through it; a
    /// one-frame-stale origin after a resize is invisible.
    cite_origin: Rc<RefCell<Option<gpui::Bounds<gpui::Pixels>>>>,
    /// Turns (by their user message id) whose changed-file row is expanded
    /// past the collapsed chip count. Lives here so virtualized rows that
    /// remount keep their state.
    pub(crate) expanded_turn_changes: Rc<RefCell<HashSet<String>>>,
    /// User messages (by server id, falling back to the row key) whose
    /// folded text is expanded via Show more. Same lifetime as the other
    /// per-row disclosure state above.
    expanded_user_messages: Rc<RefCell<HashSet<String>>>,
}

impl TranscriptView {
    pub fn new(cx: &mut Context<Self>) -> Self {
        // Top-anchored like the web app: a short conversation starts at the
        // very top. Follow-mode keeps long and streaming conversations pinned
        // to the newest message (and pauses when the user scrolls up).
        let list_state = ListState::new(0, ListAlignment::Top, px(ESTIMATED_TRANSCRIPT_ROW_HEIGHT));
        list_state.set_follow_mode(FollowMode::Tail);
        Self {
            messages: Vec::new(),
            markdown_cache: Rc::new(RefCell::new(HashMap::new())),
            presentation_cache: RefCell::new(HashMap::new()),
            content_revisions: Vec::new(),
            list_state,
            scrollbar_state: ScrollbarState::new(),
            prompts_open: false,
            prompts_focus: cx.focus_handle(),
            prompts: Rc::new(Vec::new()),
            hovered: false,
            prompts_scroll: UniformListScrollHandle::new(),
            prompts_stale: true,
            on_request_prompts: None,
            on_jump_to_prompt: None,
            focus_handle: cx.focus_handle(),
            selection: TranscriptSelection::default(),
            is_streaming: false,
            streaming_started_at: None,
            scroll_restore_generation: 0,
            scroll_animation_task: None,
            tool_calls_state: Rc::new(RefCell::new(ToolCallsState::default())),
            thinking_expanded: Rc::new(RefCell::new(HashSet::new())),
            session_cwd: None,
            tool_activity_cache: RefCell::new(HashMap::new()),
            tool_activity_cache_version: std::cell::Cell::new(0),
            has_more: false,
            next_cursor: None,
            loading_older: false,
            on_load_older: None,
            on_preview_image: None,
            on_view_subagent: None,
            on_open_file_raw: None,
            on_open_url_raw: None,
            link_handler: None,
            turn_changes: HashMap::new(),
            session_changes: Rc::new(Vec::new()),
            on_open_change: None,
            load_diff: None,
            on_fork: None,
            cite_origin: Rc::new(RefCell::new(None)),
            expanded_turn_changes: Rc::new(RefCell::new(HashSet::new())),
            expanded_user_messages: Rc::new(RefCell::new(HashSet::new())),
        }
    }

    /// Copy server-assigned ids onto this transcript's user messages that
    /// still lack one (the optimistic prompt of a live run). Pairs user
    /// messages from the tail, since the canonical page always ends at the
    /// same latest message. Per-turn file changes are keyed by that id.
    pub fn adopt_user_message_ids(&mut self, canonical: &[AgentMessage], cx: &mut Context<Self>) {
        if crate::chat::turn_changes::adopt_user_ids(&mut self.messages, canonical) {
            cx.notify();
        }
    }

    pub fn session_cwd(&self) -> Option<String> {
        self.session_cwd.clone()
    }

    /// Record the active session's working directory so tool-call rows can
    /// show paths relative to it. Called when a session loads into the pane.
    pub fn set_session_cwd(&mut self, cwd: Option<String>) -> &mut Self {
        self.session_cwd = cwd.filter(|cwd| !cwd.is_empty());
        // Summaries are derived per render, not cached.
        self.tool_activity_cache.borrow_mut().clear();
        self
    }
}

impl Render for TranscriptView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selection = self.selection.clone();
        let entity = cx.entity().downgrade();
        let is_streaming = self.is_streaming;

        // Auto-load older when scrolled near top (covers wheel, scrollbar, touch).
        // Hidden tool-transport rows have zero height, so the first visible
        // row can be at item_ix > 1 even when visually at the top. We find the
        // index of the first non-hidden message so that pages beginning with
        // long runs of tool transport rows (which take zero height) correctly
        // trigger older message pagination when the user scrolls near the top.
        if self.has_more && !self.loading_older && !self.messages.is_empty() {
            let top = self.list_state.logical_scroll_top();
            let first_visible_ix = self
                .messages
                .iter()
                .position(|m| !crate::chat::row::is_hidden_tool_transport(m))
                .unwrap_or(0);
            if top.item_ix <= first_visible_ix.saturating_add(5) {
                self.loading_older = true;
                if let Some(handler) = self.on_load_older.clone() {
                    let window_handle = _window.window_handle();
                    cx.spawn(async move |_, cx| {
                        // Defer to next frame to avoid re-entrancy during render
                        cx.background_executor()
                            .timer(std::time::Duration::from_millis(16))
                            .await;
                        let _ = cx.update_window(window_handle, |_, window, cx| {
                            handler(window, cx);
                        });
                    })
                    .detach();
                }
            }
        }

        // The Copy/Cite popup shows while a settled selection exists and its
        // end is still on screen. The head rect is recomputed from the live
        // selection registry each frame, so scrolling re-anchors the pills
        // with the text and hides them once the selection's end scrolls out.
        // Selections ending in a user message never get the popup — citing
        // is for the agent's words.
        let cite_line = {
            let state = self.selection.selection.borrow();
            let head = (!state.is_dragging())
                .then(|| state.head_span().cloned())
                .flatten()
                .filter(|span| !span.key.row.starts_with("user-"));
            drop(state);
            head.and_then(|span| {
                let registry = self.selection.registry.borrow();
                let entry = registry
                    .entries()
                    .iter()
                    .find(|entry| entry.key == span.key)?;
                let rects = render::range_rects(&entry.geometry, &span.range, 0.0, 0.0);
                let rect = rects.last()?;
                Some([
                    f32::from(rect.origin.x),
                    f32::from(rect.origin.y),
                    f32::from(rect.size.width),
                    f32::from(rect.size.height),
                ])
            })
        };
        let cite_available = self.selection.cite_handler().is_some();
        let cite_origin = self.cite_origin.clone();

        let root = div()
            .id("transcript-view")
            .on_hover(cx.listener(|this, hovered: &bool, _, cx| {
                if this.hovered != *hovered {
                    this.hovered = *hovered;
                    cx.notify();
                }
            }))
            .key_context("TranscriptView")
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, window, cx| {
                if this.prompts_open && event.keystroke.key == "escape" {
                    this.close_prompts(cx);
                    cx.stop_propagation();
                    return;
                }
                if this.has_active_cite_popup() {
                    let key = event.keystroke.key.as_str();
                    let no_mods = !event.keystroke.modifiers.control
                        && !event.keystroke.modifiers.alt
                        && !event.keystroke.modifiers.platform;
                    if (key == "r" || key == "R") && no_mods && this.trigger_cite(window, cx) {
                        cx.stop_propagation();
                        return;
                    }
                    if (key == "c" || key == "C")
                        && (no_mods || event.keystroke.modifiers.platform)
                        && this.trigger_copy(window, cx)
                    {
                        cx.stop_propagation();
                        return;
                    }
                    if key == "escape" {
                        this.clear_selection(window);
                        cx.stop_propagation();
                    }
                }
            }))
            .relative()
            .size_full()
            .bg(theme.chat_canvas)
            .child(render::frame_reset(selection.clone()))
            .child(crate::chat::cite_overlay::cite_origin_probe(
                cite_origin.clone(),
            ))
            .child(if self.messages.is_empty() && !is_streaming {
                crate::chat::row::empty_state(theme).into_any_element()
            } else {
                div()
                    .size_full()
                    .flex()
                    .flex_col()
                    .child(
                        list(self.list_state.clone(), move |index, _window, cx| {
                            crate::chat::row::transcript_row(entity.clone(), index, cx)
                        })
                        // Room at the tail for the floating composer pills:
                        // list padding is part of the scroll content, so the
                        // final row can scroll clear of the overlay instead of
                        // sitting underneath it.
                        .pb(px(44.0))
                        .size_full(),
                    )
                    .child(scrollbar::vertical(&self.list_state, &self.scrollbar_state))
                    .into_any_element()
            })
            .child(crate::chat::cite_overlay::selection_input(
                selection.clone(),
                cite_origin.clone(),
            ))
            .child(self.render_scroll_controls(theme, cx));
        if let (Some(line), true) = (cite_line, cite_available)
            && let Some(container) = *cite_origin.borrow()
        {
            return root
                .child(crate::chat::cite_overlay::cite_popup(
                    line,
                    container,
                    selection.clone(),
                    theme,
                ))
                .into_any_element();
        }
        root.into_any_element()
    }
}
