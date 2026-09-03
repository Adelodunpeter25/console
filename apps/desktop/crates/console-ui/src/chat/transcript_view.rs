//! Waku-style transcript surface backed by `console-core::AgentMessage`.

use console_core::{AgentMessage, AssistantContentPart, AssistantMessage, ToolResult};
use gpui::{
    App, Context, FocusHandle, FollowMode, IntoElement, ListAlignment, ListOffset, ListState,
    Render, SharedString, Window, canvas, div, list, prelude::*, px,
};
use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::rc::Rc;
use std::sync::Arc;
use std::time::Duration;

use crate::chat::{
    ActivityEvent, AssistantMessageBubble, ToolCallEntry, ToolCalls, ToolCallsAction,
    ToolCallsState, UserMessageBubble, WorkingIndicator,
};
use crate::markdown::render::{self, MarkdownView, TranscriptSelection};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

type MarkdownViewCache = Rc<RefCell<HashMap<(usize, usize), Rc<RefCell<MarkdownView>>>>>;

/// Most chat rows are short, while long Markdown rows are corrected after
/// their first measurement. A realistic estimate reduces initial scroll
/// correction versus the old one-thousand-pixel default.
const ESTIMATED_TRANSCRIPT_ROW_HEIGHT: f32 = 120.0;

