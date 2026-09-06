pub mod actions;
pub mod boundaries;
pub mod element;
pub mod history;
pub mod ime;
pub mod mentions;
pub mod text_runs;

use std::ops::Range;
use std::rc::Rc;
use std::time::{Duration, Instant};

pub use actions::*;
pub use boundaries::*;
use element::{visual_row_count, visual_row_offset_for_x, InputElement};
pub use history::*;
pub use mentions::*;
pub use text_runs::*;

use crate::markdown::highlight::{self, Lang, TokenClass};
use crate::primitives::menu::{context_menu, ContextMenuHandle, MenuItem};
use crate::primitives::scrollbar::{self, ScrollbarState};
use crate::theme::Theme;
use gpui::{
    div, prelude::*, px, App, ClipboardEntry, ClipboardItem, Context, CursorStyle, Entity,
    EntityInputHandler, EventEmitter, FocusHandle, Focusable, IntoElement, MouseButton,
    MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, Point, Render, ScrollHandle,
    SharedString, Subscription, Task, TextLayout, Window,
};

const CURSOR_BLINK_INTERVAL: Duration = Duration::from_millis(500);
const CURSOR_BLINK_PAUSE: Duration = Duration::from_millis(300);

/// Tallest a composer-mode field grows before its text scrolls under an
/// overlay scrollbar instead of growing the card.
pub(crate) const COMPOSER_MAX_HEIGHT: Pixels = px(300.);

pub(crate) struct BlinkCursor {
    pub(crate) visible: bool,
    pub(crate) paused: bool,
    pub(crate) epoch: usize,
    pub(crate) _task: Task<()>,
}

impl BlinkCursor {
    pub(crate) fn new() -> Self {
        Self {
            visible: false,
            paused: false,
            epoch: 0,
            _task: Task::ready(()),
        }
    }

    pub(crate) fn start(&mut self, cx: &mut Context<Self>) {
        self.blink(self.epoch, cx);
    }

    pub(crate) fn stop(&mut self, cx: &mut Context<Self>) {
        self.epoch = 0;
        cx.notify();
    }

    pub(crate) fn visible(&self) -> bool {
        self.paused || self.visible
    }

    pub(crate) fn pause(&mut self, cx: &mut Context<Self>) {
        self.paused = true;
        self.visible = true;
        cx.notify();

        let epoch = self.next_epoch();
        self._task = cx.spawn(async move |this, cx| {
            cx.background_executor().timer(CURSOR_BLINK_PAUSE).await;
            if let Some(this) = this.upgrade() {
                this.update(cx, |this, cx| {
                    this.paused = false;
                    this.blink(epoch, cx);
                });
            }
        });
    }

    fn next_epoch(&mut self) -> usize {
        self.epoch += 1;
        self.epoch
    }

    fn blink(&mut self, epoch: usize, cx: &mut Context<Self>) {
        if self.paused || epoch != self.epoch {
            self.visible = true;
            cx.notify();
            return;
        }

        self.visible = !self.visible;
        cx.notify();

        let epoch = self.next_epoch();
        self._task = cx.spawn(async move |this, cx| {
            cx.background_executor().timer(CURSOR_BLINK_INTERVAL).await;
            if let Some(this) = this.upgrade() {
                this.update(cx, |this, cx| this.blink(epoch, cx));
            }
        });
    }
}

#[derive(Clone)]
pub enum ComposerEvent {
    Submit(String),
    /// Primary modifier + Enter: deliver the message into the running turn instead of queueing
    /// it behind the turn. Only composer-mode fields emit this.
    SubmitSteer(String),
    /// The field took focus. A code editor uses this to re-read its file, so
    /// clicking back into it picks up changes made on disk meanwhile.
    Focus,
    /// The text content actually changed. Parents that derive UI from the
    /// content — filter lists, draft state, dirty markers — react to this,
    /// never to raw notifies: the field also notifies for caret blinks and
    /// selection changes, and re-rendering an owner twice a second for a
    /// blinking caret is exactly the per-frame waste the field exists to
    /// contain.
    Edited,
    /// Backspace in an already-empty composer. The chat idiom for "remove
    /// the last staged attachment"; owners without attachments ignore it.
    BackspaceOnEmpty,
}

/// Clipboard payloads whose primary representation is an image or file list.
/// The owning composer persists them and presents them as attachment chips;
/// code/search fields continue using their ordinary text paste behavior.
#[derive(Clone)]
pub struct ComposerAttachmentPaste(pub Vec<ClipboardEntry>);

/// Respect the representation priority chosen by the source application.
/// Finder puts paths first (and a text fallback second), while screenshots put
/// an image first. Text-first clipboard content remains ordinary text paste.
fn attachment_paste_entries(clipboard: &ClipboardItem) -> Option<Vec<ClipboardEntry>> {
    if !matches!(
        clipboard.entries().first(),
        Some(ClipboardEntry::Image(_) | ClipboardEntry::ExternalPaths(_))
    ) {
        return None;
    }
    let entries = clipboard
        .entries()
        .iter()
        .filter(|entry| {
            matches!(
                entry,
                ClipboardEntry::Image(_) | ClipboardEntry::ExternalPaths(_)
            )
        })
        .cloned()
        .collect::<Vec<_>>();
    (!entries.is_empty()).then_some(entries)
}

/// What the field is for. The difference is small but load-bearing: Enter
/// submits a prompt, inserts a newline in code, and in a search field submits
/// while keeping the query.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum FieldMode {
    #[default]
    Composer,
    Code,
    Search,
}

#[derive(Clone, Copy)]
pub(crate) struct VerticalNavigation {
    /// Desired horizontal position within a visual row. This survives a
    /// shorter intermediate row so moving again can return to the old column.
    pub(crate) goal_x: Pixels,
    /// The visual row carrying the caret, including soft-wrapped rows.
    pub(crate) visual_row: usize,
    /// Actual caret x within `visual_row`, which can be less than `goal_x`
    /// when that row is shorter.
    pub(crate) cursor_x: Pixels,
    pub(crate) cursor_offset: usize,
    pub(crate) layout_width: Pixels,
}

