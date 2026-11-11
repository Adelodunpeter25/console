//! Hover diff popover for the per-turn changed-file chips.
//!
//! Hovering a chip for a moment opens a card above it showing that file's
//! recorded diff. The card stays open while the pointer is on the chip or on
//! the card itself, so a long diff can be scrolled. The diff is the same
//! editor diff view the Diff tab and review tab show (line-number gutter,
//! change bars, syntax highlighting), not a separate renderer. Files with no
//! stored diff that are binary (images, archives, …) show a short "Binary
//! file" note; other files without a diff get no popover.

use std::cell::Cell;
use std::rc::Rc;
use std::time::Duration;

use super::change_chips::{LoadDiffHandler, TurnChange};
use console_core::utils::file_kind::{FileKind, file_kind_for_path};
use gpui::{
    App, Bounds, Div, ElementId, Entity, InteractiveElement, IntoElement, ParentElement, Pixels,
    Stateful, StatefulInteractiveElement, Styled, Window, canvas, deferred, div,
    prelude::FluentBuilder, px,
};

use crate::primitives::menu::{FloatingSurface, MenuAlign};
use crate::theme::Theme;
use crate::viewer::diff_viewer::{
    DIFF_LINE_HEIGHT, DiffViewer, EditorDiff, build_editor_diff_capped,
};

const SHOW_DELAY: Duration = Duration::from_millis(300);
const HIDE_DELAY: Duration = Duration::from_millis(150);
const CARD_WIDTH: f32 = 560.0;
/// The editor diff view fills its parent, so the card sizes it to the diff's
/// line count, up to this height; longer diffs scroll inside.
const CARD_DIFF_MAX_HEIGHT: f32 = 260.0;
const CARD_GAP: f32 = 6.0;
/// A hover preview only builds this many diff lines. Past it the card says
/// how much was left out; the full diff is one click away. Bounds the work of
/// the frame the card first opens on, however large the stored diff is.
const PREVIEW_MAX_LINES: usize = 400;

/// Hover bookkeeping for one chip, kept across frames.
struct PopoverHover {
    chip: bool,
    card: bool,
    open: bool,
    /// Bumped on every hover change so a delayed open/close that has been
    /// superseded does nothing.
    generation: u64,
    /// The chip's bounds as of the last frame, so the card can sit above it.
    bounds: Rc<Cell<Option<Bounds<Pixels>>>>,
    /// The diff view for this chip, built the first time the card opens, and
    /// the diff's true line count. Kept so its scroll position survives
    /// re-renders.
    diff: Option<(EditorDiff, usize)>,
    /// Lazily fetched diff text: `None` until requested, `Some(None)` when
    /// the fetch came back empty or failed.
    fetched: Option<Option<String>>,
    fetching: bool,
}

impl PopoverHover {
    fn new() -> Self {
        Self {
            chip: false,
            card: false,
            open: false,
            generation: 0,
            bounds: Rc::new(Cell::new(None)),
            diff: None,
            fetched: None,
            fetching: false,
        }
    }
}

/// What a card shows.
enum Body {
    Diff,
    Note(String),
    Loading,
}

/// `None` means the chip gets no popover at all.
fn body_for(
    change: &TurnChange,
    fetched: Option<&Option<String>>,
    can_fetch: bool,
) -> Option<Body> {
    let has_text = |t: &str| !t.trim().is_empty();
    if change.diff_text.as_deref().is_some_and(has_text) {
        return Some(Body::Diff);
    }
    match fetched {
        Some(Some(text)) if has_text(text) => return Some(Body::Diff),
        Some(_) => return binary_note(change),
        None => {}
    }
    if let Some(note) = binary_note(change) {
        return Some(note);
    }
    can_fetch.then_some(Body::Loading)
}

fn binary_note(change: &TurnChange) -> Option<Body> {
    match file_kind_for_path(&change.path) {
        FileKind::RasterImage | FileKind::Blocked => {
            let what = match change.status.as_str() {
                "added" => "added",
                "deleted" => "deleted",
                _ => "modified",
            };
            Some(Body::Note(format!("Binary file {what}")))
        }
        _ => None,
    }
}

fn set_hover(state: &Entity<PopoverHover>, chip: Option<bool>, card: Option<bool>, cx: &mut App) {
    let (generation, want_open, is_open) = state.update(cx, |s, _| {
        if let Some(value) = chip {
            s.chip = value;
        }
        if let Some(value) = card {
            s.card = value;
        }
        s.generation += 1;
        (s.generation, s.chip || s.card, s.open)
    });
    if want_open == is_open {
        return;
    }
    let delay = if want_open { SHOW_DELAY } else { HIDE_DELAY };
    let state = state.clone();
    cx.spawn(async move |cx| {
        cx.background_executor().timer(delay).await;
        cx.update(|cx| {
            state.update(cx, |s, cx| {
                let want = s.chip || s.card;
                if s.generation == generation && s.open != want {
                    s.open = want;
                    cx.notify();
                }
            });
        });
    })
    .detach();
}

