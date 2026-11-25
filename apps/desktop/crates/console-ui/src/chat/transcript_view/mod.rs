//! Waku-style transcript surface backed by server-assembled `console-core::Turn`s.

use console_core::types::SessionFileChange;
use console_core::{
    ActivityEvent, AgentMessage, AssistantContentPart, AssistantMessage, ImageAttachment,
    ToolCallEntry, ToolResult, Turn, TurnRun,
};
use gpui::{
    App, Context, FocusHandle, FollowMode, IntoElement, KeyDownEvent, ListAlignment, ListOffset,
    ListState, Render, ScrollStrategy, SharedString, Task, UniformListScrollHandle, Window, div,
    list, prelude::*, px,
};
use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::rc::Rc;
use std::sync::Arc;
use std::time::{Duration, Instant};

use crate::chat::change_chips::{ChangesSummary, TurnChanges};
use crate::chat::{ToolCallsAction, ToolCallsState};
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
pub use tool_activity::{RunActivityBlock, run_block_for_turn};
mod wake;

type LoadToolResultHandler =
    Rc<dyn Fn(String, String, &mut App) -> Task<Option<ToolResult>> + 'static>;

type MarkdownViewCache = Rc<RefCell<HashMap<(usize, usize), Rc<RefCell<MarkdownView>>>>>;

/// Most chat rows are short, while long Markdown rows are corrected after
/// their first measurement. A realistic estimate reduces initial scroll
/// correction versus the old one-thousand-pixel default.
const ESTIMATED_TRANSCRIPT_ROW_HEIGHT: f32 = 120.0;

#[derive(Clone)]
pub(crate) struct AssistantPresentation {
    pub(crate) content_parts: Rc<[AssistantContentPart]>,
    pub(crate) copy_content: SharedString,
}

/// The id of the turn in `row`, when the server has assigned one.
pub fn message_id_at(turns: &[Turn], row: usize) -> Option<String> {
    turns
        .get(row)
        .map(|turn| turn.id.clone())
        .filter(|id| !id.is_empty())
}

/// The row holding the turn with `id`, if it is loaded.
///
/// This is what makes a scroll anchor survive a reload: a row index counts
/// rows in the window that happens to be loaded, and the window moves as
/// older pages are prepended. The id names the turn itself.
pub fn row_for_message_id(turns: &[Turn], id: &str) -> Option<usize> {
    turns.iter().position(|turn| turn.id == id)
}

/// Resolve an anchor to a row of `turns`.
///
/// A turn id that is not loaded cannot be resolved: the fetch window moved
/// on. The row index is only a fallback for turns that have no id at all, an
/// optimistic prompt before the server has assigned one.
pub fn resolve_anchor_row(
    turns: &[Turn],
    message_id: Option<&str>,
    row_index: usize,
) -> Option<usize> {
    match message_id {
        Some(id) => row_for_message_id(turns, id),
        None => Some(row_index.min(turns.len().saturating_sub(1))),
    }
}