pub struct ComposerInput {
    pub(crate) focus_handle: FocusHandle,
    pub(crate) mode: FieldMode,
    pub(crate) read_only: bool,
    /// The focusing click selects the whole content on release, the way a
    /// browser address bar arms its URL for retyping.
    pub(crate) select_all_on_focus_click: bool,
    /// A plain click landed while the field was unfocused; unless it grows
    /// into a drag-selection first, the release selects everything.
    pub(crate) focus_click_select_all: bool,
    /// Language for paint-only syntax colouring, in code mode.
    pub(crate) language: Option<Lang>,
    /// Cached token spans over `content`, as absolute byte ranges. Recomputed
    /// only when the content changes, so painting a large file is free.
    pub(crate) highlight: Vec<(Range<usize>, TokenClass)>,
    /// Find-in-file match ranges painted as washes under the text, sorted and
    /// non-overlapping. Owned by the find bar, which recomputes them whenever
    /// the content or the query changes; the field only paints them.
    pub(crate) search_matches: Vec<Range<usize>>,
    /// Index into `search_matches` of the match navigation is on, painted
    /// stronger than its siblings.
    pub(crate) active_search_match: Option<usize>,
    pub(crate) content: SharedString,
    pub(crate) placeholder: SharedString,
    pub(crate) selected_range: Range<usize>,
    pub(crate) selection_reversed: bool,
    pub(crate) marked_range: Option<Range<usize>>,
    /// How far a single-line (search-mode) field's text is slid left of its
    /// clipped viewport, in pixels. Reconciled every prepaint to keep the
    /// caret in view; pinned to zero while the field is unfocused so the
    /// address bar's page echo shows the start of the URL.
    pub(crate) scroll_offset: Pixels,
    /// Vertical scroll state for a composer-mode field, whose height is
    /// capped at [`COMPOSER_MAX_HEIGHT`].
    pub(crate) scroll_handle: ScrollHandle,
    pub(crate) scrollbar_state: Rc<ScrollbarState>,
    /// Horizontal inset a composer-mode embedder moves inside the field, so
    /// the scroll viewport — and the overlay scrollbar pinned to its edge —
    /// runs to the card's edge while the text keeps the inset.
    pub(crate) padding_x: Pixels,
    /// The `(caret, content length, wrap width)` the capped viewport last
    /// followed. Prepaint scrolls the caret back into view only when this
    /// changes, so a manual wheel scroll away from the caret holds until the
    /// caret itself next moves.
    pub(crate) caret_reconciled: Option<(usize, usize, Pixels)>,
    pub(crate) last_layout: Option<TextLayout>,
    /// Horizontal goal and soft-wrap affinity for consecutive Up/Down
    /// presses. A byte offset at a wrap boundary can mean either the end of
    /// one visual row or the start of the next, so the offset alone is not
    /// enough to reproduce native textarea movement or paint its caret.
    pub(crate) vertical_navigation: Option<VerticalNavigation>,
    pub(crate) is_selecting: bool,
    pub(crate) selected_word_range: Option<Range<usize>>,
    pub(crate) history: EditHistory,
    pub(crate) prompt_history: PromptHistory,
    pub(crate) external_context_menu_focus_holds: usize,
    pub(crate) context_menu: ContextMenuHandle,
    pub(crate) mentions: Vec<ComposerMention>,
    pub(crate) blink_cursor: Entity<BlinkCursor>,
    pub(crate) _subscriptions: Vec<Subscription>,
}

