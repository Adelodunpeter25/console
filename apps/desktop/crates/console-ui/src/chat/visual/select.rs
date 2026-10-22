//! Transcript-selectable text for the visual card. Every text element takes
//! the next key in paint order under one row id (the tool call id), so drag
//! selection and copy work across the card and the surrounding messages.
//! Colors, sizes, and weights come from the parent element's styling; this
//! only supplies the text itself. Chart tick numbers are painted on a canvas
//! and stay unselectable.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{AnyElement, FontWeight, Hsla};

use crate::markdown::render::{
    Palette, SANS_FAMILY, TranscriptSelection, flatten_plain, selectable_flat_text,
};
use crate::markdown::selection::TextKey;
use crate::theme::Theme;

#[derive(Clone)]
pub(super) struct Selectable {
    row: Rc<str>,
    next: Rc<Cell<usize>>,
    selection: TranscriptSelection,
    code_wash: Hsla,
    selection_wash: Hsla,
}

impl Selectable {
    pub(super) fn new(row: String, selection: TranscriptSelection, theme: &Theme) -> Self {
        let palette = Palette::from_theme(theme);
        Self {
            row: row.into(),
            next: Rc::new(Cell::new(0)),
            selection,
            code_wash: palette.code_wash,
            selection_wash: palette.selection,
        }
    }

    /// A selectable run of text. `block_break` adds a paragraph break when
    /// copying across it; table cells pass `false` so they join with
    /// single newlines.
    pub(super) fn text(
        &self,
        text: impl Into<gpui::SharedString>,
        weight: FontWeight,
        color: Hsla,
        block_break: bool,
    ) -> AnyElement {
        let index = self.next.get();
        self.next.set(index + 1);
        let flat = flatten_plain(text, SANS_FAMILY, weight, color);
        selectable_flat_text(
            &flat,
            TextKey::new(self.row.clone(), index),
            self.selection.clone(),
            self.code_wash,
            self.selection_wash,
            block_break,
        )
    }
}
