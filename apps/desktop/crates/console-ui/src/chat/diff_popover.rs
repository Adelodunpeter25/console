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

use console_core::types::SessionFileChange;
use console_core::utils::diff::parse_unified_diff;
use console_core::utils::file_kind::{FileKind, file_kind_for_path};
use editor_ui::{DiffLine, DiffLineKind, DiffResult, DiffState, FontConfig};
use gpui::{
    App, AppContext, Bounds, Div, ElementId, Entity, InteractiveElement, IntoElement, ParentElement,
    Pixels, Stateful, StatefulInteractiveElement, Styled, Window, canvas, deferred, div, px,
};

use crate::primitives::menu::{FloatingSurface, MenuAlign};
use crate::theme::Theme;
use crate::viewer::DiffViewer;

const SHOW_DELAY: Duration = Duration::from_millis(300);
const HIDE_DELAY: Duration = Duration::from_millis(150);
const CARD_WIDTH: f32 = 560.0;
/// The editor diff view fills its parent, so the card gives it a fixed
/// height: tall enough for a useful hunk, scrolling inside beyond that.
const CARD_DIFF_HEIGHT: f32 = 260.0;
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
    /// The editor diff view for this chip, built the first time the card
    /// opens. Kept so its scroll position survives re-renders.
    view: Option<Entity<editor_ui::DiffView>>,
}

impl PopoverHover {
    fn new() -> Self {
        Self {
            chip: false,
            card: false,
            open: false,
            generation: 0,
            bounds: Rc::new(Cell::new(None)),
            view: None,
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

/// Convert the parsed unified diff into the editor's diff model.
fn editor_result(raw: &str) -> DiffResult {
    let parsed = parse_unified_diff(raw);
    let mut added = 0;
    let mut removed = 0;
    let lines = parsed
        .lines
        .iter()
        .map(|line| {
            let kind = match line.kind {
                console_core::DiffLineKind::Added => {
                    added += 1;
                    DiffLineKind::Added
                }
                console_core::DiffLineKind::Removed => {
                    removed += 1;
                    DiffLineKind::Removed
                }
                console_core::DiffLineKind::Context => DiffLineKind::Context,
            };
            DiffLine {
                kind,
                text: line.text.clone(),
                old_line: line.old_no.map(|n| n as u32),
                new_line: line.new_no.map(|n| n as u32),
            }
        })
        .collect();
    DiffResult {
        lines,
        added,
        removed,
    }
}

/// Build the diff view with the same settings the Diff tab uses: the app's
/// editor font, the theme's syntax preset, the file's language, no wrapping.
fn build_diff_view(
    raw: &str,
    path: &str,
    theme: &Theme,
    cx: &mut App,
) -> Entity<editor_ui::DiffView> {
    let language = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
    let preset = if theme.is_dark {
        syntax::ThemePreset::GitHubDark
    } else {
        syntax::ThemePreset::GitHubLight
    };
    let font = FontConfig {
        family: "JetBrains Mono".into(),
        size: px(12.0),
        line_height: px(20.0),
    };
    let state = cx.new(|_| {
        let mut state = DiffState::from_result(editor_result(raw), language);
        state.set_theme(preset);
        state.set_wrap_enabled(false);
        state.set_font(font);
        state
    });
    cx.new(|_| editor_ui::DiffView::new(&state))
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
    let (open, bounds) = {
        let s = state.read(cx);
        (s.open, s.bounds.clone())
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
        .p(px(4.0))
        .rounded(px(10.0))
        .border_1()
        .border_color(theme.border_strong)
        .bg(theme.raised)
        .shadow_md()
        .on_hover(move |hovered, _, cx| set_hover(&card_state, None, Some(*hovered), cx))
        .child(match body {
            Body::Diff(text) => {
                let existing = state.read(cx).view.clone();
                let view = match existing {
                    Some(view) => view,
                    None => {
                        let view = build_diff_view(&text, &change.path, theme, cx);
                        state.update(cx, |s, _| s.view = Some(view.clone()));
                        view
                    }
                };
                div()
                    .w_full()
                    .h(px(CARD_DIFF_HEIGHT))
                    .rounded(px(6.0))
                    .overflow_hidden()
                    .child(DiffViewer::new(view))
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
