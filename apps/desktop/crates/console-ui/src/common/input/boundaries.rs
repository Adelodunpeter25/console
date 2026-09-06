use std::ops::Range;
use unicode_segmentation::UnicodeSegmentation;

pub fn previous_boundary(content: &str, offset: usize) -> usize {
    content
        .grapheme_indices(true)
        .rev()
        .find_map(|(index, _)| (index < offset).then_some(index))
        .unwrap_or(0)
}

pub fn next_boundary(content: &str, offset: usize) -> usize {
    content
        .grapheme_indices(true)
        .find_map(|(index, _)| (index > offset).then_some(index))
        .unwrap_or(content.len())
}

pub fn previous_grapheme_boundary(content: &str, offset: usize) -> usize {
    previous_boundary(content, offset)
}

pub fn next_grapheme_boundary(content: &str, offset: usize) -> usize {
    next_boundary(content, offset)
}

pub fn previous_word_boundary(content: &str, offset: usize) -> usize {
    content[..offset]
        .split_word_bound_indices()
        .rev()
        .find(|(_, segment)| !segment.chars().all(char::is_whitespace))
        .map(|(index, _)| index)
        .unwrap_or(0)
}

pub fn next_word_boundary(content: &str, offset: usize) -> usize {
    content[offset..]
        .split_word_bound_indices()
        .find(|(_, segment)| !segment.chars().all(char::is_whitespace))
        .map(|(index, segment)| offset + index + segment.len())
        .unwrap_or(content.len())
}

pub fn word_range_at(content: &str, offset: usize) -> Range<usize> {
    content
        .split_word_bound_indices()
        .find_map(|(index, segment)| {
            let range = index..index + segment.len();
            range.contains(&offset).then_some(range)
        })
        .unwrap_or(offset..offset)
}

pub fn offset_from_utf16(content: &str, offset: usize) -> usize {
    let mut utf8_offset = 0;
    let mut utf16_count = 0;
    for character in content.chars() {
        if utf16_count >= offset {
            break;
        }
        utf16_count += character.len_utf16();
        utf8_offset += character.len_utf8();
    }
    utf8_offset
}

pub fn offset_to_utf16(content: &str, offset: usize) -> usize {
    let mut utf16_offset = 0;
    let mut utf8_count = 0;
    for character in content.chars() {
        if utf8_count >= offset {
            break;
        }
        utf8_count += character.len_utf8();
        utf16_offset += character.len_utf16();
    }
    utf16_offset
}

pub fn range_to_utf16(content: &str, range: &Range<usize>) -> Range<usize> {
    offset_to_utf16(content, range.start)..offset_to_utf16(content, range.end)
}

pub fn range_from_utf16(content: &str, range: &Range<usize>) -> Range<usize> {
    offset_from_utf16(content, range.start)..offset_from_utf16(content, range.end)
}

/// Resolves a range whose endpoints count UTF-16 units from `base`, the
/// form macOS uses for everything relative to the marked text. The offsets
/// have to be added in UTF-16 before the conversion; converting first and
/// adding to `base` overshoots once anything multi-byte precedes it.
pub fn range_from_relative_utf16(content: &str, base: usize, range: &Range<usize>) -> Range<usize> {
    let base_utf16 = offset_to_utf16(content, base);
    range_from_utf16(content, &(base_utf16 + range.start..base_utf16 + range.end))
}

/// Bytes shared at the start of both strings, backed off to a character
/// boundary so a partially shared code point is not split.
pub fn common_prefix_len(a: &str, b: &str) -> usize {
    let mut len = a
        .as_bytes()
        .iter()
        .zip(b.as_bytes())
        .take_while(|(x, y)| x == y)
        .count();
    while !a.is_char_boundary(len) {
        len -= 1;
    }
    len
}

/// Bytes shared at the end of both strings, clamped to where prefix matching
/// stopped and backed off to a character boundary.
pub fn common_suffix_len(a: &str, b: &str, prefix_len: usize) -> usize {
    let mut len = a[prefix_len..]
        .as_bytes()
        .iter()
        .rev()
        .zip(b[prefix_len..].as_bytes().iter().rev())
        .take_while(|(x, y)| x == y)
        .count();
    while !a.is_char_boundary(a.len() - len) || !b.is_char_boundary(b.len() - len) {
        len -= 1;
    }
    len
}

/// Trim identical prefix and suffix bytes from a proposed splice so the
/// resulting record captures only the changed core.
pub fn trimmed_splice(
    old_text: &str,
    old_range: &Range<usize>,
    new_text: &str,
) -> (Range<usize>, String, String) {
    let old_splice = &old_text[old_range.clone()];
    let prefix = common_prefix_len(old_splice, new_text);
    let suffix = common_suffix_len(old_splice, new_text, prefix);
    let trimmed_range = (old_range.start + prefix)..(old_range.end - suffix);
    let trimmed_old = old_splice[prefix..old_splice.len() - suffix].to_string();
    let trimmed_new = new_text[prefix..new_text.len() - suffix].to_string();
    (trimmed_range, trimmed_old, trimmed_new)
}
