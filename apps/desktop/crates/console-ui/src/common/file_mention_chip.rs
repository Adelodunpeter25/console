use std::ops::Range;

use gpui::{
    App, BorderStyle, Bounds, Element, ElementId, GlobalElementId, Hsla, InspectorElementId,
    IntoElement, LayoutId, ParentElement, Styled, StyledText, TextRun, TextStyleRefinement,
    WhiteSpace, Window, div, point, px, quad,
};

use crate::markdown::render::range_rects;
use crate::theme::Theme;

/// Icon size for the inline file-mention chip. Shared by the composer paint
/// (`input/element.rs`) and the static bubble (`chat/message_bubble.rs`) so
/// the two stay visually identical.
pub const FILE_MENTION_ICON_SIZE: f32 = 11.0;

/// Corner radius for the file-mention pill. Matches the composer's painted quad.
pub const FILE_MENTION_RADIUS: f32 = 4.0;

/// Background, border, and label colors for the file-mention pill.
/// This is the composer look: accent-tinted wash with an accent label.
pub fn file_mention_colors(theme: Theme) -> (Hsla, Hsla, Hsla) {
    (
        theme.accent.opacity(0.10),
        theme.accent.opacity(0.28),
        theme.accent,
    )
}

/// Centralised inline file pill: file-type icon + filename label.
///
/// The composer cannot embed a `div` inside its `StyledText`, so it paints the
/// same colors/metrics via
/// [`file_mention_colors`], [`FILE_MENTION_ICON_SIZE`], and
/// [`FILE_MENTION_RADIUS`] instead.
pub fn file_mention_chip(path: &str, label: impl Into<String>, theme: Theme) -> impl IntoElement {
    let (bg, border, text) = file_mention_colors(theme);
    div()
        .flex()
        .items_center()
        .gap(px(4.0))
        .px(px(6.0))
        .py(px(1.0))
        .rounded(px(FILE_MENTION_RADIUS))
        .border_1()
        .border_color(border)
        .bg(bg)
        .child(crate::primitives::file_type_icon(
            path,
            FILE_MENTION_ICON_SIZE,
        ))
        .child(
            div()
                .text_size(px(12.5))
                .text_color(text)
                .child(label.into()),
        )
}

/// Non-breaking space prefix reserved before filename to house the file icon and padding.
/// Because `\u{00A0}` is a Unicode glue character, it wraps with the filename to new rows
/// without ever collapsing or protruding into negative coordinates.
pub const FILE_MENTION_SPACER: &str = "\u{00A0}\u{00A0}\u{00A0}\u{00A0}";

/// A file mention range in a flat string. The range covers only the visible
/// filename; the surrounding whitespace remains part of the normal text flow.
#[derive(Clone, Debug)]
pub(crate) struct InlineFileMention {
    pub range: Range<usize>,
    pub path: String,
}

struct MentionIconLayout {
    spacer_range: Range<usize>,
    chip_range: Range<usize>,
    icon: gpui::AnyElement,
    layout_id: LayoutId,
}

pub(crate) struct InlineFileMentionLayout {
    text: StyledText,
    text_layout_state: (),
    mention_icons: Vec<MentionIconLayout>,
}

/// One wrapped text layout with file mentions painted inline.
///
/// A flex container cannot wrap a text child around sibling chips: each child
/// is an indivisible flex item. This element keeps the message as one
/// `StyledText` so the text shaper can wrap before and after every mention,
/// while the mention wash and icon are painted from that layout's geometry.
pub(crate) struct InlineFileMentionText {
    text: String,
    mentions: Vec<InlineFileMention>,
}

impl InlineFileMentionText {
    pub(crate) fn new(text: impl Into<String>, mentions: Vec<InlineFileMention>) -> Self {
        Self {
            text: text.into(),
            mentions,
        }
    }
}

impl IntoElement for InlineFileMentionText {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl Element for InlineFileMentionText {
    type RequestLayoutState = InlineFileMentionLayout;
    type PrepaintState = ();

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
        let style = window.text_style();
        let theme = Theme::current(cx);

        // Sort mentions by range.start and assemble display text with non-breaking spacers
        let mut sorted_mentions = self.mentions.clone();
        sorted_mentions.sort_by_key(|m| m.range.start);

        let mut display_text = String::with_capacity(
            self.text.len() + sorted_mentions.len() * FILE_MENTION_SPACER.len(),
        );
        struct PreparedMention {
            spacer_range: Range<usize>,
            chip_range: Range<usize>,
            path: String,
        }
        let mut prepared_mentions = Vec::with_capacity(sorted_mentions.len());
        let mut last_idx = 0;

