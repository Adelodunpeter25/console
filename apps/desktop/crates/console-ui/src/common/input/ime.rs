use std::ops::Range;

use gpui::{point, Bounds, Context, EntityInputHandler, Pixels, Point, UTF16Selection, Window};

use super::{ComposerEvent, ComposerInput};

impl EntityInputHandler for ComposerInput {
    fn text_for_range(
        &mut self,
        range_utf16: Range<usize>,
        actual_range: &mut Option<Range<usize>>,
        _: &mut Window,
        _: &mut Context<Self>,
    ) -> Option<String> {
        let range = self.range_from_utf16(&range_utf16);
        actual_range.replace(self.range_to_utf16(&range));
        Some(self.content[range].to_string())
    }

    fn selected_text_range(
        &mut self,
        _: bool,
        _: &mut Window,
        _: &mut Context<Self>,
    ) -> Option<UTF16Selection> {
        Some(UTF16Selection {
            range: self.range_to_utf16(&self.selected_range),
            reversed: self.selection_reversed,
        })
    }

    fn marked_text_range(&self, _: &mut Window, _: &mut Context<Self>) -> Option<Range<usize>> {
        self.marked_range
            .as_ref()
            .map(|range| self.range_to_utf16(range))
    }

    fn unmark_text(&mut self, _: &mut Window, _: &mut Context<Self>) {
        self.marked_range = None;
        self.history.finalize_composition();
    }

    fn replace_text_in_range(
        &mut self,
        range_utf16: Option<Range<usize>>,
        new_text: &str,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.read_only {
            return;
        }
        // While text is marked, macOS reports replacement ranges relative to
        // the marked text, so the marked span itself is the commit target —
        // Zed's reading of the protocol. Absolute ranges only arrive outside
        // composition (e.g. the Accessibility Keyboard's completions).
        self.prompt_history.reset_navigation();
        let composing = self.marked_range.is_some();
        let range = self.marked_range.clone().unwrap_or_else(|| {
            range_utf16
                .as_ref()
                .map(|range| self.range_from_utf16(range))
                .unwrap_or(self.selected_range.clone())
        });
        self.record_edit_history(&range, new_text, composing);
        let previous = self.content.clone();
        self.content =
            (self.content[..range.start].to_owned() + new_text + &self.content[range.end..]).into();
        let offset = range.start + new_text.len();
        self.selected_range = offset..offset;
        self.marked_range = None;
        self.vertical_navigation = None;
        if composing {
            self.history.finalize_composition();
        }
        self.refresh_highlight();
        self.pause_blink_cursor(cx);
        if previous != self.content {
            cx.emit(ComposerEvent::Edited);
        }
        cx.notify();
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        range_utf16: Option<Range<usize>>,
        new_text: &str,
        new_selected_range_utf16: Option<Range<usize>>,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.read_only {
            return;
        }
        self.prompt_history.reset_navigation();
        // A range arriving while text is marked is relative to the marked
        // text, and clipped to it the way Zed clips; only without marked
        // text is it absolute.
        let range = match (range_utf16.as_ref(), self.marked_range.as_ref()) {
            (Some(range_utf16), Some(marked)) => {
                let absolute = self.range_from_relative_utf16(marked.start, range_utf16);
                absolute.start.clamp(marked.start, marked.end)
                    ..absolute.end.clamp(marked.start, marked.end)
            }
            (Some(range_utf16), None) => self.range_from_utf16(range_utf16),
            (None, Some(marked)) => marked.clone(),
            (None, None) => self.selected_range.clone(),
        };
        self.record_edit_history(&range, new_text, true);
        let previous = self.content.clone();
        self.content =
            (self.content[..range.start].to_owned() + new_text + &self.content[range.end..]).into();
        self.marked_range =
            (!new_text.is_empty()).then_some(range.start..range.start + new_text.len());
        // Empty composition text is a cancel; close its undo step so a
        // netted-out composition leaves no trace.
        if self.marked_range.is_none() {
            self.history.finalize_composition();
        }
        // The composition's selection is also relative to the marked text,
        // which now starts at `range.start`.
        self.selected_range = new_selected_range_utf16
            .as_ref()
            .map(|new_range| self.range_from_relative_utf16(range.start, new_range))
            .unwrap_or_else(|| {
                let offset = range.start + new_text.len();
                offset..offset
            });
        self.vertical_navigation = None;
        self.pause_blink_cursor(cx);
        if previous != self.content {
            cx.emit(ComposerEvent::Edited);
        }
        cx.notify();
    }

    fn bounds_for_range(
        &mut self,
        range_utf16: Range<usize>,
        bounds: Bounds<Pixels>,
        _: &mut Window,
        _: &mut Context<Self>,
    ) -> Option<Bounds<Pixels>> {
        let layout = self.last_layout.as_ref()?;
        let range = self.range_from_utf16(&range_utf16);
        let start = layout.position_for_index(range.start)?;
        let end = layout.position_for_index(range.end)?;
        let line_height = layout.line_height();
        if start.y == end.y {
            Some(Bounds::from_corners(
                start,
                point(end.x, end.y + line_height),
            ))
        } else {
            Some(Bounds::from_corners(
                point(bounds.left(), start.y),
                point(bounds.right(), end.y + line_height),
            ))
        }
    }

    fn character_index_for_point(
        &mut self,
        point: Point<Pixels>,
        _: &mut Window,
        _: &mut Context<Self>,
    ) -> Option<usize> {
        let layout = self.last_layout.as_ref()?;
        let utf8_index = layout
            .index_for_position(point)
            .unwrap_or_else(|index| index)
            .min(self.content.len());
        Some(self.offset_to_utf16(utf8_index))
    }
}
