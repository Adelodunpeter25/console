//! Composer autocomplete for slash commands and `@` file references.
//!
//! This module owns only trigger detection, filtering, keyboard bindings, and
//! popup presentation. The desktop app owns the backend requests and keeps the
//! results scoped to each workspace pane/session.

use console_core::{FileSearchResult, SlashCommandInfo};
use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    Anchor, App, Bounds, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    Pixels, RenderOnce, StatefulInteractiveElement, Styled, Window, actions, anchored, canvas,
    deferred, div, point, prelude::FluentBuilder, px,
};

use crate::primitives::{IconName, app_icon};
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
                format!("@{} ", path)
            }
        }
    }
}

/// Detect the trigger immediately before the composer's caret.
///
/// Slash commands are recognized only at the beginning of a line. File
/// references must begin at whitespace or the beginning of the prompt, which
/// prevents email addresses and ordinary prose from opening the popup.
pub fn detect_trigger(value: &str, cursor: usize) -> Option<AutocompleteTrigger> {
    let cursor = cursor.min(value.len());
    if !value.is_char_boundary(cursor) {
        return None;
    }
    let before = &value[..cursor];
    let line_start = before.rfind('\n').map_or(0, |index| index + 1);

    // The caret may briefly report the position before the inserted trigger
    // while the input element is reconciling. Never construct `trigger + 1..`
    // until the caret is known to be after the trigger byte.
    if cursor > line_start && value.as_bytes().get(line_start) == Some(&b'/') {
        let query_start = line_start + 1;
        if query_start <= cursor && value.is_char_boundary(query_start) {
            let query = &value[query_start..cursor];
            if query.chars().all(valid_command_character) {
                return Some(AutocompleteTrigger {
                    kind: AutocompleteKind::Command,
                    range: line_start..cursor,
                    query: query.to_owned(),
                });
            }
        }
    }

    let at_start = before.rfind('@')?;
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
#[derive(IntoElement)]
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
                .min_h(px(30.0))
                .px(px(8.0))
                .py(px(4.0))
                .rounded(px(6.0))
                .flex()
                .items_center()
                .gap(px(7.0))
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
                    .child(app_icon(IconName::Sparkle, 13.0, theme.accent))
                    .child(
                        div()
                            .min_w(px(0.0))
                            .flex_1()
                            .truncate()
                            .child(
                                div()
                                    .text_size(px(12.0))
                                    .font_weight(FontWeight::MEDIUM)
                                    .text_color(theme.text)
                                    .child(format!("/{}", command.name)),
                            )
                            .child(
                                div()
                                    .truncate()
                                    .text_size(px(10.5))
                                    .text_color(theme.text_tertiary)
                                    .child(command.description),
                            ),
                    )
                    .into_any_element(),
                AutocompleteItem::File(file) => row
                    .child(app_icon(
                        if file.is_dir {
                            IconName::Folder
                        } else {
                            IconName::File
                        },
                        13.0,
                        theme.text_tertiary,
                    ))
                    .child(
                        div()
                            .min_w(px(0.0))
                            .flex_1()
                            .truncate()
                            .text_size(px(12.0))
                            .text_color(theme.text)
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_line_start_slash_commands() {
        let trigger = detect_trigger("build\n/plan", 11).expect("slash trigger");
        assert_eq!(trigger.kind, AutocompleteKind::Command);
        assert_eq!(trigger.range, 6..11);
        assert_eq!(trigger.query, "plan");
    }

    #[test]
    fn detects_whitespace_delimited_file_references() {
        let trigger = detect_trigger("read @src/main", 14).expect("file trigger");
        assert_eq!(trigger.kind, AutocompleteKind::File);
        assert_eq!(trigger.query, "src/main");
    }

    #[test]
    fn does_not_treat_email_addresses_as_file_references() {
        assert!(detect_trigger("mail me@example", 15).is_none());
    }

    #[test]
    fn does_not_panic_when_caret_precedes_a_new_slash_trigger() {
        assert!(detect_trigger("/", 0).is_none());
    }

    #[test]
    fn does_not_panic_when_caret_precedes_a_new_file_trigger() {
        assert!(detect_trigger("@src/main", 0).is_none());
    }
}
