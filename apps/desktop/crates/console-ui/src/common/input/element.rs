use gpui::{
    fill, point, px, quad, size, App, BorderStyle, Bounds, DispatchPhase,
    Element, ElementId, ElementInputHandler, Entity, GlobalElementId, InspectorElementId,
    IntoElement, LayoutId, MouseMoveEvent, PaintQuad, Pixels, Point, ScrollHandle, StyledText,
    TextLayout, TextRun, Window,
};

use super::text_runs::{input_text_runs, SearchPaint};
use super::{ComposerInput, FieldMode};
use crate::theme::Theme;

pub(crate) fn visual_row_count(layout: &TextLayout) -> usize {
    layout
        .line_layouts()
        .iter()
        .map(|line| line.wrap_boundaries().len() + 1)
        .sum()
}

/// Resolve the closest caret offset on one rendered row for a desired x.
/// GPUI's whole-text `position_for_index` intentionally gives a soft-wrap
/// boundary to the preceding row, so this works against the concrete wrapped
/// row and returns its unambiguous caret x as well as the byte offset.
pub(crate) fn visual_row_offset_for_x(
    layout: &TextLayout,
    visual_row: usize,
    goal_x: Pixels,
) -> Option<(usize, Pixels)> {
    let line_height = layout.line_height();
    let mut first_visual_row = 0;
    let mut first_byte = 0;

    for line in layout.line_layouts() {
        let line_row_count = line.wrap_boundaries().len() + 1;
        if visual_row < first_visual_row + line_row_count {
            let row_in_line = visual_row - first_visual_row;
            let local_offset = line
                .closest_index_for_position(
                    point(goal_x, line_height * (row_in_line as f32 + 0.5)),
                    line_height,
                )
                .unwrap_or_else(|offset| offset);
            let row_start = row_in_line
                .checked_sub(1)
                .and_then(|boundary_index| line.wrap_boundaries().get(boundary_index))
                .map(|boundary| {
                    line.unwrapped_layout.runs[boundary.run_ix].glyphs[boundary.glyph_ix].index
                })
                .unwrap_or(0);
            let cursor_x = line.unwrapped_layout.x_for_index(local_offset)
                - line.unwrapped_layout.x_for_index(row_start);
            return Some((first_byte + local_offset, cursor_x));
        }
        first_visual_row += line_row_count;
        // TextLayout separates its newline-delimited shaped lines by the one
        // source byte occupied by `\n`.
        first_byte += line.len() + 1;
    }
    None
}

pub(crate) fn cursor_should_be_visible(
    window_active: bool,
    input_focused: bool,
    context_menu_preserves_focus: bool,
    blink_visible: bool,
) -> bool {
    window_active && (context_menu_preserves_focus || (input_focused && blink_visible))
}

/// Horizontal scroll for a focused single-line field, reconciled every frame
/// against the selection the way Zed's `autoscroll_horizontally` resolves an
/// autoscroll request. `start_x`/`head_x`/`end_x` are text-relative pixel
/// positions of the selection edges (all equal for a caret), `em` is one
/// character advance of lookahead kept past the caret, and `previous` is last
/// frame's scroll, moved as little as possible:
/// - a caret, or a selection that fits, is revealed whole plus the lookahead;
/// - a partial selection too wide to fit follows its head, the way a native
///   field tracks shift+End;
/// - a whole-content selection holds still — native `selectAll` never moves
///   the view, which keeps the focus shortcut on the address bar showing the host.
pub(crate) fn single_line_scroll(
    previous: Pixels,
    viewport: Pixels,
    em: Pixels,
    text_width: Pixels,
    (start_x, head_x, end_x): (Pixels, Pixels, Pixels),
    whole_content_selected: bool,
) -> Pixels {
    let max_scroll = (text_width + em - viewport).max(px(0.));
    let scroll = previous.min(max_scroll).max(px(0.));
    let (target_left, target_right) = if end_x - start_x + em <= viewport {
        (start_x, end_x + em)
    } else if whole_content_selected {
        return scroll;
    } else {
        (head_x, head_x + em)
    };
    if target_left < scroll {
        target_left
    } else if target_right > scroll + viewport {
        target_right - viewport
    } else {
        scroll
    }
}