#[derive(Clone)]
struct AssistantPresentation {
    content_parts: Rc<Vec<AssistantContentPart>>,
    copy_content: SharedString,
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
    list_state: ListState,
    scrollbar_state: std::rc::Rc<ScrollbarState>,
    focus_handle: FocusHandle,
    selection: TranscriptSelection,
    is_streaming: bool,
    /// Unix seconds when the current run started streaming; the live
    /// "Working for Ns" indicator ticks from this.
    streaming_started_at: Option<i64>,
    /// Invalidates deferred scroll restores from older chat switches.
    scroll_restore_generation: u64,
    /// Disclosure state for grouped tool-call activity. This belongs to the
    /// transcript entity so virtualization does not reset it when rows remount.
    tool_calls_state: Rc<RefCell<ToolCallsState>>,
    /// IDs of thinking blocks the user has expanded. Thinking is collapsed by
    /// default, but the choice survives virtualized row remounts.
    thinking_expanded: Rc<RefCell<HashSet<String>>>,
    /// The active session's working directory, for relative tool paths.
    session_cwd: Option<String>,
    /// Cache for run activity derived from messages. Keyed by user_index,
    /// invalidated when messages or streaming state changes.
    tool_activity_cache: RefCell<HashMap<usize, (u64, Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)>)>>,
    tool_activity_cache_version: std::cell::Cell<u64>,
    /// Pagination for incremental loading: whether older messages exist and cursor.
    has_more: bool,
    next_cursor: Option<i64>,
    /// Whether a pagination fetch is in flight.
    loading_older: bool,
    on_load_older: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    /// Opens the image preview modal with the decoded image when the user
    /// clicks an image inside a message bubble.
    on_preview_image: Option<Rc<dyn Fn(Arc<gpui::Image>, &mut Window, &mut App) + 'static>>,
    on_view_subagent: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
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
            focus_handle: cx.focus_handle(),
            selection: TranscriptSelection::default(),
            is_streaming: false,
            streaming_started_at: None,
            scroll_restore_generation: 0,
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
        }
    }

    /// Wire the image-preview opener (the app opens its modal with the
    /// decoded image). Called by the shell when rendering the transcript.
    pub fn set_on_preview_image(
        &mut self,
        handler: impl Fn(Arc<gpui::Image>, &mut Window, &mut App) + 'static,
    ) {
        self.on_preview_image = Some(Rc::new(handler));
    }

    pub fn set_on_view_subagent(
        &mut self,
        handler: impl Fn(String, &mut Window, &mut App) + 'static,
    ) {
        self.on_view_subagent = Some(Rc::new(handler));
    }

    /// Record the active session's working directory so tool-call rows can
    /// show paths relative to it. Called when a session loads into the pane.
    pub fn set_session_cwd(&mut self, cwd: Option<String>) -> &mut Self {
        self.session_cwd = cwd.filter(|cwd| !cwd.is_empty());
        // Summaries are derived per render, not cached.
        self.tool_activity_cache.borrow_mut().clear();
        self
    }

    pub fn set_pagination(&mut self, has_more: bool, next_cursor: Option<i64>) {
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;
    }

    pub fn set_on_load_older(
        &mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) {
        self.on_load_older = Some(Rc::new(handler));
    }

    pub fn prepend_messages(&mut self, older: Vec<AgentMessage>, has_more: bool, next_cursor: Option<i64>, cx: &mut Context<Self>) {
        if older.is_empty() {
            self.has_more = has_more;
            self.next_cursor = next_cursor;
            self.loading_older = false;
            cx.notify();
            return;
        }
        // Preserve scroll anchor at current top item before prepending
        let anchor = self.scroll_anchor();
        let old_len = self.messages.len();
        // Prepend older messages
        let mut new_messages = older;
        new_messages.extend(self.messages.drain(..));
        self.messages = new_messages;
        // Rebuild revisions for new messages (simple: reset all)
        self.content_revisions = vec![0; self.messages.len()];
        self.presentation_cache.borrow_mut().clear();
        self.markdown_cache.borrow_mut().clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.invalidate_activity_cache();
        self.has_more = has_more;
        self.next_cursor = next_cursor;
        self.loading_older = false;
        // Keep scroll position stable: offset by number of prepended items
        let prepended = self.messages.len() - old_len;
        if let Some((row, offset, _at_tail)) = anchor {
            let new_row = row + prepended;
            // Delay one frame to let list remeasure
            let entity = cx.entity().downgrade();
            cx.spawn(async move |_, cx| {
                cx.background_executor().timer(Duration::from_millis(16)).await;
                cx.update(|cx| {
                    if let Some(e) = entity.upgrade() {
                        e.update(cx, |this, _| {
                            this.list_state.scroll_to(ListOffset {
                                item_ix: new_row,
                                offset_in_item: px(offset),
                            });
                        });
                    }
                });
            }).detach();
        }
        self.refresh_list();
        cx.notify();
    }

    pub fn set_loading_older(&mut self, loading: bool) {
        self.loading_older = loading;
    }

    /// Re-anchor the list after the message set changed: keep following the
    /// tail when the user is already following it (streaming, new prompts),
    /// otherwise leave the scroll position alone.
    fn refresh_list(&self) {
        let row_count = self.row_count();
        if self.list_state.item_count() != row_count {
            self.list_state.reset(row_count);
        }
        if self.list_state.is_following_tail() {
            self.list_state.scroll_to_end();
        }
    }

    /// Invalidate only the row whose content changed. GPUI can retain the
    /// measured heights and virtualization state for every other message.
    fn remeasure_message(&self, message_index: usize) {
        if message_index < self.list_state.item_count() {
            self.list_state
                .remeasure_items(message_index..message_index + 1);
        }
    }

    fn invalidate_activity_cache(&self) {
        self.tool_activity_cache_version
            .set(self.tool_activity_cache_version.get().wrapping_add(1));
        self.tool_activity_cache.borrow_mut().clear();
    }

    /// Drop only the newest user's cached run timeline. Stream deltas,
    /// tool-call upserts/results, and streaming-flag changes can only alter
    /// the run after the last user message, so earlier users' cached
    /// timelines stay valid. Deliberately does NOT bump the cache version:
    /// surviving entries must keep matching it to remain hits.
    fn invalidate_latest_activity_cache(&self) {
        let latest_user = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }));
        if let Some(user_index) = latest_user {
            self.tool_activity_cache.borrow_mut().remove(&user_index);
        }
    }

    /// List rows: the messages plus, while a run is streaming, the trailing
    /// "Working…" indicator — a row like in Waku, so it sits directly below
    /// whatever has streamed in instead of floating at the bottom edge.
    fn row_count(&self) -> usize {
        // Cheap check: does the last user run contain any tool call?
        // Avoid building full ActivityEvent vector just to test existence.
        let has_active_tool_activity = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }))
            .is_some_and(|user_index| {
                self.messages.iter().skip(user_index + 1).any(|message| match message {
                    AgentMessage::Assistant { content, .. } => content
                        .iter()
                        .any(|part| matches!(part, AssistantContentPart::ToolCall { .. })),
                    AgentMessage::User { .. } => false,
                    AgentMessage::ToolResult { .. } => false,
                })
            });
        self.messages.len() + usize::from(self.is_streaming && !has_active_tool_activity)
    }

    /// Return the logical row currently at the top of the viewport and its
    /// offset within that row. This remains stable when Markdown rows change
    /// height, unlike a total pixel offset from the top of the document.
    pub fn scroll_anchor(&self) -> Option<(usize, f32, bool)> {
        if self.messages.is_empty() {
            return None;
        }
        let top = self.list_state.logical_scroll_top();
        Some((
            top.item_ix,
            f32::from(top.offset_in_item),
            self.list_state.is_following_tail(),
        ))
    }

    /// Restore a logical row anchor after a session's message list is installed.
    /// GPUI can then resolve the anchor again as visible rows are measured.
    pub fn restore_scroll_anchor(
        &mut self,
        row_index: usize,
        offset_in_row: f32,
        at_tail: bool,
        cx: &mut Context<Self>,
    ) {
        self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
        let generation = self.scroll_restore_generation;
        self.apply_scroll_anchor(row_index, offset_in_row, at_tail);

        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            cx.background_executor()
                .timer(Duration::from_millis(16))
                .await;
            cx.update(|cx| {
                if let Some(entity) = entity.upgrade() {
                    entity.update(cx, |this, _| {
                        if this.scroll_restore_generation == generation {
                            this.apply_scroll_anchor(row_index, offset_in_row, at_tail);
                        }
                    });
                }
            });
        })
        .detach();
    }

    fn apply_scroll_anchor(&self, row_index: usize, offset_in_row: f32, at_tail: bool) {
        if at_tail {
            self.list_state.set_follow_mode(FollowMode::Tail);
            self.list_state.scroll_to_end();
            return;
        }
        self.list_state.set_follow_mode(FollowMode::Normal);
        let row_index = row_index.min(self.row_count().saturating_sub(1));
        self.list_state.scroll_to(ListOffset {
            item_ix: row_index,
            offset_in_item: px(offset_in_row),
        });
    }

    /// The currently selected transcript text, if the user has a selection.
    pub fn selected_text(&self) -> Option<String> {
        self.selection.selection.borrow().selected_text()
    }

    fn markdown_view_for(
        &self,
        message_index: usize,
        part_index: usize,
    ) -> Rc<RefCell<MarkdownView>> {
        self.markdown_cache
            .borrow_mut()
            .entry((message_index, part_index))
            .or_insert_with(|| Rc::new(RefCell::new(MarkdownView::new())))
            .clone()
    }

    fn assistant_presentation(
        &self,
        message_index: usize,
        content: &[AssistantContentPart],
    ) -> AssistantPresentation {
        let revision = self
            .content_revisions
            .get(message_index)
            .copied()
            .unwrap_or_default();
        let mut cache = self.presentation_cache.borrow_mut();
        if let Some((cached_revision, presentation)) = cache.get(&message_index)
            && *cached_revision == revision
        {
            return presentation.clone();
        }

        let content_parts = Rc::new(content.to_vec());
        let copy_content = content_parts
            .iter()
            .filter_map(|part| match part {
                AssistantContentPart::Text { text, .. }
                | AssistantContentPart::Thinking { text } => Some(text.as_str()),
                AssistantContentPart::ToolCall { .. } | AssistantContentPart::Image { .. } => None,
            })
            .collect::<Vec<_>>()
            .join("\n\n");
        let presentation = AssistantPresentation {
            content_parts,
            copy_content: SharedString::from(copy_content),
        };
        cache.insert(message_index, (revision, presentation.clone()));
        presentation
    }

    fn bump_last_content_revision(&mut self) {
        if let Some(revision) = self.content_revisions.last_mut() {
            *revision = revision.wrapping_add(1);
        }
    }

    pub fn set_messages(&mut self, messages: Vec<AgentMessage>, cx: &mut Context<Self>) {
        // Keep cached views whose message/part identity still exists. When a
        // session is replaced, the next `set_text` compares the new source and
        // safely resets any retained parser state for a reused index.
        let valid_keys = messages
            .iter()
            .enumerate()
            .filter_map(|(message_index, message)| match message {
                AgentMessage::Assistant { content, .. } => Some(
                    content
                        .iter()
                        .enumerate()
                        .filter_map(move |(part_index, part)| {
                            matches!(
                                part,
                                AssistantContentPart::Text { .. }
                                    | AssistantContentPart::Thinking { .. }
                            )
                            .then_some((message_index, part_index))
                        }),
                ),
                _ => None,
            })
            .flatten()
            .collect::<std::collections::HashSet<_>>();
        self.markdown_cache
            .borrow_mut()
            .retain(|key, _| valid_keys.contains(key));
        self.presentation_cache.borrow_mut().clear();
        self.messages = messages;
        self.content_revisions = vec![0; self.messages.len()];
        self.scroll_restore_generation = self.scroll_restore_generation.wrapping_add(1);
        self.selection.clear();
        self.tool_calls_state.borrow_mut().clear();
        self.thinking_expanded.borrow_mut().clear();
        self.invalidate_activity_cache();
        self.is_streaming = false;
        self.streaming_started_at = None;
        // Re-engage tail-following so a freshly loaded session lands on its
        // latest message (or at the top when it is still short).
        self.list_state.set_follow_mode(FollowMode::Tail);
        self.list_state.reset(self.row_count());
        self.list_state.scroll_to_end();
        cx.notify();
    }

    pub fn push_message(&mut self, msg: AgentMessage, cx: &mut Context<Self>) {
        self.messages.push(msg);
        self.content_revisions.push(0);
        // A new prompt means the user wants to see its response: re-engage
        // the tail so streaming stays in view.
        self.list_state.set_follow_mode(FollowMode::Tail);
        self.refresh_list();
        cx.notify();
    }

    pub fn begin_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(chrono::Utc::now().timestamp());
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    /// Re-engage the trailing "Working for Ns" row after `set_messages` reset
    /// it. Used when switching back to a chat whose run is still active: the
    /// started_at is the run's real start time (from `running_sessions`) so the
    /// elapsed counter continues seamlessly instead of restarting from zero.
    pub fn resume_streaming(&mut self, started_at: i64, cx: &mut Context<Self>) {
        self.is_streaming = true;
        self.streaming_started_at = Some(started_at);
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    pub fn finish_streaming(&mut self, cx: &mut Context<Self>) {
        self.is_streaming = false;
        self.streaming_started_at = None;
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    pub fn append_assistant_text(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(AssistantContentPart::Text { text: existing, .. }) = content.last_mut() {
                existing.push_str(text);
            } else {
                content.push(AssistantContentPart::Text {
                    text: text.to_string(),
                    thought_signature: None,
                });
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::Text {
                    text: text.to_string(),
                    thought_signature: None,
                }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
    }

    pub fn append_assistant_thinking(&mut self, text: &str, _cx: &mut Context<Self>) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(AssistantContentPart::Thinking { text: existing }) = content.last_mut() {
                existing.push_str(text);
            } else {
                content.push(AssistantContentPart::Thinking {
                    text: text.to_string(),
                });
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::Thinking {
                    text: text.to_string(),
                }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
    }

    /// Flush the latest coalesced stream state into the list and publish one
    /// transcript repaint. Text deltas update the message buffer immediately,
    /// but this is called only once per stream cadence window.
    pub fn flush_stream_render(&mut self, cx: &mut Context<Self>) {
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        self.refresh_list();
        cx.notify();
    }

    pub fn finalize_assistant_message(
        &mut self,
        message: AssistantMessage,
        cx: &mut Context<Self>,
    ) {
        if let Some(AgentMessage::Assistant {
            id,
            content,
            stop_reason,
            created_at,
        }) = self.messages.last_mut()
        {
            if message.id.is_some() {
                *id = message.id;
            }
            *content = message.content;
            *stop_reason = message.stop_reason;
            if message.created_at.is_some() {
                *created_at = message.created_at;
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: message.id,
                content: message.content,
                stop_reason: message.stop_reason,
                created_at: message.created_at,
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn upsert_assistant_tool_call(
        &mut self,
        call: console_core::ToolCall,
        cx: &mut Context<Self>,
    ) {
        if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
            if let Some(AssistantContentPart::ToolCall { call: existing }) = content.iter_mut().find(|part| {
                matches!(part, AssistantContentPart::ToolCall { call: existing } if existing.id == call.id)
            }) {
                *existing = call;
            } else {
                content.push(AssistantContentPart::ToolCall { call });
            }
            self.bump_last_content_revision();
            self.invalidate_latest_activity_cache();
        } else {
            self.messages.push(AgentMessage::Assistant {
                id: None,
                content: vec![AssistantContentPart::ToolCall { call }],
                stop_reason: None,
                created_at: Some(chrono::Utc::now().timestamp()),
            });
            self.content_revisions.push(0);
            self.invalidate_latest_activity_cache();
        }
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn upsert_assistant_tool_calls(
        &mut self,
        calls: Vec<console_core::ToolCall>,
        cx: &mut Context<Self>,
    ) {
        if calls.is_empty() {
            return;
        }
        for call in calls {
            if let Some(AgentMessage::Assistant { content, .. }) = self.messages.last_mut() {
                if let Some(AssistantContentPart::ToolCall { call: existing }) =
                    content.iter_mut().find(|part| {
                        matches!(part, AssistantContentPart::ToolCall { call: existing } if existing.id == call.id)
                    })
                {
                    *existing = call;
                } else {
                    content.push(AssistantContentPart::ToolCall { call });
                }
            } else {
                self.messages.push(AgentMessage::Assistant {
                    id: None,
                    content: vec![AssistantContentPart::ToolCall { call }],
                    stop_reason: None,
                    created_at: Some(chrono::Utc::now().timestamp()),
                });
                self.content_revisions.push(0);
            }
        }
        // Coalesce to one revision bump / cache invalidation / remeasure
        self.bump_last_content_revision();
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        if let Some(message_index) = self.messages.len().checked_sub(1) {
            self.remeasure_message(message_index);
        }
        cx.notify();
    }

    pub fn append_tool_results(&mut self, results: Vec<ToolResult>, cx: &mut Context<Self>) {
        let existing_ids = self
            .messages
            .iter()
            .filter_map(|message| match message {
                AgentMessage::ToolResult { results, .. } => Some(results),
                _ => None,
            })
            .flatten()
            .map(|result| result.tool_call_id.as_str())
            .collect::<std::collections::HashSet<_>>();
        let results = results
            .into_iter()
            .filter(|result| !existing_ids.contains(result.tool_call_id.as_str()))
            .collect::<Vec<_>>();
        if results.is_empty() {
            return;
        }
        self.messages.push(AgentMessage::ToolResult {
            results,
            created_at: Some(chrono::Utc::now().timestamp()),
        });
        self.content_revisions.push(0);
        self.invalidate_latest_activity_cache();
        self.refresh_list();
        cx.notify();
    }

    /// Collect the tool calls, thinking, and progress text belonging to the
    /// assistant run after a user message, in chronological content order. The
    /// desktop client uses the same boundary: a new user message starts the
    /// next run, and thinking/text between tool calls stay in the timeline.
    fn tool_activity_for_user(
        &self,
        user_index: usize,
    ) -> Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)> {
        let version = self.tool_activity_cache_version.get();
        if let Some((cached_version, cached)) =
            self.tool_activity_cache.borrow().get(&user_index)
        {
            if *cached_version == version {
                return cached.clone();
            }
        }
        let computed = self.compute_tool_activity_for_user(user_index);
        self.tool_activity_cache
            .borrow_mut()
            .insert(user_index, (version, computed.clone()));
        return computed;
    }

    fn compute_tool_activity_for_user(
        &self,
        user_index: usize,
    ) -> Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)> {
        let mut events: Vec<ActivityEvent> = Vec::new();
        let mut results = HashMap::<String, ToolResult>::new();
        let mut ended_at = None;

        for message in self.messages.iter().skip(user_index + 1) {
            if matches!(message, AgentMessage::User { .. }) {
                break;
            }
            match message {
                AgentMessage::Assistant {
                    content,
                    created_at,
                    ..
                } => {
                    ended_at = ended_at
                        .max(created_at.and_then(crate::utils::time::normalize_unix_timestamp));
                    // Mirror Electron: text/thinking only enters the run timeline if
                    // that same turn contains a tool call. A final answer with no
                    // tool calls is rendered as a standalone bubble, not in activity.
                    let has_tool_call_in_message = content
                        .iter()
                        .any(|part| matches!(part, AssistantContentPart::ToolCall { .. }));
                    if !has_tool_call_in_message {
                        continue;
                    }
                    for part in content {
                        match part {
                            AssistantContentPart::Thinking { text }
                                if !text.trim().is_empty() =>
                            {
                                events.push(ActivityEvent::Thinking {
                                    id: format!("run-thinking-{user_index}-{}", events.len()),
                                    text: text.clone(),
                                });
                            }
                            AssistantContentPart::Text { text, .. }
                                if !text.trim().is_empty() =>
                            {
                                events.push(ActivityEvent::Text {
                                    id: format!("run-text-{user_index}-{}", events.len()),
                                    text: text.clone(),
                                });
                            }
                            AssistantContentPart::ToolCall { call } => {
                                events.push(ActivityEvent::ToolCall(ToolCallEntry {
                                    call: call.clone(),
                                    result: None,
                                }));
                            }
                            _ => {}
                        }
                    }
                }
                AgentMessage::ToolResult {
                    results: message_results,
                    created_at,
                } => {
                    ended_at = ended_at
                        .max(created_at.and_then(crate::utils::time::normalize_unix_timestamp));
                    for result in message_results {
                        results.insert(result.tool_call_id.clone(), result.clone());
                    }
                }
                AgentMessage::User { .. } => unreachable!(),
            }
        }

        let has_tool_calls = events
            .iter()
            .any(|event| matches!(event, ActivityEvent::ToolCall(_)));
        if !has_tool_calls {
            return None;
        }
        let events = events
            .into_iter()
            .map(|event| match event {
                ActivityEvent::ToolCall(mut entry) => {
                    entry.result = results.get(&entry.call.id).cloned();
                    ActivityEvent::ToolCall(entry)
                }
                other => other,
            })
            .collect::<Vec<_>>();

        let user_started_at = match self.messages.get(user_index) {
            Some(AgentMessage::User { created_at, .. }) => {
                created_at.and_then(crate::utils::time::normalize_unix_timestamp)
            }
            _ => None,
        };
        let latest_user = self
            .messages
            .iter()
            .rposition(|message| matches!(message, AgentMessage::User { .. }));
        let working = self.is_streaming && latest_user == Some(user_index);
        let started_at = if working {
            self.streaming_started_at.or(user_started_at)
        } else {
            user_started_at
        };
        let elapsed = crate::chat::toolcalls::elapsed_seconds(started_at, ended_at);
        Some((events, started_at, elapsed, working))
    }

    fn apply_thinking_toggle(
        &mut self,
        thinking_id: String,
        expanded: bool,
        cx: &mut Context<Self>,
    ) {
        let mut expanded_state = self.thinking_expanded.borrow_mut();
        if expanded {
            expanded_state.insert(thinking_id);
        } else {
            expanded_state.remove(&thinking_id);
        }
        drop(expanded_state);
        cx.notify();
    }

    fn apply_tool_calls_action(
        &mut self,
        action: ToolCallsAction,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let mut state = self.tool_calls_state.borrow_mut();
        match action {
            ToolCallsAction::ToggleRun { run_id, expanded } => {
                if expanded {
                    state.expanded_runs.insert(run_id.clone());
                    state.collapsed_runs.remove(&run_id);
                } else {
                    state.expanded_runs.remove(&run_id);
                    state.collapsed_runs.insert(run_id);
                }
            }
            ToolCallsAction::ToggleCall { call_id, expanded } => {
                if expanded {
                    state.expanded_calls.insert(call_id);
                } else {
                    state.expanded_calls.remove(&call_id);
                }
            }
            ToolCallsAction::ToggleThinking { id, expanded } => {
                if expanded {
                    state.thinking_expanded.insert(id);
                } else {
                    state.thinking_expanded.remove(&id);
                }
            }
            ToolCallsAction::ViewSubagentInPanel { call_id } => {
                let handler = self.on_view_subagent.clone();
                drop(state);
                if let Some(handler) = handler {
                    handler(call_id, window, cx);
                }
                return;
            }
        }
        drop(state);
        cx.notify();
    }
}

impl Render for TranscriptView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selection = self.selection.clone();
        let entity = cx.entity().downgrade();
        let is_streaming = self.is_streaming;

        // Auto-load older when scrolled to top (covers wheel, scrollbar, touch).
        // Checks on every render: if at top and has_more, trigger load.
        if self.has_more && !self.loading_older && !self.messages.is_empty() {
            let top = self.list_state.logical_scroll_top();
            if top.item_ix == 0 && top.offset_in_item == px(0.0) {
                if let Some(handler) = self.on_load_older.clone() {
                    let window_handle = _window.window_handle();
                    cx.spawn(async move |_, cx| {
                        // Defer to next frame to avoid re-entrancy during render
                        cx.background_executor().timer(std::time::Duration::from_millis(16)).await;
                        let _ = cx.update_window(window_handle, |_, window, cx| {
                            handler(window, cx);
                        });
                    }).detach();
                }
            }
        }

        div()
            .id("transcript-view")
            .key_context("TranscriptView")
            .track_focus(&self.focus_handle)
            .relative()
            .size_full()
            .bg(theme.chat_canvas)
            .child(render::frame_reset(selection.clone()))
            .child(if self.messages.is_empty() && !is_streaming {
                empty_state(theme).into_any_element()
            } else {
                div()
                    .size_full()
                    .flex()
                    .flex_col()
                    .child(
                        list(self.list_state.clone(), move |index, _window, cx| {
                            transcript_row(entity.clone(), index, cx)
                        })
                        .size_full(),
                    )
                    .child(scrollbar::vertical(&self.list_state, &self.scrollbar_state))
                    .into_any_element()
            })
            .child(selection_input(selection))
    }
}

fn transcript_row(
    entity: gpui::WeakEntity<TranscriptView>,
    index: usize,
    cx: &mut gpui::App,
) -> gpui::AnyElement {
    let Some(view) = entity.upgrade() else {
        return div().into_any_element();
    };
    let view_ref = view.read(cx);

    // While a run is streaming, the trailing row is the live working
    // indicator — pulsing dots plus "Working for Ns" — pinned right below
    // whatever content has arrived.
    if index >= view_ref.messages.len() {
        let theme = Theme::current(cx);
        let started_at = view_ref
            .streaming_started_at
            .unwrap_or_else(|| chrono::Utc::now().timestamp());
        return div()
            .w_full()
            .px(px(20.0))
            .py(px(4.0))
            .flex()
            .justify_center()
            .child(
                div()
                    .w_full()
                    .max_w(px(768.0))
                    .child(WorkingIndicator::new(started_at, theme)),
            )
            .into_any_element();
    }

    let Some(message) = view_ref.messages.get(index) else {
        return div().into_any_element();
    };

    // These messages are transport-only after RunActivity takes ownership of
    // their visual output. Return before the normal row padding is applied;
    // rendering an empty child inside the wrapper still reserves vertical
    // space, which creates a gap between the run summary and final response.
    if is_hidden_tool_transport(message) {
        return div().into_any_element();
    }

    let preview_handler = view_ref.on_preview_image.clone();
    let row = match message {
        AgentMessage::User {
            content,
            attachments,
            created_at,
        } => {
            let mut bubble = UserMessageBubble::new(content.clone())
                .created_at(*created_at)
                .selection(view_ref.selection.clone(), format!("message-{index}"));
            if let Some(attachments) = attachments {
                bubble = bubble.attachments(attachments.clone());
            }
            if let Some(handler) = &preview_handler {
                bubble = bubble.on_preview_image(handler.clone());
            }
            let activity = view_ref.tool_activity_for_user(index);
            let activity_state = view_ref.tool_calls_state.clone();
            let activity_entity = entity.clone();
            div()
                .w_full()
                .flex()
                .flex_col()
                .gap(px(6.0))
                .child(bubble)
                .when_some(
                    activity,
                    move |element, (entries, started_at, elapsed, working)| {
                        let activity = ToolCalls::new(
                            format!("run-{index}"),
                            entries,
                            working,
                            started_at,
                            elapsed,
                            activity_state,
                        )
                        .cwd(view_ref.session_cwd.clone())
                        .selection(view_ref.selection.clone())
                        .on_action(move |action, window, cx| {
                            if let Some(view) = activity_entity.upgrade() {
                                view.update(cx, |view, cx| {
                                    view.apply_tool_calls_action(action, window, cx);
                                });
                            }
                        });
                        element.child(activity)
                    },
                )
                .into_any_element()
        }
        AgentMessage::Assistant {
            content,
            created_at,
            ..
        } => {
            let presentation = view_ref.assistant_presentation(index, content);
            let markdown_views = presentation
                .content_parts
                .iter()
                .enumerate()
                .map(|(part_index, part)| match part {
                    AssistantContentPart::Text { .. } | AssistantContentPart::Thinking { .. } => {
                        Some(view_ref.markdown_view_for(index, part_index))
                    }
                    AssistantContentPart::ToolCall { .. } | AssistantContentPart::Image { .. } => {
                        None
                    }
                })
                .collect();
            let thinking_state = view_ref.thinking_expanded.clone();
            let thinking_entity = entity.clone();
            AssistantMessageBubble::new(presentation.content_parts)
                .copy_content(presentation.copy_content)
                .thinking_expanded(thinking_state)
                .on_thinking_toggle(move |thinking_id, expanded, _window, cx| {
                    if let Some(view) = thinking_entity.upgrade() {
                        view.update(cx, |view, cx| {
                            view.apply_thinking_toggle(thinking_id, expanded, cx);
                        });
                    }
                })
                .created_at(*created_at)
                .selection(view_ref.selection.clone(), format!("message-{index}"))
                .markdown_views(markdown_views)
                .streaming(view_ref.is_streaming && index + 1 == view_ref.messages.len())
                .when_some(preview_handler.clone(), |bubble, handler| {
                    bubble.on_preview_image(handler)
                })
                .into_any_element()
        }
        // Tool-result messages are persistence transport. Their results are
        // joined to the calls and rendered in the owning user's run activity.
        AgentMessage::ToolResult { .. } => div().into_any_element(),
    };

    div()
        .w_full()
        .px(px(20.0))
        .py(px(4.0))
        .flex()
        .justify_center()
        .child(div().w_full().max_w(px(768.0)).child(row))
        .into_any_element()
}

fn is_hidden_tool_transport(message: &AgentMessage) -> bool {
    match message {
        AgentMessage::ToolResult { .. } => true,
        AgentMessage::Assistant { content, .. } => content
            .iter()
            .any(|part| matches!(part, AssistantContentPart::ToolCall { .. })),
        AgentMessage::User { .. } => false,
    }
}

fn selection_input(selection: TranscriptSelection) -> impl IntoElement {
    canvas(
        |_, _, _| (),
        move |_, _, window, _| render::install_selection_input(window, &selection),
    )
    .absolute()
    .w(px(0.0))
    .h(px(0.0))
}

fn empty_state(theme: Theme) -> impl IntoElement {
    div()
        .size_full()
        .flex()
        .flex_col()
        .items_center()
        .justify_center()
        .gap(px(8.0))
        .child(app_icon(IconName::Bot, 32.0, theme.text_ghost))
        .child(
            div()
                .text_size(px(14.0))
                .text_color(theme.text_ghost)
                .child("Start a conversation"),
        )
}