        for mention in &sorted_mentions {
            if mention.range.start < last_idx
                || mention.range.end > self.text.len()
                || mention.range.start >= mention.range.end
                || !self.text.is_char_boundary(mention.range.start)
                || !self.text.is_char_boundary(mention.range.end)
            {
                continue;
            }
            display_text.push_str(&self.text[last_idx..mention.range.start]);
            let spacer_start = display_text.len();
            display_text.push_str(FILE_MENTION_SPACER);
            let spacer_end = display_text.len();
            display_text.push_str(&self.text[mention.range.clone()]);
            let chip_end = display_text.len();
            last_idx = mention.range.end;

            prepared_mentions.push(PreparedMention {
                spacer_range: spacer_start..spacer_end,
                chip_range: spacer_start..chip_end,
                path: mention.path.clone(),
            });
        }
        if last_idx < self.text.len() {
            display_text.push_str(&self.text[last_idx..]);
        }

        let mut boundaries = vec![0, display_text.len()];
        for m in &prepared_mentions {
            boundaries.push(m.chip_range.start);
            boundaries.push(m.chip_range.end);
        }
        boundaries.sort_unstable();
        boundaries.dedup();

        let runs = boundaries
            .windows(2)
            .map(|range| {
                let in_mention = prepared_mentions.iter().any(|m| {
                    m.chip_range.start <= range[0] && m.chip_range.end >= range[1]
                });
                TextRun {
                    len: range[1] - range[0],
                    font: style.font(),
                    color: if in_mention {
                        theme.accent
                    } else {
                        style.color
                    },
                    background_color: None,
                    underline: None,
                    strikethrough: None,
                }
            })
            .collect();

        let mut text = StyledText::new(display_text).with_runs(runs);
        let (text_layout_id, text_layout_state) = window.with_text_style(
            Some(TextStyleRefinement {
                white_space: Some(WhiteSpace::Normal),
                ..Default::default()
            }),
            |window| text.request_layout(id, inspector_id, window, cx),
        );

        let mut mention_icons = Vec::new();
        let mut child_layout_ids = vec![text_layout_id];
        for m in prepared_mentions {
            let mut icon = div()
                .absolute()
                .child(crate::primitives::file_type_icon(
                    &m.path,
                    FILE_MENTION_ICON_SIZE,
                ))
                .into_any_element();
            let layout_id = icon.request_layout(window, cx);
            child_layout_ids.push(layout_id);
            mention_icons.push(MentionIconLayout {
                spacer_range: m.spacer_range,
                chip_range: m.chip_range,
                icon,
                layout_id,
            });
        }

        // Keep the text child in a block with a definite width. A flex parent
        // would hand it max-content width and disable soft wrapping.
        let layout_id = window.request_layout(
            gpui::Style {
                display: gpui::Display::Block,
                size: gpui::size(
                    gpui::Length::Definite(gpui::relative(1.0)),
                    gpui::Length::Auto,
                ),
                min_size: gpui::size(
                    gpui::Length::Definite(gpui::relative(0.0)),
                    gpui::Length::Auto,
                ),
                ..Default::default()
            },
            child_layout_ids,
            cx,
        );

        (
            layout_id,
            InlineFileMentionLayout {
                text,
                text_layout_state,
                mention_icons,
            },
        )
    }

    fn prepaint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        bounds: Bounds<gpui::Pixels>,
        layout_state: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        layout_state.text.prepaint(
            None,
            None,
            bounds,
            &mut layout_state.text_layout_state,
            window,
            cx,
        );

        let layout = layout_state.text.layout().clone();
        let icon_size = px(FILE_MENTION_ICON_SIZE);
        for mention_icon in &mut layout_state.mention_icons {
            let rects = range_rects(&layout, &mention_icon.spacer_range, 0.0, 2.0);
            let Some(first_rect) = rects.first() else {
                continue;
            };
            let icon_origin = point(
                first_rect.origin.x + px(3.0),
                first_rect.origin.y + (first_rect.size.height - icon_size) / 2.0,
            );
            let child_origin = window.layout_bounds(mention_icon.layout_id).origin;
            let offset = icon_origin - child_origin;
            window.with_element_offset(
                gpui::Point::new(offset.x.round(), offset.y.round()),
                |window| mention_icon.icon.prepaint(window, cx),
            );
        }
    }

    fn paint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<gpui::Pixels>,
        layout_state: &mut Self::RequestLayoutState,
        _prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        let theme = Theme::current(cx);
        let (mention_bg, mention_border, _) = file_mention_colors(theme);
        let layout = layout_state.text.layout().clone();

        for mention_icon in &layout_state.mention_icons {
            let rects = range_rects(&layout, &mention_icon.chip_range, 0.0, 2.0);
            let num_rects = rects.len();
            for (index, rect) in rects.into_iter().enumerate() {
                let mut quad_rect = rect;
                if index == num_rects - 1 {
                    quad_rect.size.width += px(4.0);
                }
                window.paint_quad(quad(
                    quad_rect,
                    px(FILE_MENTION_RADIUS),
                    mention_bg,
                    px(1.0),
                    mention_border,
                    BorderStyle::default(),
                ));
            }
        }

        layout_state.text.paint(
            None,
            None,
            layout.bounds(),
            &mut layout_state.text_layout_state,
            &mut (),
            window,
            cx,
        );
        for mention_icon in &mut layout_state.mention_icons {
            mention_icon.icon.paint(window, cx);
        }
    }
}