/// Give a chip its hover diff popover. Returns the chip unchanged when the
/// file has nothing to show.
pub fn with_diff_popover(
    chip: Stateful<Div>,
    key: &str,
    change: &TurnChange,
    load_diff: Option<LoadDiffHandler>,
    theme: &Theme,
    window: &mut Window,
    cx: &mut App,
) -> Stateful<Div> {
    let state = window.use_keyed_state(
        ElementId::Name(format!("turn-change-hover-{key}").into()),
        cx,
        |_, _| PopoverHover::new(),
    );
    let (open, bounds, fetched, fetching) = {
        let s = state.read(cx);
        (s.open, s.bounds.clone(), s.fetched.clone(), s.fetching)
    };
    let Some(body) = body_for(change, fetched.as_ref(), load_diff.is_some()) else {
        return chip;
    };

    if open && matches!(body, Body::Loading) && !fetching {
        if let Some(load_diff) = load_diff {
            state.update(cx, |s, _| s.fetching = true);
            let task = load_diff(&change.path, change.turn_index, cx);
            let state = state.clone();
            cx.spawn(async move |cx| {
                let text = task.await;
                cx.update(|cx| {
                    state.update(cx, |s, cx| {
                        s.fetched = Some(text);
                        s.fetching = false;
                        cx.notify();
                    });
                });
            })
            .detach();
        }
    }

    let chip_state = state.clone();
    let probe_bounds = bounds.clone();
    let chip = chip
        .relative()
        .child(
            canvas(
                move |probe: Bounds<Pixels>, _, _| probe_bounds.set(Some(probe)),
                |_, _, _, _| (),
            )
            .absolute()
            .inset_0(),
        )
        .on_hover(move |hovered, _, cx| set_hover(&chip_state, Some(*hovered), None, cx));

    let Some(anchor) = bounds.get().filter(|_| open) else {
        return chip;
    };

    let card_state = state.clone();
    let card = div()
        .id(ElementId::Name(format!("turn-change-card-{key}").into()))
        .occlude()
        .w(px(CARD_WIDTH))
        .max_w(px(CARD_WIDTH))
        .p(px(4.0))
        .rounded(px(10.0))
        .border_1()
        .border_color(theme.border_strong)
        .bg(theme.raised)
        .shadow_md()
        .on_hover(move |hovered, _, cx| set_hover(&card_state, None, Some(*hovered), cx))
        .child(match body {
            Body::Diff => {
                let existing = state.read(cx).diff.clone();
                let (diff, total_lines) = match existing {
                    Some(built) => built,
                    None => {
                        let text = change
                            .diff_text
                            .clone()
                            .filter(|t| !t.trim().is_empty())
                            .or_else(|| state.read(cx).fetched.clone().flatten())
                            .unwrap_or_default();
                        let built = build_editor_diff_capped(
                            &text,
                            &change.path,
                            theme,
                            PREVIEW_MAX_LINES,
                            cx,
                        );
                        state.update(cx, |s, _| s.diff = Some(built.clone()));
                        built
                    }
                };
                // Exact height for short diffs (plus a little breathing
                // room), capped so long ones scroll inside the card.
                let height = (diff.line_count as f32 * DIFF_LINE_HEIGHT + 8.0)
                    .min(CARD_DIFF_MAX_HEIGHT);
                let left_out = total_lines.saturating_sub(diff.line_count);
                div()
                    .flex()
                    .flex_col()
                    .child(
                        div()
                            .w_full()
                            .h(px(height))
                            .rounded(px(6.0))
                            .overflow_hidden()
                            .child(DiffViewer::new(diff.view)),
                    )
                    .when(left_out > 0, |element| {
                        element.child(
                            div()
                                .px(px(8.0))
                                .pt(px(5.0))
                                .pb(px(2.0))
                                .text_size(px(11.0))
                                .text_color(theme.text_tertiary)
                                .child(format!(
                                    "Showing the first {} of {} lines — click the file to open the full diff",
                                    diff.line_count, total_lines
                                )),
                        )
                    })
                    .into_any_element()
            }
            Body::Loading => div()
                .px(px(8.0))
                .py(px(6.0))
                .text_size(px(12.0))
                .text_color(theme.text_tertiary)
                .child("Loading diff…")
                .into_any_element(),
            Body::Note(note) => div()
                .px(px(8.0))
                .py(px(6.0))
                .text_size(px(12.0))
                .text_color(theme.text_tertiary)
                .child(note)
                .into_any_element(),
        });

    chip.child(
        deferred(FloatingSurface::new(
            card.into_any_element(),
            anchor,
            MenuAlign::AboveLeft,
            px(CARD_GAP),
            px(8.0),
        ))
        // Above embedded browser surfaces, like menus.
        .with_priority(100),
    )
}
