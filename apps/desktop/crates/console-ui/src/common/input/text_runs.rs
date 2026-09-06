use std::ops::Range;

use gpui::{px, Hsla, TextRun, UnderlineStyle};

use super::mentions::ComposerMention;
use crate::markdown::highlight::TokenClass;

/// Search-match rendering configuration passed into [`input_text_runs`].
///
/// Matches paint underneath the selection wash. Inactive matches get
/// `match_color`, and the one navigation is on gets `active_color`, which also
/// wins over the selection wash so the current match keeps its identity while
/// selected — the way a find widget conventionally paints it.
pub struct SearchPaint<'a> {
    pub matches: &'a [Range<usize>],
    pub active: Option<&'a Range<usize>>,
    pub match_color: Hsla,
    pub active_color: Hsla,
}

impl SearchPaint<'static> {
    pub fn none() -> Self {
        Self {
            matches: &[],
            active: None,
            match_color: gpui::transparent_black(),
            active_color: gpui::transparent_black(),
        }
    }
}

#[allow(clippy::too_many_arguments)]
pub fn input_text_runs(
    display_len: usize,
    base_run: TextRun,
    selected_range: Option<&Range<usize>>,
    marked_range: Option<&Range<usize>>,
    selection_color: Hsla,
    highlight: &[(Range<usize>, TokenClass)],
    token_color: impl Fn(TokenClass) -> Hsla,
    search: SearchPaint,
    mentions: &[ComposerMention],
    mention_color: Hsla,
    mention_bg_color: Hsla,
) -> Vec<TextRun> {
    let mut boundaries = vec![0, display_len];
    for range in [selected_range, marked_range].into_iter().flatten() {
        boundaries.push(range.start.min(display_len));
        boundaries.push(range.end.min(display_len));
    }
    for (range, _) in highlight {
        boundaries.push(range.start.min(display_len));
        boundaries.push(range.end.min(display_len));
    }
    for range in search.matches {
        boundaries.push(range.start.min(display_len));
        boundaries.push(range.end.min(display_len));
    }
    for mention in mentions {
        boundaries.push(mention.range.start.min(display_len));
        boundaries.push(mention.range.end.min(display_len));
    }
    boundaries.sort_unstable();
    boundaries.dedup();

    // Token and match lists are sorted and non-overlapping, and every one of
    // their edges is a boundary, so each window has exactly one candidate —
    // found by binary search, keeping this linear-ish in the token count
    // rather than quadratic.
    let covering_match = |start: usize, end: usize| -> bool {
        let index = search.matches.partition_point(|range| range.end <= start);
        search
            .matches
            .get(index)
            .is_some_and(|range| range.start <= start && range.end >= end)
    };
    let covering_mention = |start: usize, end: usize| -> bool {
        let index = mentions.partition_point(|m| m.range.end <= start);
        mentions
            .get(index)
            .is_some_and(|m| m.range.start <= start && m.range.end >= end)
    };

    boundaries
        .windows(2)
        .filter_map(|boundary| {
            let start = boundary[0];
            let end = boundary[1];
            let token_index = highlight.partition_point(|(range, _)| range.end <= start);
            let is_in_mention = covering_mention(start, end);
            let color = if is_in_mention
                && selected_range.is_none_or(|range| range.start >= end || range.end <= start)
            {
                mention_color
            } else {
                highlight
                    .get(token_index)
                    .filter(|(range, _)| range.start <= start && range.end >= end)
                    .map_or(base_run.color, |(_, class)| token_color(*class))
            };
            let background_color = if search
                .active
                .is_some_and(|range| range.start <= start && range.end >= end)
            {
                Some(search.active_color)
            } else if selected_range.is_some_and(|range| range.start < end && range.end > start) {
                Some(selection_color)
            } else if covering_match(start, end) {
                Some(search.match_color)
            } else if is_in_mention {
                Some(mention_bg_color)
            } else {
                None
            };
            (start < end).then(|| TextRun {
                len: end - start,
                color,
                background_color,
                underline: marked_range
                    .filter(|range| range.start < end && range.end > start)
                    .map(|_| UnderlineStyle {
                        color: Some(base_run.color),
                        thickness: px(1.0),
                        wavy: false,
                    }),
                ..base_run.clone()
            })
        })
        .collect()
}