/// Vertical analogue of [`single_line_scroll`] for a composer-mode field
/// capped at [`COMPOSER_MAX_HEIGHT`]: scroll the viewport the minimum needed
/// to keep the caret inside it. `caret_top` is the caret's window position in
/// this frame's already-scrolled layout, and the container consumed this
/// frame's offset before prepainting children, so a correction lands on the
/// next frame — which this requests.
pub(crate) fn follow_caret(
    caret_top: Point<Pixels>,
    line_height: Pixels,
    scroll_handle: &ScrollHandle,
    window: &mut Window,
) {
    let viewport = scroll_handle.bounds();
    if viewport.size.height <= px(0.) {
        return;
    }
    let offset = scroll_handle.offset();
    let mut y = offset.y;
    let caret_bottom = caret_top.y + line_height;
    if caret_bottom > viewport.bottom() {
        y -= caret_bottom - viewport.bottom();
    } else if caret_top.y < viewport.top() {
        y += viewport.top() - caret_top.y;
    }
    let y = y.clamp(-scroll_handle.max_offset().y, px(0.));
    if (y - offset.y).abs() > px(0.5) {
        scroll_handle.set_offset(point(offset.x, y));
        window.request_animation_frame();
    }
}

pub(crate) struct InputElement {
    pub(crate) input: Entity<ComposerInput>,
}

