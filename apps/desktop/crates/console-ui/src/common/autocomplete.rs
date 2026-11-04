//! Composer autocomplete for slash commands and `@` file references.
//!
//! This module owns only trigger detection, filtering, keyboard bindings, and
//! popup presentation. The desktop app owns the backend requests and keeps the
//! results scoped to each workspace pane/session.

use console_core::{FileSearchResult, SessionHeader, SlashCommandInfo};
use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    Anchor, App, Bounds, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    Pixels, RenderOnce, StatefulInteractiveElement, Styled, Window, actions, anchored, canvas,
    deferred, div, point, prelude::FluentBuilder, px,
};

use crate::primitives::{IconName, app_icon, file_type_icon};
use crate::theme::Theme;

actions!(
    console_autocomplete,
    [
        AutocompleteNext,
        AutocompletePrevious,
        AutocompleteConfirm,
        AutocompleteDismiss
    ]
);

/// Parent key context declared by the composer while autocomplete is visible.
pub const AUTOCOMPLETE_CONTEXT: &str = "ComposerAutocomplete";
const AUTOCOMPLETE_FIELD_CONTEXT: &str = "ComposerAutocomplete > ComposerInput";

/// Bind autocomplete keys after the generic ComposerInput bindings so these
/// actions win while a suggestion list is visible.
pub fn init(cx: &mut App) {
    use gpui::KeyBinding;
    cx.bind_keys([
        KeyBinding::new("down", AutocompleteNext, Some(AUTOCOMPLETE_FIELD_CONTEXT)),
        KeyBinding::new("up", AutocompletePrevious, Some(AUTOCOMPLETE_FIELD_CONTEXT)),
        KeyBinding::new(
            "enter",
            AutocompleteConfirm,
            Some(AUTOCOMPLETE_FIELD_CONTEXT),
        ),
        KeyBinding::new("tab", AutocompleteConfirm, Some(AUTOCOMPLETE_FIELD_CONTEXT)),
        KeyBinding::new(
            "escape",
            AutocompleteDismiss,
            Some(AUTOCOMPLETE_FIELD_CONTEXT),
        ),
    ]);
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AutocompleteKind {
    Command,
    File,
    Chat,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AutocompleteTrigger {
    pub kind: AutocompleteKind,
    /// Byte range covering the trigger and query, such as `/plan` or `@src/`.
    pub range: std::ops::Range<usize>,
    /// Text after `/` or `@`.
    pub query: String,
}

#[derive(Clone, Debug)]
pub enum AutocompleteItem {
    Command(SlashCommandInfo),
    File(FileSearchResult),
    Chat(SessionHeader),
}

/// Comparable identity of an [`AutocompleteView`]'s content. See
/// [`AutocompleteView::content_key`].
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub struct AutocompleteContentKey {
    pub count: usize,
    pub highlighted: usize,
    pub loading: bool,
    pub ids: u64,
}

impl AutocompleteItem {
    pub fn insert_text(&self) -> String {
        match self {
            Self::Command(command) => format!("/{} ", command.name),
            Self::File(file) => {
                // Backend sometimes returns absolute_path only; fall back so click never inserts "@ ".
                let path = if file.relative_path.is_empty() {
                    file.absolute_path
                        .rsplit('/')
                        .next()
                        .unwrap_or(&file.absolute_path)
                } else {
                    &file.relative_path
                };
                format!("{} ", path)
            }
            Self::Chat(session) => {
                let slug = slugify_chat_title(&session.title);
                let slug = if slug.is_empty() {
                    session.id.as_str()
                } else {
                    slug.as_str()
                };
                format!("${slug} ")
            }
        }
    }
}

/// Detect the trigger immediately before the composer's caret.
///
/// Slash commands must begin at whitespace or the beginning of the prompt.
/// File references use the same boundary rule, which prevents email addresses,
/// URLs, and ordinary prose from opening the popup.
pub fn detect_trigger(value: &str, cursor: usize) -> Option<AutocompleteTrigger> {
    let cursor = cursor.min(value.len());
    if !value.is_char_boundary(cursor) {
        return None;
    }
    let before = &value[..cursor];
    // Find the trigger nearest to the caret. A slash command can appear after
    // whitespace anywhere in the prompt, not only at the beginning of a line.
    // Searching backwards also makes multiple commands in one prompt behave
    // naturally: only the command currently being typed is replaced.
    if let Some(slash) = before.char_indices().rev().find_map(|(index, character)| {
        (character == '/'
            && (index == 0
                || before[..index]
                    .chars()
                    .next_back()
                    .is_some_and(char::is_whitespace)))
        .then_some(index)
    }) {
        let query_start = slash + 1;
        if value.is_char_boundary(query_start) {
            let query = &value[query_start..cursor];
            if query.chars().all(valid_command_character) {
                return Some(AutocompleteTrigger {
                    kind: AutocompleteKind::Command,
                    range: slash..cursor,
                    query: query.to_owned(),
                });
            }
        }
    }

    // `$` chat references: same boundary rule as `/` (start, whitespace, or
    // opening punctuation), slug/id characters only. Digit-only queries are
    // prices, not refs — the server only treats a digit-led token as a ref
    // when it looks like a session id.
    if let Some(dollar) = before
        .char_indices()
        .rev()
        .find_map(|(index, character)| (character == '$').then_some(index))
    {
        let previous = before[..dollar].chars().next_back();
        let query = &value[dollar + 1..cursor];
        let boundary = previous.is_none()
            || previous.is_some_and(|c| c.is_whitespace() || "([{<\"'".contains(c));
        if boundary
            && query.chars().all(valid_ref_character)
            && !(query.chars().all(|c| c.is_ascii_digit()) && !query.is_empty())
        {
            return Some(AutocompleteTrigger {
                kind: AutocompleteKind::Chat,
                range: dollar..cursor,
                query: query.to_owned(),
            });
        }
    }

    let at_start = before
        .char_indices()
        .rev()
        .find_map(|(index, character)| (character == '@').then_some(index))?;
    let previous = before[..at_start].chars().next_back();
    let query = &value[at_start + 1..cursor];
    if (previous.is_none() || previous.is_some_and(char::is_whitespace))
        && query.chars().all(valid_file_character)
    {
        return Some(AutocompleteTrigger {
            kind: AutocompleteKind::File,
            range: at_start..cursor,
            query: query.to_owned(),
        });
    }

    None
}

fn valid_command_character(character: char) -> bool {
    character.is_ascii_alphanumeric() || matches!(character, '_' | '-' | ':')
}

fn valid_ref_character(character: char) -> bool {
    character.is_ascii_alphanumeric() || matches!(character, '_' | '-')
}

/// Chat-title slug, mirroring the server's SlugifyTitle: lowercase, every
/// run of non-letters/digits collapsed to one '-', trimmed. A `$` ref typed
/// from this slug resolves server-side against the chat's title.
pub fn slugify_chat_title(title: &str) -> String {
    let mut out = String::new();
    let mut last_dash = true;
    for character in title.to_lowercase().chars() {
        if character.is_alphanumeric() {
            out.push(character);
            last_dash = false;
        } else if !last_dash {
            out.push('-');
            last_dash = true;
        }
    }
    out.trim_matches('-').to_owned()
}

fn valid_file_character(character: char) -> bool {
    character.is_ascii_alphanumeric() || matches!(character, '_' | '-' | '.' | '/')
}

const MAX_AUTOCOMPLETE_ITEMS: usize = 80;

/// Filter the backend results for the active trigger. File results are already
/// ranked by the backend search endpoint; this local filter only removes stale
/// results when the query changes faster than the response can arrive.
///
/// Results are capped to `MAX_AUTOCOMPLETE_ITEMS` and short-circuit after the
/// cap is hit so an empty `@` query over a 5k-file repo doesn't build/scan
/// thousands of rows or allocate per-row lowercased strings beyond what will
/// ever be painted. The popup is a flex `overflow_y_scroll` — not virtualized —
/// so element count directly drives layout/paint cost and scroll jank.
pub fn filter_items(
    commands: &[SlashCommandInfo],
    files: &[FileSearchResult],
    chats: &[SessionHeader],
    trigger: &AutocompleteTrigger,
) -> Vec<AutocompleteItem> {
    let query = trigger.query.to_ascii_lowercase();
    match trigger.kind {
        AutocompleteKind::Command => {
            if query.is_empty() {
                return commands
                    .iter()
                    .take(MAX_AUTOCOMPLETE_ITEMS)
                    .cloned()
                    .map(AutocompleteItem::Command)
                    .collect();
            }
            let mut out = Vec::new();
            out.reserve(MAX_AUTOCOMPLETE_ITEMS.min(commands.len()));
            for command in commands {
                if command.name.to_ascii_lowercase().starts_with(&query)
                    || command.description.to_ascii_lowercase().contains(&query)
                {
                    out.push(AutocompleteItem::Command(command.clone()));
                    if out.len() >= MAX_AUTOCOMPLETE_ITEMS {
                        break;
                    }
                }
            }
            out
        }
        AutocompleteKind::Chat => {
            let mut scored: Vec<(u8, &SessionHeader)> = Vec::new();
            for session in chats {
                let slug = slugify_chat_title(&session.title);
                let title = session.title.to_lowercase();
                let id = session.id.to_lowercase();
                let score = if query.is_empty() {
                    Some(0)
                } else if slug.starts_with(&query) {
                    Some(3)
                } else if id.starts_with(&query) {
                    Some(2)
                } else if title.starts_with(&query) {
                    Some(1)
                } else if slug.contains(&query) || title.contains(&query) {
                    Some(0)
                } else {
                    None
                };
                if let Some(score) = score {
                    scored.push((score, session));
                    if scored.len() >= MAX_AUTOCOMPLETE_ITEMS {
                        break;
                    }
                }
            }
            scored.sort_by(|a, b| b.0.cmp(&a.0).then_with(|| a.1.title.cmp(&b.1.title)));
            scored
                .into_iter()
                .map(|(_, session)| AutocompleteItem::Chat(session.clone()))
                .collect()
        }
        AutocompleteKind::File => {
            if query.is_empty() {
                return files
                    .iter()
                    .take(MAX_AUTOCOMPLETE_ITEMS)
                    .cloned()
                    .map(AutocompleteItem::File)
                    .collect();
            }
            let mut out = Vec::new();
            out.reserve(MAX_AUTOCOMPLETE_ITEMS.min(files.len()));
            for file in files {
                if file.relative_path.to_ascii_lowercase().contains(&query) {
                    out.push(AutocompleteItem::File(file.clone()));
                    if out.len() >= MAX_AUTOCOMPLETE_ITEMS {
                        break;
                    }
                }
            }
            out
        }
    }
}

/// The popup view. Selection state and backend fetching remain in the desktop
/// app so the component can be reused by every pane without hidden session
/// coupling.
/// Clone so a surface that owns the popup across frames (the floating
/// composer) can keep it in its state and hand a copy to the view.
#[derive(IntoElement, Clone)]
pub struct AutocompleteView {
    items: Vec<AutocompleteItem>,
    highlighted: usize,
    loading: bool,
    anchor_bounds: Rc<Cell<Option<Bounds<Pixels>>>>,
    on_select: Rc<dyn Fn(AutocompleteItem, &mut Window, &mut App) + 'static>,
}

impl AutocompleteView {
    pub fn new(items: Vec<AutocompleteItem>, highlighted: usize, loading: bool) -> Self {
        Self {
            items,
            highlighted,
            loading,
            anchor_bounds: Rc::new(Cell::new(None)),
            on_select: Rc::new(|_, _, _| {}),
        }
    }

    pub fn anchor_cell(&self) -> Rc<Cell<Option<Bounds<Pixels>>>> {
        self.anchor_bounds.clone()
    }

    /// A cheap comparable identity for the popup's *content*.
    ///
    /// The popup is rebuilt every frame, so an owner that stores it and
    /// notifies on every write would re-render forever. Comparing this instead
    /// lets the owner skip an unchanged popup. The id hash is included because
    /// a new query routinely returns the same number of rows with different
    /// paths.
    pub fn content_key(&self) -> AutocompleteContentKey {
        let mut ids: u64 = 0;
        for item in &self.items {
            let id = match item {
                AutocompleteItem::Command(command) => &command.name,
                AutocompleteItem::File(file) => {
                    if file.relative_path.is_empty() {
                        &file.absolute_path
                    } else {
                        &file.relative_path
                    }
                }
                AutocompleteItem::Chat(session) => &session.id,
            };
            for byte in id.as_bytes() {
                ids = ids.wrapping_mul(31).wrapping_add(*byte as u64);
            }
        }
        AutocompleteContentKey {
            count: self.items.len(),
            highlighted: self.highlighted,
            loading: self.loading,
            ids,
        }
    }

    pub fn with_anchor_cell(mut self, anchor_bounds: Rc<Cell<Option<Bounds<Pixels>>>>) -> Self {
        self.anchor_bounds = anchor_bounds;
        self
    }

    /// A zero-cost probe that records the composer card's border-box bounds.
    /// The popup uses the previous frame's bounds to anchor above the card.
    pub fn bounds_probe(cell: Rc<Cell<Option<Bounds<Pixels>>>>) -> impl IntoElement {
        canvas(
            move |bounds: Bounds<Pixels>, _, _| cell.set(Some(bounds)),
            |_, _, _, _| (),
        )
        .absolute()
        .inset_0()
    }

    pub fn on_select(
        mut self,
        handler: impl Fn(AutocompleteItem, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_select = std::rc::Rc::new(handler);
        self
    }
}

impl RenderOnce for AutocompleteView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let Some(anchor_bounds) = self.anchor_bounds.get() else {
            return div().into_any_element();
        };
        let on_select = self.on_select;
        let highlighted = self.highlighted;

        let mut popup = div()
            .id("composer-autocomplete")
            // Occlude the elements underneath (transcript, composer card):
            // without this, a mousedown on the popup also hits the underlying
            // focus-tracking transcript, which steals focus from the composer.
            // The app unmounts the popup when the composer blurs, so the
            // mouse-up lands nowhere and the row's on_click never fires —
            // clicks "just closed" the popup instead of selecting the item.
            .occlude()
            .w(anchor_bounds.size.width)
            .max_h(px(220.0))
            .rounded(px(10.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.raised)
            .shadow_xl()
            .overflow_y_scroll()
            .on_scroll_wheel(|_, _, cx| cx.stop_propagation())
            .p(px(4.0))
            .flex()
            .flex_col()
            .gap(px(2.0));

        if self.loading && self.items.is_empty() {
            popup = popup.child(
                div()
                    .h(px(30.0))
                    .px(px(8.0))
                    .flex()
                    .items_center()
                    .text_size(px(11.5))
                    .text_color(theme.text_tertiary)
                    .child("Searching…"),
            );
        }

        for (index, item) in self.items.into_iter().enumerate() {
            let selected = index == highlighted;
            let on_select = on_select.clone();
            let click_item = item.clone();
            let row = div()
                .id(ElementId::Name(
                    format!("composer-autocomplete-{index}").into(),
                ))
                .min_h(px(28.0))
                .px(px(8.0))
                .py(px(4.0))
                .rounded(px(6.0))
                .flex()
                .items_center()
                .gap(px(8.0))
                .cursor_default()
                .when(selected, |element| element.bg(theme.overlay_strong))
                .when(!selected, |element| {
                    element.hover(|style| style.bg(theme.overlay))
                })
                .on_click(move |_, window, cx| {
                    cx.stop_propagation();
                    (on_select)(click_item.clone(), window, cx);
                });

            popup = popup.child(match item {
                AutocompleteItem::Command(command) => row
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::NORMAL)
                            .text_color(theme.text_tertiary)
                            .flex_none()
                            .child("/"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(if selected {
                                theme.text
                            } else {
                                theme.text_secondary
                            })
                            .flex_none()
                            .child(command.name),
                    )
                    .when(!command.description.is_empty(), |el| {
                        el.child(
                            div()
                                .min_w(px(0.0))
                                .flex_1()
                                .truncate()
                                .text_size(px(12.0))
                                .text_color(theme.text_tertiary)
                                .child(command.description),
                        )
                    })
                    .into_any_element(),
                AutocompleteItem::Chat(session) => {
                    let slug = slugify_chat_title(&session.title);
                    let slug = if slug.is_empty() {
                        session.id.clone()
                    } else {
                        slug
                    };
                    // The title is what the user recognises; the slug it
                    // inserts is secondary.
                    row.child(
                        app_icon(IconName::ChatRoundLine, 13.0, theme.text_tertiary)
                            .into_any_element(),
                    )
                    .child(
                        div()
                            .min_w(px(0.0))
                            .flex_1()
                            .truncate()
                            .text_size(px(12.5))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(if selected {
                                theme.text
                            } else {
                                theme.text_secondary
                            })
                            .child(session.title.clone()),
                    )
                    .child(
                        div()
                            .text_size(px(12.0))
                            .text_color(theme.text_tertiary)
                            .flex_none()
                            .child(format!("${slug}")),
                    )
                    .into_any_element()
                }
                AutocompleteItem::File(file) => row
                    .child(if file.is_dir {
                        app_icon(IconName::Folder, 13.0, theme.text_tertiary).into_any_element()
                    } else {
                        file_type_icon(&file.relative_path, 13.0).into_any_element()
                    })
                    .child(
                        div()
                            .min_w(px(0.0))
                            .flex_1()
                            .truncate()
                            .text_size(px(12.5))
                            .text_color(if selected {
                                theme.text
                            } else {
                                theme.text_secondary
                            })
                            .child(file.relative_path),
                    )
                    .into_any_element(),
            });
        }

        deferred(
            anchored()
                .position(point(
                    anchor_bounds.origin.x,
                    anchor_bounds.origin.y - px(6.0),
                ))
                .anchor(Anchor::BottomLeft)
                .snap_to_window_with_margin(px(8.0))
                .child(popup),
        )
        .with_priority(1)
        .into_any_element()
    }
}