impl ComposerInput {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let focus_handle = cx.focus_handle();
        let blink_cursor = cx.new(|_| BlinkCursor::new());
        let _subscriptions = vec![
            cx.observe(&blink_cursor, |_, _, cx| cx.notify()),
            cx.observe_window_activation(window, |input, window, cx| {
                if window.is_window_active()
                    && (input.focus_handle.is_focused(window)
                        || input.context_menu_preserves_visual_focus())
                {
                    input.blink_cursor.update(cx, |cursor, cx| cursor.start(cx));
                } else if !window.is_window_active() {
                    input.blink_cursor.update(cx, |cursor, cx| cursor.stop(cx));
                }
            }),
            cx.on_focus(&focus_handle, window, Self::on_focus),
            cx.on_blur(&focus_handle, window, Self::on_blur),
        ];
        Self {
            focus_handle,
            mode: FieldMode::Composer,
            read_only: false,
            select_all_on_focus_click: false,
            focus_click_select_all: false,
            language: None,
            highlight: Vec::new(),
            search_matches: Vec::new(),
            active_search_match: None,
            content: "".into(),
            placeholder: "Message...".into(),
            selected_range: 0..0,
            selection_reversed: false,
            marked_range: None,
            scroll_offset: px(0.),
            scroll_handle: ScrollHandle::new(),
            scrollbar_state: ScrollbarState::new(),
            padding_x: px(0.),
            caret_reconciled: None,
            last_layout: None,
            vertical_navigation: None,
            is_selecting: false,
            selected_word_range: None,
            history: EditHistory::default(),
            prompt_history: PromptHistory::default(),
            external_context_menu_focus_holds: 0,
            context_menu: {
                // The menu takes real focus while open, so the composer holds
                // its caret visible for the duration — otherwise right-clicking
                // the input looks like it defocused.
                let composer = cx.entity().downgrade();
                ContextMenuHandle::new(cx).on_toggle(move |open, window, cx| {
                    let _ = composer.update(cx, |composer: &mut Self, cx| {
                        if open {
                            composer.preserve_visual_focus_for_context_menu(window, cx);
                        } else {
                            composer.release_visual_focus_for_context_menu(window, cx);
                        }
                    });
                })
            },
            mentions: Vec::new(),
            blink_cursor,
            _subscriptions,
        }
    }

    pub fn focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }

    pub fn is_visually_focused(&self, window: &Window) -> bool {
        self.focus_handle.is_focused(window) || self.context_menu_preserves_visual_focus()
    }

    pub fn preserve_visual_focus_for_context_menu(
        &mut self,
        window: &Window,
        cx: &mut Context<Self>,
    ) -> bool {
        if !self.is_visually_focused(window) {
            return false;
        }
        self.external_context_menu_focus_holds += 1;
        cx.notify();
        true
    }

    pub fn release_visual_focus_for_context_menu(
        &mut self,
        window: &Window,
        cx: &mut Context<Self>,
    ) {
        self.external_context_menu_focus_holds =
            self.external_context_menu_focus_holds.saturating_sub(1);
        if !self.is_visually_focused(window) {
            self.blink_cursor.update(cx, |cursor, cx| cursor.stop(cx));
        }
        cx.notify();
    }

    /// Whether this field's right-click menu is open. The browser surface
    /// treats that as an overlay above its native webview.
    pub fn context_menu_open(&self) -> bool {
        self.context_menu.is_open()
    }

    pub(crate) fn context_menu_preserves_visual_focus(&self) -> bool {
        self.external_context_menu_focus_holds > 0
    }

    /// Placeholder shown while the field is empty.
    pub fn placeholder(mut self, placeholder: impl Into<SharedString>) -> Self {
        self.placeholder = placeholder.into();
        self
    }

    /// Replace the placeholder after construction. Picker fields use this to
    /// name the workspace they are searching without recreating the focused
    /// input entity whenever the selected project changes.
    pub fn set_placeholder(
        &mut self,
        placeholder: impl Into<SharedString>,
        cx: &mut Context<Self>,
    ) {
        let placeholder = placeholder.into();
        if self.placeholder != placeholder {
            self.placeholder = placeholder;
            cx.notify();
        }
    }

    /// Horizontal inset kept inside the field's scroll viewport rather than
    /// on the embedding card, so the overlay scrollbar sits at the card's
    /// edge instead of floating next to the text.
    pub fn padding_x(mut self, padding: Pixels) -> Self {
        self.padding_x = padding;
        self
    }

    /// Turn the field into a code editor: Enter inserts a newline instead of
    /// submitting, and `language` (when recognised) colours the text.
    pub fn code_editor(mut self, language: Option<&str>) -> Self {
        self.mode = FieldMode::Code;
        self.language = language.and_then(highlight::lang_for_tag);
        self
    }

    /// Turn the field into a search box: Enter submits without clearing, so
    /// "find next" keeps the query, and pasted line breaks become spaces.
    pub fn search_field(mut self) -> Self {
        self.mode = FieldMode::Search;
        self
    }

    /// Make the focusing click select the whole content on release, the way a
    /// browser address bar arms its URL for retyping. A drag from unfocused
    /// still selects the dragged range, and the next click places the caret.
    pub fn select_all_on_focus_click(mut self) -> Self {
        self.select_all_on_focus_click = true;
        self
    }

    /// Reject edits while still allowing selection and copy.
    pub fn read_only(mut self, read_only: bool) -> Self {
        self.read_only = read_only;
        self
    }

    /// Flip read-only after construction. A file editor starts locked because
    /// its contents are still being read off the UI thread, and unlocks once
    /// the read lands and says the file is writable.
    pub fn set_read_only(&mut self, read_only: bool) {
        self.read_only = read_only;
    }

    /// Re-tokenize after a content change. Cheap for a composer (no language),
    /// one linear pass for a code editor.
    pub(crate) fn refresh_highlight(&mut self) {
        let Some(language) = self.language else {
            return;
        };
        self.highlight.clear();
        let mut line_start = 0;
        for (line, tokens) in self
            .content
            .split('\n')
            .zip(highlight::tokenize(language, &self.content))
        {
            self.highlight.extend(tokens.into_iter().map(|token| {
                (
                    line_start + token.range.start..line_start + token.range.end,
                    token.class,
                )
            }));
            line_start += line.len() + 1;
        }
    }

    pub fn content(&self) -> &str {
        &self.content
    }

    /// The caret's byte offset into `content`.
    pub fn cursor(&self) -> usize {
        self.cursor_offset()
    }

    pub fn mentions(&self) -> &[ComposerMention] {
        &self.mentions
    }

    /// Insert an accepted file mention chip into the composer.
    /// Inserts `<path> ` into the underlying text buffer and tracks the `<path>` range as a chip.
    pub fn insert_file_mention(&mut self, range: Range<usize>, path: &str, cx: &mut Context<Self>) {
        let insert_text = format!("{} ", path);
        let mention_len = path.len();
        let start = range.start.min(self.content.len());
        self.replace_range(range, &insert_text, cx);
        let mention_range = start..(start + mention_len);
        self.mentions.retain(|m| m.range.end <= start || m.range.start >= start + insert_text.len());
        self.mentions.push(ComposerMention {
            range: mention_range,
            path: path.to_string(),
        });
        self.reconcile_mentions();
        cx.notify();
    }

    pub fn add_mention(&mut self, range: Range<usize>, path: impl Into<String>) {
        self.mentions.push(ComposerMention {
            range,
            path: path.into(),
        });
        self.reconcile_mentions();
    }

    fn adjust_mentions(&mut self, edit_range: &Range<usize>, new_text_len: usize) {
        mentions::adjust_mentions(&mut self.mentions, edit_range, new_text_len);
    }

    fn reconcile_mentions(&mut self) {
        mentions::reconcile_mentions(&mut self.mentions, &self.content);
    }

    /// Splice `text` over `range` and put the caret after it. This is the
    /// autocompletion insert: unlike [`EntityInputHandler::replace_text_in_range`]
    /// it takes byte offsets and no window, so an action handler can call it.
    pub fn replace_range(&mut self, range: Range<usize>, text: &str, cx: &mut Context<Self>) {
        if self.read_only {
            return;
        }
        let range = range.start.min(self.content.len())..range.end.min(self.content.len());
        if !self.content.is_char_boundary(range.start)
            || !self.content.is_char_boundary(range.end)
            || range.start > range.end
        {
            return;
        }
        // Editing a recalled prompt leaves history navigation mode; the next
        // Up starts a new navigation with this edited text as the draft.
        self.prompt_history.reset_navigation();
        // A discrete undo step: picking a completion or replacing a match
        // must not coalesce with the typing around it.
        self.history.seal();
        self.record_edit_history(&range, text, false);
        self.adjust_mentions(&range, text.len());
        self.content =
            (self.content[..range.start].to_owned() + text + &self.content[range.end..]).into();
        let offset = range.start + text.len();
        self.selected_range = offset..offset;
        self.selection_reversed = false;
        self.marked_range = None;
        self.vertical_navigation = None;
        self.history.seal();
        self.refresh_highlight();
        self.reconcile_mentions();
        self.pause_blink_cursor(cx);
        cx.emit(ComposerEvent::Edited);
        cx.notify();
    }

    /// Replace the painted find-match washes. Ranges must be sorted and
    /// non-overlapping; `active` indexes into `matches`. Purely visual — the
    /// content is untouched, so no [`ComposerEvent::Edited`] is emitted.
    pub fn set_search_matches(
        &mut self,
        matches: Vec<Range<usize>>,
        active: Option<usize>,
        cx: &mut Context<Self>,
    ) {
        if self.search_matches == matches && self.active_search_match == active {
            return;
        }
        self.search_matches = matches;
        self.active_search_match = active;
        cx.notify();
    }

    pub fn selected_range(&self) -> Range<usize> {
        self.selected_range.clone()
    }

    /// Move the selection to `range`, as find-next does when it lands on a
    /// match. Ignored unless the range sits on character boundaries, so a
    /// selection computed against stale content cannot split a code point.
    pub fn select_range(&mut self, range: Range<usize>, cx: &mut Context<Self>) {
        if range.start > range.end
            || range.end > self.content.len()
            || !self.content.is_char_boundary(range.start)
            || !self.content.is_char_boundary(range.end)
        {
            return;
        }
        self.selected_range = range;
        self.selection_reversed = false;
        self.vertical_navigation = None;
        self.pause_blink_cursor(cx);
        cx.notify();
    }

    /// Select the whole content, the way reopening a find bar re-arms its
    /// query for retyping. Unlike the `SelectAll` action this needs no window.
    pub fn select_all_text(&mut self, cx: &mut Context<Self>) {
        self.selected_range = 0..self.content.len();
        self.selection_reversed = false;
        self.vertical_navigation = None;
        self.pause_blink_cursor(cx);
        cx.notify();
    }

    /// Where `offset` sits in the window as of the last paint, with the line
    /// height, so a find bar can scroll a match into view. `None` until the
    /// field has painted once.
    pub fn position_for_offset(&self, offset: usize) -> Option<(Point<Pixels>, Pixels)> {
        let layout = self.last_layout.as_ref()?;
        let position = layout.position_for_index(offset.min(self.content.len()))?;
        Some((position, layout.line_height()))
    }

    /// Height of each logical line as laid out, so a gutter can put one number
    /// per line even when soft wrap gives a line several visual rows.
    ///
    /// Read from the previous frame's layout — a gutter is therefore one frame
    /// behind a reflow, which is invisible next to the reflow itself.
    pub fn wrapped_line_heights(&self) -> Vec<Pixels> {
        let Some(layout) = self.last_layout.as_ref() else {
            return Vec::new();
        };
        let line_height = layout.line_height();
        layout
            .line_layouts()
            .iter()
            .map(|line| line_height * (line.wrap_boundaries().len() + 1) as f32)
            .collect()
    }

    pub fn clear(&mut self, cx: &mut Context<Self>) {
        self.prompt_history.reset_navigation();
        let changed = !self.content.is_empty();
        self.content = "".into();
        self.selected_range = 0..0;
        self.selection_reversed = false;
        self.marked_range = None;
        self.vertical_navigation = None;
        self.highlight.clear();
        self.mentions.clear();
        // A programmatic clear is a new baseline, not an edit to step back
        // over — a submitted prompt should not resurface via the undo shortcut.
        if changed {
            self.history = EditHistory::default();
        }
        self.pause_blink_cursor(cx);
        if changed {
            cx.emit(ComposerEvent::Edited);
        }
        cx.notify();
    }

    pub fn set_content(&mut self, content: impl Into<SharedString>, cx: &mut Context<Self>) {
        self.prompt_history.reset_navigation();
        let content = content.into();
        let changed = self.content != content;
        self.content = content;
        let offset = self.content.len();
        self.selected_range = offset..offset;
        self.selection_reversed = false;
        self.marked_range = None;
        self.vertical_navigation = None;
        self.mentions.clear();
        // A load or reload from disk is a new baseline: undoing into text
        // from before an external change would silently revert that change.
        // An unchanged reload keeps the history alive.
        if changed {
            self.history = EditHistory::default();
        }
        self.refresh_highlight();
        self.pause_blink_cursor(cx);
        if changed {
            cx.emit(ComposerEvent::Edited);
        }
        cx.notify();
    }

    fn on_focus(&mut self, _: &mut Window, cx: &mut Context<Self>) {
        // Regaining focus is a gesture boundary — Zed finalizes its last
        // transaction here too — so edits from separate visits never merge
        // into one undo step.
        self.history.seal();
        self.blink_cursor.update(cx, |cursor, cx| cursor.start(cx));
        cx.emit(ComposerEvent::Focus);
    }

    fn on_blur(&mut self, _: &mut Window, cx: &mut Context<Self>) {
        // An armed focusing click dies with the focus it was tied to.
        self.focus_click_select_all = false;
        if self.context_menu_preserves_visual_focus() {
            cx.notify();
            return;
        }
        self.blink_cursor.update(cx, |cursor, cx| cursor.stop(cx));
    }

    pub(crate) fn pause_blink_cursor(&mut self, cx: &mut Context<Self>) {
        self.blink_cursor.update(cx, |cursor, cx| cursor.pause(cx));
    }

    fn left(&mut self, _: &Left, _: &mut Window, cx: &mut Context<Self>) {
        if self.selected_range.is_empty() {
            self.move_to(self.previous_boundary(self.cursor_offset()), cx);
        } else {
            self.move_to(self.selected_range.start, cx);
        }
    }

    fn right(&mut self, _: &Right, _: &mut Window, cx: &mut Context<Self>) {
        if self.selected_range.is_empty() {
            self.move_to(self.next_boundary(self.cursor_offset()), cx);
        } else {
            self.move_to(self.selected_range.end, cx);
        }
    }

    fn up(&mut self, _: &Up, _: &mut Window, cx: &mut Context<Self>) {
        if self.mode == FieldMode::Composer
            && (self.prompt_history.is_navigating() || self.is_at_visual_boundary(false))
            && self.navigate_prompt_history(false, cx)
        {
            return;
        }
        self.move_vertically(false, cx);
    }

    fn down(&mut self, _: &Down, _: &mut Window, cx: &mut Context<Self>) {
        if self.mode == FieldMode::Composer
            && self.prompt_history.is_navigating()
            && self.navigate_prompt_history(true, cx)
        {
            return;
        }
        self.move_vertically(true, cx);
    }

    fn is_at_visual_boundary(&self, first: bool) -> bool {
        if !self.selected_range.is_empty() {
            return false;
        }
        let Some(layout) = self
            .last_layout
            .as_ref()
            .filter(|layout| layout.len() == self.content.len())
        else {
            return !self.content.contains('\n');
        };
        let Some(position) = layout.position_for_index(self.cursor_offset()) else {
            return false;
        };
        let row = ((position.y - layout.bounds().top()) / layout.line_height()) as usize;
        let row_count = visual_row_count(layout);
        if first {
            row == 0
        } else {
            row + 1 >= row_count
        }
    }

    fn navigate_prompt_history(&mut self, down: bool, cx: &mut Context<Self>) -> bool {
        let Some(content) = self.prompt_history.navigate(down, &self.content) else {
            return false;
        };
        self.content = content.into();
        let offset = self.content.len();
        self.selected_range = offset..offset;
        self.selection_reversed = false;
        self.marked_range = None;
        self.vertical_navigation = None;
        self.refresh_highlight();
        self.pause_blink_cursor(cx);
        cx.emit(ComposerEvent::Edited);
        cx.notify();
        true
    }

    /// Replace the current composer prompt with one of the submitted user
    /// prompts. Moving down past the newest entry restores the draft captured
    /// when history navigation began.
    pub fn set_prompt_history(&mut self, entries: Vec<String>, cx: &mut Context<Self>) {
        self.prompt_history.set_entries(entries);
        cx.notify();
    }

    /// Record a prompt submitted by the owning app. Consecutive duplicates are
    /// collapsed, while older repeated prompts remain valid history entries.
    pub fn record_prompt_history(&mut self, prompt: impl Into<String>, cx: &mut Context<Self>) {
        self.prompt_history.record(prompt.into());
        cx.notify();
    }

    /// Move by one rendered row, not merely one newline-delimited line. This
    /// is the textarea convention: soft wraps count, and the original x goal
    /// survives a shorter row between two longer ones.
    fn move_vertically(&mut self, down: bool, cx: &mut Context<Self>) {
        if self.mode == FieldMode::Search {
            self.vertical_navigation = None;
            cx.propagate();
            return;
        }

        let anchor = if down {
            self.selected_range.end
        } else {
            self.selected_range.start
        };
        let Some(layout) = self
            .last_layout
            .as_ref()
            .filter(|layout| layout.len() == self.content.len())
        else {
            // The field normally has a current layout whenever it can receive
            // a key. If an edit and this action race the next paint, let an
            // enclosing surface handle the arrow rather than navigating with
            // stale geometry.
            self.vertical_navigation = None;
            cx.propagate();
            return;
        };
        let row_count = visual_row_count(layout);
        if row_count == 0 {
            cx.propagate();
            return;
        }

        let bounds = layout.bounds();
        let layout_width = bounds.size.width;
        let continuing = if self.selected_range.is_empty() {
            self.vertical_navigation.filter(|navigation| {
                navigation.cursor_offset == anchor
                    && navigation.layout_width == layout_width
                    && navigation.visual_row < row_count
            })
        } else {
            None
        };
        let (current_row, goal_x) = if let Some(navigation) = continuing {
            (navigation.visual_row, navigation.goal_x)
        } else {
            let Some(position) = layout.position_for_index(anchor) else {
                self.vertical_navigation = None;
                cx.propagate();
                return;
            };
            let row = ((position.y - bounds.top()) / layout.line_height()) as usize;
            (row.min(row_count - 1), position.x - bounds.left())
        };
        let target_row = if down {
            (current_row + 1).min(row_count - 1)
        } else {
            current_row.saturating_sub(1)
        };
        let Some((offset, cursor_x)) = visual_row_offset_for_x(layout, target_row, goal_x) else {
            self.vertical_navigation = None;
            cx.propagate();
            return;
        };

        let previous_range = self.selected_range.clone();
        let previous_row = continuing.map(|navigation| navigation.visual_row);
        self.selected_range = offset..offset;
        self.selection_reversed = false;
        self.vertical_navigation = Some(VerticalNavigation {
            goal_x,
            visual_row: target_row,
            cursor_x,
            cursor_offset: offset,
            layout_width,
        });
        self.pause_blink_cursor(cx);
        cx.notify();

        // Match Zed/native controls at the boundary: if neither the text
        // selection nor its soft-wrap affinity moved, an enclosing surface
        // gets a chance to use the arrow.
        if previous_range == self.selected_range && previous_row == Some(target_row) {
            cx.propagate();
        }
    }

    fn select_left(&mut self, _: &SelectLeft, _: &mut Window, cx: &mut Context<Self>) {
        self.select_to(self.previous_boundary(self.cursor_offset()), cx);
    }

    fn select_right(&mut self, _: &SelectRight, _: &mut Window, cx: &mut Context<Self>) {
        self.select_to(self.next_boundary(self.cursor_offset()), cx);
    }

    fn select_all(&mut self, _: &SelectAll, _: &mut Window, cx: &mut Context<Self>) {
        self.move_to(0, cx);
        self.select_to(self.content.len(), cx);
    }

    fn home(&mut self, _: &Home, _: &mut Window, cx: &mut Context<Self>) {
        self.move_to(0, cx);
    }

    fn end(&mut self, _: &End, _: &mut Window, cx: &mut Context<Self>) {
        self.move_to(self.content.len(), cx);
    }

    fn move_to_previous_word(
        &mut self,
        _: &MoveToPreviousWord,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let offset = if self.selected_range.is_empty() {
            previous_word_boundary(&self.content, self.cursor_offset())
        } else {
            self.selected_range.start
        };
        self.move_to(offset, cx);
    }

    fn move_to_next_word(&mut self, _: &MoveToNextWord, _: &mut Window, cx: &mut Context<Self>) {
        let offset = if self.selected_range.is_empty() {
            next_word_boundary(&self.content, self.cursor_offset())
        } else {
            self.selected_range.end
        };
        self.move_to(offset, cx);
    }

    fn select_to_start(&mut self, _: &SelectToStart, _: &mut Window, cx: &mut Context<Self>) {
        self.select_to(0, cx);
    }

    fn select_to_end(&mut self, _: &SelectToEnd, _: &mut Window, cx: &mut Context<Self>) {
        self.select_to(self.content.len(), cx);
    }

    fn select_to_previous_word(
        &mut self,
        _: &SelectToPreviousWord,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.select_to(
            previous_word_boundary(&self.content, self.cursor_offset()),
            cx,
        );
    }

    fn select_to_next_word(
        &mut self,
        _: &SelectToNextWord,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.select_to(next_word_boundary(&self.content, self.cursor_offset()), cx);
    }

    fn backspace(&mut self, _: &Backspace, window: &mut Window, cx: &mut Context<Self>) {
        if matches!(self.mode, FieldMode::Composer) && self.content.is_empty() {
            cx.emit(ComposerEvent::BackspaceOnEmpty);
            return;
        }
        if matches!(self.mode, FieldMode::Composer) && self.selected_range.is_empty() {
            let cursor = self.cursor_offset();
            if let Some(mention) = self.mentions.iter().find(|m| m.range.end == cursor).cloned() {
                self.selected_range = mention.range;
                self.replace_text_in_range(None, "", window, cx);
                return;
            }
        }
        if self.selected_range.is_empty() {
            self.select_to(self.previous_boundary(self.cursor_offset()), cx);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn delete(&mut self, _: &Delete, window: &mut Window, cx: &mut Context<Self>) {
        if matches!(self.mode, FieldMode::Composer) && self.selected_range.is_empty() {
            let cursor = self.cursor_offset();
            if let Some(mention) = self.mentions.iter().find(|m| m.range.start == cursor).cloned() {
                self.selected_range = mention.range;
                self.replace_text_in_range(None, "", window, cx);
                return;
            }
        }
        if self.selected_range.is_empty() {
            self.select_to(self.next_boundary(self.cursor_offset()), cx);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn delete_to_start(&mut self, _: &DeleteToStart, window: &mut Window, cx: &mut Context<Self>) {
        if self.selected_range.is_empty() {
            self.select_to(0, cx);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn delete_to_end(&mut self, _: &DeleteToEnd, window: &mut Window, cx: &mut Context<Self>) {
        if self.selected_range.is_empty() {
            self.select_to(self.content.len(), cx);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn delete_to_previous_word(
        &mut self,
        _: &DeleteToPreviousWord,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.selected_range.is_empty() {
            self.select_to(
                previous_word_boundary(&self.content, self.cursor_offset()),
                cx,
            );
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn delete_to_next_word(
        &mut self,
        _: &DeleteToNextWord,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.selected_range.is_empty() {
            self.select_to(next_word_boundary(&self.content, self.cursor_offset()), cx);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn enter(&mut self, _: &Enter, window: &mut Window, cx: &mut Context<Self>) {
        match self.mode {
            FieldMode::Code => {
                self.replace_text_in_range(None, "\n", window, cx);
            }
            // A search query survives its own submission — Enter means "find
            // next", not "send" — and stays untrimmed because leading or
            // trailing spaces are part of what is searched for.
            FieldMode::Search => {
                cx.emit(ComposerEvent::Submit(self.content.to_string()));
            }
            FieldMode::Composer => {
                let value = self.content.trim().to_owned();
                if !value.is_empty() {
                    self.prompt_history.record(value.clone());
                    cx.emit(ComposerEvent::Submit(value));
                    self.clear(cx);
                }
            }
        }
    }

    fn newline(&mut self, _: &Newline, window: &mut Window, cx: &mut Context<Self>) {
        if self.mode == FieldMode::Search {
            // Find bars and picker fields assign Shift+Enter their own meaning.
            cx.propagate();
            return;
        }
        self.replace_text_in_range(None, "\n", window, cx);
    }

    fn submit_steer(&mut self, _: &SubmitSteer, _: &mut Window, cx: &mut Context<Self>) {
        if self.mode != FieldMode::Composer {
            // Search and code fields have no running turn to steer; let an
            // outer handler claim the primary-modifier Enter shortcut instead of swallowing it.
            cx.propagate();
            return;
        }
        let value = self.content.trim().to_owned();
        if !value.is_empty() {
            self.prompt_history.record(value.clone());
            cx.emit(ComposerEvent::SubmitSteer(value));
            self.clear(cx);
        }
    }

    fn paste(&mut self, _: &Paste, window: &mut Window, cx: &mut Context<Self>) {
        let Some(clipboard) = cx.read_from_clipboard() else {
            return;
        };
        if self.mode == FieldMode::Composer
            && let Some(entries) = attachment_paste_entries(&clipboard)
        {
            cx.emit(ComposerAttachmentPaste(entries));
            return;
        }
        let Some(text) = clipboard.text() else {
            return;
        };
        let text = match self.mode {
            // A composer is one prompt — and a search box one query — so
            // pasted line breaks become spaces.
            FieldMode::Composer | FieldMode::Search => text.replace(['\n', '\r'], " "),
            FieldMode::Code => text.replace('\r', ""),
        };
        // A paste is its own undo step, never part of the typing around it —
        // the native NSTextView boundary, stricter than Zed's time grouping.
        self.history.seal();
        self.replace_text_in_range(None, &text, window, cx);
        self.history.seal();
    }

    fn copy(&mut self, _: &Copy, _: &mut Window, cx: &mut Context<Self>) {
        if self.selected_range.is_empty() {
            // Nothing here to copy. The composer holds focus almost all the
            // time, so propagating lets an outer handler — the transcript's
            // text selection — answer the keystroke instead of swallowing it.
            cx.propagate();
            return;
        }
        cx.write_to_clipboard(ClipboardItem::new_string(
            self.content[self.selected_range.clone()].to_string(),
        ));
    }

    fn cut(&mut self, _: &Cut, window: &mut Window, cx: &mut Context<Self>) {
        if !self.selected_range.is_empty() {
            cx.write_to_clipboard(ClipboardItem::new_string(
                self.content[self.selected_range.clone()].to_string(),
            ));
            // Like paste, a cut never coalesces with surrounding deletions.
            self.history.seal();
            self.replace_text_in_range(None, "", window, cx);
            self.history.seal();
        }
    }

    /// Route a splice into the history before it is applied: composition
    /// splices amend the open composition step, everything else records —
    /// and possibly coalesces — normally.
    pub(crate) fn record_edit_history(&mut self, range: &Range<usize>, new_text: &str, composing: bool) {
        if composing {
            self.history.record_composition(
                &self.content,
                range,
                new_text,
                self.selected_range.clone(),
                self.selection_reversed,
                Instant::now(),
            );
        } else {
            self.history.record(
                &self.content,
                range,
                new_text,
                self.selected_range.clone(),
                self.selection_reversed,
                Instant::now(),
            );
        }
    }

    fn undo(&mut self, _: &Undo, _: &mut Window, cx: &mut Context<Self>) {
        // While text is marked the IME owns the field; undoing under an
        // active composition would desync it.
        if self.read_only || self.marked_range.is_some() {
            return;
        }
        let Some((content, selection, selection_reversed)) = self.history.undo(&self.content)
        else {
            return;
        };
        self.apply_history_step(content, selection, selection_reversed, cx);
    }

    fn redo(&mut self, _: &Redo, _: &mut Window, cx: &mut Context<Self>) {
        if self.read_only || self.marked_range.is_some() {
            return;
        }
        let Some((content, selection, selection_reversed)) = self.history.redo(&self.content)
        else {
            return;
        };
        self.apply_history_step(content, selection, selection_reversed, cx);
    }

    fn apply_history_step(
        &mut self,
        content: String,
        selection: Range<usize>,
        selection_reversed: bool,
        cx: &mut Context<Self>,
    ) {
        self.content = content.into();
        self.selected_range = selection;
        self.selection_reversed = selection_reversed;
        self.marked_range = None;
        self.vertical_navigation = None;
        self.refresh_highlight();
        self.reconcile_mentions();
        self.pause_blink_cursor(cx);
        cx.emit(ComposerEvent::Edited);
        cx.notify();
    }

    fn on_mouse_down(
        &mut self,
        event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.vertical_navigation = None;
        self.is_selecting = true;
        self.selected_word_range = None;
        // A plain click that is also the focusing click arms select-all for
        // its release. This handler runs before gpui's focus-on-mouse-down
        // transfer (user listeners dispatch first in the bubble phase), so
        // the pre-click focus is still observable here.
        self.focus_click_select_all = self.select_all_on_focus_click
            && event.click_count == 1
            && !event.modifiers.shift
            && !self.is_visually_focused(window);
        let offset = self.index_for_mouse_position(event.position);

        if event.click_count >= 3 {
            self.selected_range = 0..self.content.len();
            self.selection_reversed = false;
            self.selected_word_range = Some(self.selected_range.clone());
            self.pause_blink_cursor(cx);
            cx.notify();
            return;
        }

        if event.click_count == 2 {
            let range = word_range_at(&self.content, offset);
            self.selected_range = range.clone();
            self.selection_reversed = false;
            self.selected_word_range = (!range.is_empty()).then_some(range);
            self.pause_blink_cursor(cx);
            cx.notify();
            return;
        }

        if event.modifiers.shift {
            self.select_to(offset, cx);
        } else {
            self.move_to(offset, cx);
        }
    }

    fn on_context_mouse_down(
        &mut self,
        _: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        window.focus(&self.focus_handle, cx);
    }

    fn on_mouse_up(&mut self, _: &MouseUpEvent, _: &mut Window, cx: &mut Context<Self>) {
        self.is_selecting = false;
        self.selected_word_range = None;
        if self.focus_click_select_all {
            self.focus_click_select_all = false;
            self.select_all_text(cx);
        }
    }

    pub(crate) fn on_mouse_move(&mut self, event: &MouseMoveEvent, _: &mut Window, cx: &mut Context<Self>) {
        if self.is_selecting {
            self.select_to(self.index_for_mouse_position(event.position), cx);
            // Growing a real drag-selection turns the focusing click into a
            // range selection; releasing must keep it.
            if !self.selected_range.is_empty() {
                self.focus_click_select_all = false;
            }
        }
    }

    fn move_to(&mut self, offset: usize, cx: &mut Context<Self>) {
        self.selected_range = offset..offset;
        self.selection_reversed = false;
        self.vertical_navigation = None;
        self.pause_blink_cursor(cx);
        cx.notify();
    }

    pub(crate) fn cursor_offset(&self) -> usize {
        if self.selection_reversed {
            self.selected_range.start
        } else {
            self.selected_range.end
        }
    }

    fn index_for_mouse_position(&self, position: Point<Pixels>) -> usize {
        if self.content.is_empty() {
            return 0;
        }
        let Some(layout) = self.last_layout.as_ref() else {
            return 0;
        };
        layout
            .index_for_position(position)
            .unwrap_or_else(|index| index)
            .min(self.content.len())
    }

    fn select_to(&mut self, offset: usize, cx: &mut Context<Self>) {
        self.vertical_navigation = None;
        if self.selection_reversed {
            self.selected_range.start = offset;
        } else {
            self.selected_range.end = offset;
        }
        if self.selected_range.end < self.selected_range.start {
            self.selection_reversed = !self.selection_reversed;
            self.selected_range = self.selected_range.end..self.selected_range.start;
        }
        if let Some(word_range) = self.selected_word_range.as_ref() {
            self.selected_range.start = self.selected_range.start.min(word_range.start);
            self.selected_range.end = self.selected_range.end.max(word_range.end);
        }
        self.pause_blink_cursor(cx);
        cx.notify();
    }

    #[allow(dead_code)]
    pub(crate) fn offset_from_utf16(&self, offset: usize) -> usize {
        boundaries::offset_from_utf16(&self.content, offset)
    }

    pub(crate) fn offset_to_utf16(&self, offset: usize) -> usize {
        boundaries::offset_to_utf16(&self.content, offset)
    }

    pub(crate) fn range_to_utf16(&self, range: &Range<usize>) -> Range<usize> {
        boundaries::range_to_utf16(&self.content, range)
    }

    pub(crate) fn range_from_utf16(&self, range: &Range<usize>) -> Range<usize> {
        boundaries::range_from_utf16(&self.content, range)
    }

    pub(crate) fn range_from_relative_utf16(&self, base: usize, range: &Range<usize>) -> Range<usize> {
        boundaries::range_from_relative_utf16(&self.content, base, range)
    }

    fn previous_boundary(&self, offset: usize) -> usize {
        previous_grapheme_boundary(&self.content, offset)
    }

    fn next_boundary(&self, offset: usize) -> usize {
        next_grapheme_boundary(&self.content, offset)
    }
}

impl EventEmitter<ComposerEvent> for ComposerInput {}
impl EventEmitter<ComposerAttachmentPaste> for ComposerInput {}

impl Render for ComposerInput {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let input = cx.entity();
        let context_menu_input = input.clone();
        let scroll_handle = self.scroll_handle.clone();
        let padding_x = self.padding_x;
        let scrollbar = (self.mode == FieldMode::Composer)
            .then(|| scrollbar::vertical(&self.scroll_handle, &self.scrollbar_state));
        let field = div()
            .key_context("ComposerInput")
            .id("composer-field")
            .track_focus(&self.focus_handle(cx))
            .cursor(CursorStyle::IBeam)
            .on_action(cx.listener(Self::backspace))
            .on_action(cx.listener(Self::delete))
            .on_action(cx.listener(Self::left))
            .on_action(cx.listener(Self::right))
            .on_action(cx.listener(Self::up))
            .on_action(cx.listener(Self::down))
            .on_action(cx.listener(Self::select_left))
            .on_action(cx.listener(Self::select_right))
            .on_action(cx.listener(Self::select_all))
            .on_action(cx.listener(Self::home))
            .on_action(cx.listener(Self::end))
            .on_action(cx.listener(Self::move_to_previous_word))
            .on_action(cx.listener(Self::move_to_next_word))
            .on_action(cx.listener(Self::select_to_start))
            .on_action(cx.listener(Self::select_to_end))
            .on_action(cx.listener(Self::select_to_previous_word))
            .on_action(cx.listener(Self::select_to_next_word))
            .on_action(cx.listener(Self::delete_to_start))
            .on_action(cx.listener(Self::delete_to_end))
            .on_action(cx.listener(Self::delete_to_previous_word))
            .on_action(cx.listener(Self::delete_to_next_word))
            .on_action(cx.listener(Self::paste))
            .on_action(cx.listener(Self::cut))
            .on_action(cx.listener(Self::copy))
            .on_action(cx.listener(Self::undo))
            .on_action(cx.listener(Self::redo))
            .on_action(cx.listener(Self::enter))
            .on_action(cx.listener(Self::newline))
            .on_action(cx.listener(Self::submit_steer))
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_down(MouseButton::Right, cx.listener(Self::on_context_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .w_full()
            .text_color(theme.text)
            // A composer owns its own metrics; a code editor inherits the
            // caller's, so a gutter beside it can rely on the same line height.
            .when(self.mode == FieldMode::Composer, |field| {
                field
                    .min_h(px(24.0))
                    .max_h(COMPOSER_MAX_HEIGHT)
                    .overflow_y_scroll()
                    .track_scroll(&scroll_handle)
                    .px(padding_x)
                    .line_height(px(22.0))
                    .text_size(px(13.5))
            })
            // A search-mode field is visually one line: the text never wraps,
            // and the overlong remainder slides horizontally under this
            // clipped viewport to follow the caret — no scrollbar.
            .when(self.mode == FieldMode::Search, |field| {
                field.whitespace_nowrap().overflow_hidden()
            })
            .child(InputElement { input });

        context_menu(
            div().w_full().child(field).children(scrollbar),
            "composer-context-menu",
            &self.context_menu,
            move |cx| {
                let (has_selection, has_content, all_selected) = {
                    let input = context_menu_input.read(cx);
                    let has_selection = !input.selected_range.is_empty();
                    let has_content = !input.content.is_empty();
                    let all_selected = has_content
                        && input.selected_range.start == 0
                        && input.selected_range.end == input.content.len();
                    (has_selection, has_content, all_selected)
                };
                let can_paste = cx
                    .read_from_clipboard()
                    .and_then(|item| item.text())
                    .is_some();

                // Call the editing methods directly rather than dispatching the
                // actions: by the time an item runs, focus is still unwinding
                // from the menu card, so a dispatch would have nowhere to land.
                let run = |input: &Entity<ComposerInput>,
                           action: fn(
                    &mut ComposerInput,
                    &mut Window,
                    &mut Context<ComposerInput>,
                )| {
                    let input = input.clone();
                    move |window: &mut Window, cx: &mut App| {
                        let focus = input.read(cx).focus_handle.clone();
                        window.focus(&focus, cx);
                        input.update(cx, |input, cx| action(input, window, cx));
                    }
                };

                vec![
                    MenuItem::new(
                        "Cut",
                        run(&context_menu_input, |input, window, cx| {
                            input.cut(&Cut, window, cx)
                        }),
                    )
                    .disabled(!has_selection),
                    MenuItem::new(
                        "Copy",
                        run(&context_menu_input, |input, window, cx| {
                            input.copy(&Copy, window, cx)
                        }),
                    )
                    .disabled(!has_selection),
                    MenuItem::new(
                        "Paste",
                        run(&context_menu_input, |input, window, cx| {
                            input.paste(&Paste, window, cx)
                        }),
                    )
                    .disabled(!can_paste),
                    MenuItem::Separator,
                    MenuItem::new(
                        "Select All",
                        run(&context_menu_input, |input, window, cx| {
                            input.select_all(&SelectAll, window, cx)
                        }),
                    )
                    .disabled(!has_content || all_selected),
                ]
            },
        )
    }
}

impl Focusable for ComposerInput {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}
