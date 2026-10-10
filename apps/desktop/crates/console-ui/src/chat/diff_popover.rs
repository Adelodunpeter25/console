//! Hover diff popover for the per-turn changed-file chips.
//!
//! Hovering a chip for a moment opens a card above it showing that file's
//! recorded diff. The card stays open while the pointer is on the chip or on
//! the card itself, so a long diff can be scrolled. Files with no stored diff
//! that are binary (images, archives, …) show a short "Binary file" note;
//! other files without a diff get no popover.

use std::cell::Cell;
use std::rc::Rc;
use std::time::Duration;

use console_core::types::SessionFileChange;
use console_core::utils::diff::{DiffResult, parse_unified_diff};
use console_core::utils::file_kind::{FileKind, file_kind_for_path};
use gpui::{
    App, Bounds, Div, ElementId, Entity, InteractiveElement, IntoElement, ParentElement, Pixels,
    ScrollHandle, Stateful, StatefulInteractiveElement, Styled, TextRun, Window, canvas, deferred,
    div, px,
};

use crate::chat::DiffView;
use crate::chat::diff_view::highlight_diff;
use crate::primitives::menu::{FloatingSurface, MenuAlign};
use crate::theme::Theme;

const SHOW_DELAY: Duration = Duration::from_millis(300);
const HIDE_DELAY: Duration = Duration::from_millis(150);
const CARD_WIDTH: f32 = 520.0;
const CARD_GAP: f32 = 6.0;

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
    scroll: ScrollHandle,
    /// Parsed and syntax-highlighted diff, built the first time the card
    /// opens so the per-frame render only clones it.
    prepared: Option<Prepared>,
}

#[derive(Clone)]
struct Prepared {
    diff: DiffResult,
    highlights: Option<Rc<Vec<Vec<TextRun>>>>,
}

impl PopoverHover {
    fn new() -> Self {
        Self {
            chip: false,
            card: false,
            open: false,
            generation: 0,
            bounds: Rc::new(Cell::new(None)),
            scroll: ScrollHandle::new(),
            prepared: None,
        }
    }
}

enum Body {
    Diff(String),
    Note(String),
}

fn body_for(change: &SessionFileChange) -> Option<Body> {
    if let Some(text) = change.diff_text.as_deref().filter(|t| !t.trim().is_empty()) {
        return Some(Body::Diff(text.to_string()));
    }
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

fn set_hover(
    state: &Entity<PopoverHover>,
    chip: Option<bool>,
    card: Option<bool>,
    cx: &mut App,
) {
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
    change: &SessionFileChange,
    theme: &Theme,
    window: &mut Window,
    cx: &mut App,
) -> Stateful<Div> {
    let Some(body) = body_for(change) else {
        return chip;
    };
    let state = window.use_keyed_state(
        ElementId::Name(format!("turn-change-hover-{key}").into()),
        cx,
        |_, _| PopoverHover::new(),
    );
    let (open, bounds, scroll) = {
        let s = state.read(cx);
        (s.open, s.bounds.clone(), s.scroll.clone())
    };

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
        .p(px(6.0))
        .rounded(px(10.0))
        .border_1()
        .border_color(theme.border_strong)
        .bg(theme.raised)
        .shadow_md()
        .on_hover(move |hovered, _, cx| set_hover(&card_state, None, Some(*hovered), cx))
        .child(match body {
            Body::Diff(text) => {
                let prepared = state.update(cx, |s, _| {
                    s.prepared
                        .get_or_insert_with(|| {
                            let diff = parse_unified_diff(&text);
                            let highlights = highlight_diff(&diff, &change.path, theme);
                            Prepared { diff, highlights }
                        })
                        .clone()
                });
                DiffView::new(format!("popover-{key}"), prepared.diff)
                    .hide_header(true)
                    .scroll_handle(scroll)
                    .highlights(prepared.highlights)
                    .into_any_element()
            }
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