/// The row an anchor points at, given the row at the top of the viewport.
///
/// Clamped to the last turn: while a run streams the list carries a trailing
/// "Working…" row that is not a turn, and an index pointing at it would point
/// past the end once that row is gone.
pub fn anchor_row(turn_count: usize, top_row: usize) -> usize {
    top_row.min(turn_count.saturating_sub(1))
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
    pub turns: Vec<Turn>,
    /// The assistant message still streaming into the last turn. It moves into
    /// the turn's run (when it holds a tool call) or its responses once the
    /// stream finalizes it.
    pub(crate) live: Vec<AssistantContentPart>,
    /// Results that arrived before their tool call settled into a run.
    pub(crate) pending_results: HashMap<String, ToolResult>,
    /// Gives optimistic turns a run id the server has not assigned yet.
    pub(crate) live_counter: u64,
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
    pub(crate) prompts: Rc<[console_core::SessionPrompt]>,
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
    /// The prompt the user just sent (row index, send time). Only that row
    /// plays the send animation, and only briefly, so history loads and
    /// scrolling back never replay it.
    pub(crate) just_sent: Option<(usize, Instant)>,
    /// Disclosure state for grouped tool-call activity. This belongs to the
    /// transcript entity so virtualization does not reset it when rows remount.
    pub(crate) tool_calls_state: Rc<RefCell<ToolCallsState>>,
    /// IDs of thinking blocks the user has expanded. Thinking is collapsed by
    /// default, but the choice survives virtualized row remounts.
    pub(crate) thinking_expanded: Rc<RefCell<HashSet<String>>>,
    /// The active session's working directory, for relative tool paths.
    pub(crate) session_cwd: Option<String>,
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
    load_tool_result: Option<LoadToolResultHandler>,
    fetching_results: HashSet<String>,
    on_open_file_raw: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    pub(crate) on_open_url_raw: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    pub(crate) on_open_html_editor: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    pub(crate) link_handler: Option<LinkHandler>,
    /// The session's recorded file changes grouped by the user message that
    /// started each turn, built once per fetch. A turn's chip row under its
    /// final assistant message is a lookup here, never a scan or a copy.
    turn_changes: HashMap<String, Rc<TurnChanges>>,
    /// The raw session change list the grouped map was built from, kept for
    /// the whole-session totals in the pinned summary pill.
    session_changes: Rc<[SessionFileChange]>,
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
            turns: Vec::new(),
            live: Vec::new(),
            pending_results: HashMap::new(),
            live_counter: 0,
            markdown_cache: Rc::new(RefCell::new(HashMap::new())),
            presentation_cache: RefCell::new(HashMap::new()),
            content_revisions: Vec::new(),
            list_state,
            scrollbar_state: ScrollbarState::new(),
            prompts_open: false,
            prompts_focus: cx.focus_handle(),
            prompts: Rc::from([]),
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
            just_sent: None,
            tool_calls_state: Rc::new(RefCell::new(ToolCallsState::default())),
            thinking_expanded: Rc::new(RefCell::new(HashSet::new())),
            session_cwd: None,
            has_more: false,
            next_cursor: None,
            loading_older: false,
            on_load_older: None,
            on_preview_image: None,
            on_view_subagent: None,
            load_tool_result: None,
            fetching_results: HashSet::new(),
            on_open_file_raw: None,
            on_open_url_raw: None,
            on_open_html_editor: None,
            link_handler: None,
            turn_changes: HashMap::new(),
            session_changes: Rc::from([]),
            on_open_change: None,
            load_diff: None,
            on_fork: None,
            cite_origin: Rc::new(RefCell::new(None)),
            expanded_turn_changes: Rc::new(RefCell::new(HashSet::new())),
            expanded_user_messages: Rc::new(RefCell::new(HashSet::new())),
        }
    }

    /// Copy server-assigned ids onto this transcript's optimistic user turns.
    /// Pairs user turns from the tail, since the canonical page always ends at
    /// the same latest turn. Per-turn file changes are keyed by that id.
    pub fn adopt_user_message_ids(&mut self, canonical: &[Turn], cx: &mut Context<Self>) {
        if crate::chat::turn_changes::adopt_user_ids(&mut self.turns, canonical) {
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
        self
    }
}

impl Render for TranscriptView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selection = self.selection.clone();
        let entity = cx.entity().downgrade();
        let is_streaming = self.is_streaming;

        // Auto-load older when scrolled near the top (covers wheel, scrollbar,
        // touch).
        if self.has_more && !self.loading_older && !self.turns.is_empty() {
            let top = self.list_state.logical_scroll_top();
            if top.item_ix <= 5 {
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
            .child(if self.turns.is_empty() && !is_streaming {
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
            .child(gpui::deferred(self.render_scroll_controls(theme, cx)));
        if let (Some(line), true) = (cite_line, cite_available)
            && let Some(container) = *cite_origin.borrow()
        {
            return root
                .child(crate::chat::cite_overlay::cite_popup(
                    line, container, selection, theme,
                ))
                .into_any_element();
        }
        root.into_any_element()
    }
}