impl InputElement {
    /// For a single-line (search-mode) field, the bounds the text is actually
    /// laid out at: the unwrapped line anchored `scroll_offset` left of the
    /// clipped viewport so the caret stays in view. `None` for wrapping
    /// fields, which lay out at their element bounds. Also reconciles the
    /// scroll for this frame, so the caller must prepaint at the returned
    /// bounds for index↔position math to agree with what is painted.
    fn single_line_text_bounds(
        &self,
        bounds: Bounds<Pixels>,
        layout_state: &mut InputLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Bounds<Pixels>> {
        let (focused, selection, whole_content_selected, previous_scroll) = {
            let input = self.input.read(cx);
            if input.mode != FieldMode::Search {
                return None;
            }
            (
                input.is_visually_focused(window),
                (
                    input.selected_range.start,
                    input.cursor_offset(),
                    input.selected_range.end,
                ),
                !input.content.is_empty() && input.selected_range == (0..input.content.len()),
                input.scroll_offset,
            )
        };
        // Anchor the line at the natural origin first so index → position
        // can measure it; the definitive prepaint happens at the scrolled
        // origin returned from here.
        layout_state.text.prepaint(
            None,
            None,
            bounds,
            &mut layout_state.text_layout_state,
            window,
            cx,
        );
        let layout = layout_state.text.layout().clone();
        let x_for_index = |index: usize| {
            layout
                .position_for_index(index)
                .map_or(px(0.), |position| position.x - bounds.origin.x)
        };
        let text_width = x_for_index(layout.len());
        let scroll = if focused {
            let style = window.text_style();
            let font_id = window.text_system().resolve_font(&style.font());
            let font_size = style.font_size.to_pixels(window.rem_size());
            let em = window
                .text_system()
                .em_advance(font_id, font_size)
                .unwrap_or(px(8.));
            single_line_scroll(
                previous_scroll,
                bounds.size.width,
                em,
                text_width,
                (
                    x_for_index(selection.0),
                    x_for_index(selection.1),
                    x_for_index(selection.2),
                ),
                whole_content_selected,
            )
        } else {
            px(0.)
        };
        self.input
            .update(cx, |input, _| input.scroll_offset = scroll);
        Some(Bounds::new(
            point(bounds.origin.x - scroll, bounds.origin.y),
            size(bounds.size.width.max(text_width), bounds.size.height),
        ))
    }
}

pub(crate) struct InputLayoutState {
    pub(crate) text: StyledText,
    pub(crate) text_layout_state: (),
}

pub(crate) struct PrepaintState {
    pub(crate) cursor: Option<PaintQuad>,
}

impl IntoElement for InputElement {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl Element for InputElement {
    type RequestLayoutState = InputLayoutState;
    type PrepaintState = PrepaintState;

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static core::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        id: Option<&GlobalElementId>,
        inspector_id: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        let input = self.input.read(cx);
        let content = input.content.clone();
        let style = window.text_style();
        let theme = Theme::current(cx);
        let content_is_empty = content.is_empty();
        let (display_text, text_color, selected_range, marked_range) = if content_is_empty {
            (input.placeholder.clone(), theme.text_ghost, None, None)
        } else {
            (
                content,
                style.color,
                Some(&input.selected_range),
                input.marked_range.as_ref(),
            )
        };
        let base_run = TextRun {
            len: display_text.len(),
            font: style.font(),
            color: text_color,
            background_color: None,
            underline: None,
            strikethrough: None,
        };
        let palette = crate::markdown::render::Palette::from_theme(&theme);
        let search = if content_is_empty {
            SearchPaint::none()
        } else {
            SearchPaint {
                matches: &input.search_matches,
                active: input
                    .active_search_match
                    .and_then(|index| input.search_matches.get(index)),
                match_color: theme.warning.opacity(0.22),
                active_color: theme.warning.opacity(0.5),
            }
        };
        let runs = input_text_runs(
            display_text.len(),
            base_run,
            selected_range,
            marked_range,
            theme.selection,
            if content_is_empty {
                &[]
            } else {
                &input.highlight
            },
            |class| palette.token(class),
            search,
            if content_is_empty || input.mode != FieldMode::Composer {
                &[]
            } else {
                &input.mentions
            },
            theme.accent,
            theme.accent.opacity(0.12),
        );
        let mut text = StyledText::new(display_text).with_runs(runs);
        let (layout_id, text_layout_state) = text.request_layout(id, inspector_id, window, cx);
        (
            layout_id,
            InputLayoutState {
                text,
                text_layout_state,
            },
        )
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        layout_state: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let text_bounds = self
            .single_line_text_bounds(bounds, layout_state, window, cx)
            .unwrap_or(bounds);
        layout_state.text.prepaint(
            None,
            None,
            text_bounds,
            &mut layout_state.text_layout_state,
            window,
            cx,
        );
        let theme = Theme::current(cx);
        let layout = layout_state.text.layout().clone();
        let (cursor_position, cursor, follow) = {
            let input = self.input.read(cx);
            let cursor = input.cursor_offset();
            let cursor_visible = cursor_should_be_visible(
                window.is_window_active(),
                input.focus_handle.is_focused(window),
                input.context_menu_preserves_visual_focus(),
                input.blink_cursor.read(cx).visible(),
            );
            // The caret's position feeds both its painted quad and the capped
            // viewport's follow below, so resolve it regardless of blink
            // phase or selection.
            let cursor_position = input
                .vertical_navigation
                .filter(|navigation| {
                    navigation.cursor_offset == cursor
                        && navigation.layout_width == layout.bounds().size.width
                })
                .map(|navigation| {
                    point(
                        layout.bounds().left() + navigation.cursor_x,
                        layout.bounds().top() + layout.line_height() * navigation.visual_row as f32,
                    )
                })
                .or_else(|| layout.position_for_index(cursor));
            let quad = (input.selected_range.is_empty() && cursor_visible)
                .then_some(cursor_position)
                .flatten()
                .map(|cursor_position| {
                    fill(
                        Bounds::new(cursor_position, size(px(1.5), layout.line_height())),
                        theme.accent,
                    )
                });
            let follow = (input.mode == FieldMode::Composer).then(|| {
                (
                    (cursor, input.content.len(), layout.bounds().size.width),
                    input.caret_reconciled,
                    input.scroll_handle.clone(),
                )
            });
            (cursor_position, quad, follow)
        };
        if let Some((follow_state, reconciled, scroll_handle)) = follow
            && reconciled != Some(follow_state)
        {
            if let Some(position) = cursor_position {
                follow_caret(position, layout.line_height(), &scroll_handle, window);
            }
            self.input
                .update(cx, |input, _| input.caret_reconciled = Some(follow_state));
        }
        PrepaintState { cursor }
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        layout_state: &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        let input = self.input.read(cx);
        let focus_handle = input.focus_handle.clone();
        let visually_focused = input.is_visually_focused(window);
        window.handle_input(
            &focus_handle,
            ElementInputHandler::new(bounds, self.input.clone()),
            cx,
        );
        // Element-level mouse listeners are hitbox-gated, so a drag that
        // leaves the field would freeze the selection at the border. Track
        // the drag window-level instead — like a native text view, the
        // selection keeps extending outside the bounds because
        // index_for_mouse_position clamps an outside point to the nearest
        // line edge (above maps to the start, below to the end).
        window.on_mouse_event({
            let input = self.input.clone();
            move |event: &MouseMoveEvent, phase, window, cx| {
                if phase == DispatchPhase::Bubble && input.read(cx).is_selecting {
                    input.update(cx, |input, cx| input.on_mouse_move(event, window, cx));
                }
            }
        });
        if input.mode == FieldMode::Composer && !input.mentions.is_empty() {
            let theme = Theme::current(cx);
            let layout = layout_state.text.layout();
            for mention in &input.mentions {
                if mention.range.end <= input.content.len() {
                    for rect in crate::markdown::render::range_rects(layout, &mention.range, 3.0, 1.0) {
                        window.paint_quad(quad(
                            rect,
                            px(4.0),
                            theme.accent.opacity(0.10),
                            px(1.0),
                            theme.accent.opacity(0.28),
                            BorderStyle::default(),
                        ));
                    }
                }
            }
        }
        layout_state.text.paint(
            None,
            None,
            bounds,
            &mut layout_state.text_layout_state,
            &mut (),
            window,
            cx,
        );
        if visually_focused && let Some(cursor) = prepaint.cursor.take() {
            window.paint_quad(cursor);
        }
        let text_layout = layout_state.text.layout().clone();
        self.input.update(cx, |input, _| {
            input.last_layout = Some(text_layout);
        });
    }
}
