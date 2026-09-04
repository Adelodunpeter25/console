//! Shared markdown helpers for chat bubbles (A+B consolidation).
//!
//! One `render_selectable_markdown` plus `Ctx` constructors so bubbles,
//! thinking blocks, run activity, and subagent summaries share the same
//! borrowing + fallback + link-handler wiring instead of drifting copies.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{AnyElement, IntoElement, div, prelude::*};

use crate::markdown::render::{
    self as markdown, Ctx as MarkdownCtx, LinkHandler, MarkdownView, Metrics, Palette,
    TranscriptSelection,
};

pub fn render_selectable_markdown(
    text: &str,
    view: Option<&Rc<RefCell<MarkdownView>>>,
    ctx: &MarkdownCtx<'_>,
    mend: bool,
) -> AnyElement {
    let Some(view) = view else {
        return div().child(text.to_owned()).into_any_element();
    };
    let mut view = view.borrow_mut();
    view.set_text(text, mend);
    markdown::markdown(&view, ctx)
        .unwrap_or_else(|| div().child(text.to_owned()).into_any_element())
}

fn with_optional_link(ctx: MarkdownCtx<'_>, handler: Option<LinkHandler>) -> MarkdownCtx<'_> {
    match handler {
        Some(handler) => ctx.with_link_handler(handler),
        None => ctx,
    }
}

/// Assistant body text with optional file-link handling.
pub fn assistant_ctx<'a>(
    row: String,
    palette: &'a Palette,
    selection: TranscriptSelection,
    link_handler: Option<LinkHandler>,
) -> MarkdownCtx<'a> {
    with_optional_link(
        MarkdownCtx::new(row, palette, Metrics::BODY, selection),
        link_handler,
    )
}

/// Compact text (tool results, subagent summaries) with optional links.
pub fn compact_ctx<'a>(
    row: String,
    palette: &'a Palette,
    selection: TranscriptSelection,
    link_handler: Option<LinkHandler>,
) -> MarkdownCtx<'a> {
    with_optional_link(
        MarkdownCtx::new(row, palette, Metrics::COMPACT, selection),
        link_handler,
    )
}
