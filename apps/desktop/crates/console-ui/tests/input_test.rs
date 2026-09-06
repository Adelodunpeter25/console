use std::ops::Range;
use std::time::{Duration, Instant};
use unicode_segmentation::UnicodeSegmentation;

// ── Boundary & UTF-16 Conversion Helpers ─────────────────────────────────────

fn previous_boundary(content: &str, offset: usize) -> usize {
    content
        .grapheme_indices(true)
        .rev()
        .find_map(|(index, _)| (index < offset).then_some(index))
        .unwrap_or(0)
}

fn next_boundary(content: &str, offset: usize) -> usize {
    content
        .grapheme_indices(true)
        .find_map(|(index, _)| (index > offset).then_some(index))
        .unwrap_or(content.len())
}

fn previous_word_boundary(content: &str, offset: usize) -> usize {
    content[..offset]
        .split_word_bound_indices()
        .rev()
        .find(|(_, segment)| !segment.chars().all(char::is_whitespace))
        .map(|(index, _)| index)
        .unwrap_or(0)
}

fn next_word_boundary(content: &str, offset: usize) -> usize {
    content[offset..]
        .split_word_bound_indices()
        .find(|(_, segment)| !segment.chars().all(char::is_whitespace))
        .map(|(index, segment)| offset + index + segment.len())
        .unwrap_or(content.len())
}

fn word_range_at(content: &str, offset: usize) -> Range<usize> {
    content
        .split_word_bound_indices()
        .find_map(|(index, segment)| {
            let range = index..index + segment.len();
            range.contains(&offset).then_some(range)
        })
        .unwrap_or(offset..offset)
}

fn offset_from_utf16(content: &str, offset: usize) -> usize {
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

fn offset_to_utf16(content: &str, offset: usize) -> usize {
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

fn range_to_utf16(content: &str, range: &Range<usize>) -> Range<usize> {
    offset_to_utf16(content, range.start)..offset_to_utf16(content, range.end)
}

fn range_from_utf16(content: &str, range: &Range<usize>) -> Range<usize> {
    offset_from_utf16(content, range.start)..offset_from_utf16(content, range.end)
}

fn common_prefix_len(a: &str, b: &str) -> usize {
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

fn common_suffix_len(a: &str, b: &str) -> usize {
    let mut len = a
        .as_bytes()
        .iter()
        .rev()
        .zip(b.as_bytes().iter().rev())
        .take_while(|(x, y)| x == y)
        .count();
    while !a.is_char_boundary(a.len() - len) {
        len -= 1;
    }
    len
}

fn trimmed_splice(content: &str, range: &Range<usize>, new_text: &str) -> (usize, String, String) {
    let old = &content[range.clone()];
    let prefix = common_prefix_len(old, new_text);
    let (old, new_text) = (&old[prefix..], &new_text[prefix..]);
    let suffix = common_suffix_len(old, new_text);
    (
        range.start + prefix,
        old[..old.len() - suffix].to_owned(),
        new_text[..new_text.len() - suffix].to_owned(),
    )
}

// ── Test Cases ───────────────────────────────────────────────────────────────

#[test]
fn test_grapheme_boundaries_ascii_and_multibyte() {
    let text = "a🦀b";
    // 'a' is at index 0 (len 1)
    // '🦀' is at index 1 (len 4, bytes 1..5)
    // 'b' is at index 5 (len 1)

    assert_eq!(previous_boundary(text, 0), 0);
    assert_eq!(previous_boundary(text, 1), 0);
    assert_eq!(previous_boundary(text, 5), 1);
    assert_eq!(previous_boundary(text, 6), 5);

    assert_eq!(next_boundary(text, 0), 1);
    assert_eq!(next_boundary(text, 1), 5);
    assert_eq!(next_boundary(text, 5), 6);
    assert_eq!(next_boundary(text, 6), 6);
}

#[test]
fn test_word_boundaries() {
    let text = "hello  world\nfoo_bar baz";
    // Word navigation forwards & backwards
    assert_eq!(next_word_boundary(text, 0), 5); // "hello"
    assert_eq!(next_word_boundary(text, 5), 12); // "world"
    assert_eq!(previous_word_boundary(text, 12), 7); // start of "world"
    assert_eq!(previous_word_boundary(text, 7), 0); // start of "hello"

    // Word range at offset
    assert_eq!(word_range_at(text, 2), 0..5); // inside "hello"
    assert_eq!(word_range_at(text, 9), 7..12); // inside "world"
}

#[test]
fn test_utf16_conversions() {
    // "Hi 🦀!"
    // 'H' = 1 utf8, 1 utf16
    // 'i' = 1 utf8, 1 utf16
    // ' ' = 1 utf8, 1 utf16
    // '🦀' = 4 utf8, 2 utf16 (surrogate pair)
    // '!' = 1 utf8, 1 utf16
    let text = "Hi 🦀!";
    assert_eq!(text.len(), 8); // UTF-8 byte len
    assert_eq!(text.chars().map(|c| c.len_utf16()).sum::<usize>(), 6); // UTF-16 len

    assert_eq!(offset_to_utf16(text, 0), 0);
    assert_eq!(offset_to_utf16(text, 3), 3); // before 🦀
    assert_eq!(offset_to_utf16(text, 7), 5); // after 🦀, before !
    assert_eq!(offset_to_utf16(text, 8), 6); // end of string

    assert_eq!(offset_from_utf16(text, 0), 0);
    assert_eq!(offset_from_utf16(text, 3), 3);
    assert_eq!(offset_from_utf16(text, 5), 7);
    assert_eq!(offset_from_utf16(text, 6), 8);

    let range_utf8 = 3..7; // covers 🦀
    let range_utf16 = range_to_utf16(text, &range_utf8);
    assert_eq!(range_utf16, 3..5);
    assert_eq!(range_from_utf16(text, &range_utf16), range_utf8);
}

#[test]
fn test_trimmed_splice() {
    let content = "The quick brown fox";
    // Replace "quick brown" with "quick red" -> shared prefix "quick "
    let range = 4..15;
    let (start, old, new) = trimmed_splice(content, &range, "quick red");
    assert_eq!(start, 10);
    assert_eq!(old, "brown");
    assert_eq!(new, "red");
}

#[test]
fn test_common_prefix_and_suffix_multibyte() {
    let a = "hello 🦀 world";
    let b = "hello 🦀 universe";
    let prefix = common_prefix_len(a, b);
    assert_eq!(prefix, 11); // "hello 🦀 "
    assert_eq!(&a[..prefix], "hello 🦀 ");

    let c = "start 🦀 end";
    let d = "finish 🦀 end";
    let suffix = common_suffix_len(c, d);
    assert_eq!(suffix, 9); // " 🦀 end" (1 + 4 + 4 bytes)
    assert_eq!(&c[c.len() - suffix..], " 🦀 end");
}

#[test]
fn test_prompt_history_navigation_logic() {
    struct PromptHistory {
        entries: Vec<String>,
        draft: Option<String>,
        index: Option<usize>,
    }

    impl PromptHistory {
        fn record(&mut self, text: String) {
            if text.trim().is_empty() {
                return;
            }
            if self.entries.last() == Some(&text) {
                return;
            }
            self.entries.push(text);
            self.index = None;
            self.draft = None;
        }

        fn reset_navigation(&mut self) {
            self.index = None;
            self.draft = None;
        }

        fn navigate(&mut self, next: bool, current: &str) -> Option<String> {
            if self.entries.is_empty() {
                return None;
            }
            if next {
                let Some(index) = self.index else {
                    return None;
                };
                if index + 1 < self.entries.len() {
                    let next = index + 1;
                    self.index = Some(next);
                    return Some(self.entries[next].clone());
                }
                self.index = None;
                return Some(self.draft.take().unwrap_or_default());
            }

            let next = match self.index {
                Some(index) => index.saturating_sub(1),
                None => {
                    self.draft = Some(current.to_owned());
                    self.entries.len() - 1
                }
            };
            self.index = Some(next);
            Some(self.entries[next].clone())
        }
    }

    let mut history = PromptHistory {
        entries: Vec::new(),
        draft: None,
        index: None,
    };

    history.record("prompt 1".into());
    history.record("prompt 2".into());

    // Up: recalls latest prompt ("prompt 2") and saves current draft
    let recalled = history.navigate(false, "current draft");
    assert_eq!(recalled, Some("prompt 2".into()));
    assert_eq!(history.draft, Some("current draft".into()));

    // Up again: recalls "prompt 1"
    let recalled_prev = history.navigate(false, "prompt 2");
    assert_eq!(recalled_prev, Some("prompt 1".into()));

    // Down: returns to "prompt 2"
    let down_1 = history.navigate(true, "prompt 1");
    assert_eq!(down_1, Some("prompt 2".into()));

    // Down again: returns to saved "current draft"
    let down_2 = history.navigate(true, "prompt 2");
    assert_eq!(down_2, Some("current draft".into()));
    assert_eq!(history.index, None);

    // Reset navigation
    history.reset_navigation();
    assert_eq!(history.index, None);
    assert_eq!(history.draft, None);
}

#[test]
fn test_edit_history_coalescing_and_undo_redo() {
    struct EditRecord {
        start: usize,
        old: String,
        new: String,
        selection_before: Range<usize>,
        selection_reversed_before: bool,
        edited_at: Instant,
        sealed: bool,
        composing: bool,
    }

    #[derive(Default)]
    struct EditHistory {
        undo: Vec<EditRecord>,
        redo: Vec<EditRecord>,
    }

    const UNDO_GROUP_INTERVAL: Duration = Duration::from_millis(300);

    impl EditHistory {
        fn record(
            &mut self,
            content: &str,
            range: &Range<usize>,
            new_text: &str,
            selection: Range<usize>,
            selection_reversed: bool,
            now: Instant,
        ) {
            let (start, old, new) = trimmed_splice(content, range, new_text);
            if old.is_empty() && new.is_empty() {
                return;
            }
            self.redo.clear();
            if let Some(last) = self.undo.last_mut()
                && !last.sealed
                && !last.composing
                && now.duration_since(last.edited_at) < UNDO_GROUP_INTERVAL
            {
                if old.is_empty() && !last.new.is_empty() && start == last.start + last.new.len() {
                    last.new.push_str(&new);
                    last.edited_at = now;
                    return;
                }
                if new.is_empty() && last.new.is_empty() && !last.old.is_empty() {
                    if start + old.len() == last.start {
                        last.start = start;
                        last.old.insert_str(0, &old);
                        last.edited_at = now;
                        return;
                    }
                    if start == last.start {
                        last.old.push_str(&old);
                        last.edited_at = now;
                        return;
                    }
                }
            }
            self.undo.push(EditRecord {
                start,
                old,
                new,
                selection_before: selection,
                selection_reversed_before: selection_reversed,
                edited_at: now,
                sealed: false,
                composing: false,
            });
        }

        fn seal(&mut self) {
            if let Some(last) = self.undo.last_mut() {
                last.sealed = true;
            }
        }

        fn undo(&mut self, content: &str) -> Option<(String, Range<usize>, bool)> {
            let record = self.undo.pop()?;
            let span = record.start..record.start + record.new.len();
            if content.get(span.clone()) != Some(record.new.as_str()) {
                self.undo.clear();
                self.redo.clear();
                return None;
            }
            let restored = [&content[..span.start], &record.old, &content[span.end..]].concat();
            let selection = record.selection_before.start.min(restored.len())
                ..record.selection_before.end.min(restored.len());
            let selection_reversed = record.selection_reversed_before;
            self.redo.push(record);
            Some((restored, selection, selection_reversed))
        }

        fn redo(&mut self, content: &str) -> Option<(String, Range<usize>, bool)> {
            let record = self.redo.pop()?;
            let span = record.start..record.start + record.old.len();
            if content.get(span.clone()) != Some(record.old.as_str()) {
                self.undo.clear();
                self.redo.clear();
                return None;
            }
            let applied = [&content[..span.start], &record.new, &content[span.end..]].concat();
            let caret = record.start + record.new.len();
            self.undo.push(record);
            Some((applied, caret..caret, false))
        }
    }

    let mut history = EditHistory::default();
    let mut content = String::new();
    let t0 = Instant::now();

    // 1. Coalesced typing: "h", "e", "l", "l", "o"
    history.record(&content, &(0..0), "h", 0..0, false, t0);
    content.push('h');
    history.record(&content, &(1..1), "e", 1..1, false, t0 + Duration::from_millis(50));
    content.push('e');
    history.record(&content, &(2..2), "llo", 2..2, false, t0 + Duration::from_millis(100));
    content.push_str("llo");

    assert_eq!(content, "hello");
    assert_eq!(history.undo.len(), 1); // all coalesced into 1 step

    // Undo restores empty string
    let (undone, sel, _) = history.undo(&content).expect("undo");
    assert_eq!(undone, "");
    assert_eq!(sel, 0..0);

    // Redo restores "hello"
    let (redone, sel, _) = history.redo(&undone).expect("redo");
    assert_eq!(redone, "hello");
    assert_eq!(sel, 5..5);

    // 2. Sealed edit (e.g. paste): does NOT coalesce with subsequent typing
    history.seal();
    history.record(&redone, &(5..5), " world", 5..5, false, t0 + Duration::from_millis(150));
    let mut content2 = format!("{} world", redone);
    assert_eq!(history.undo.len(), 2);

    history.seal();
    history.record(&content2, &(11..11), "!", 11..11, false, t0 + Duration::from_millis(200));
    content2.push('!');
    assert_eq!(history.undo.len(), 3);

    // Undo "!"
    let (undone1, _, _) = history.undo(&content2).expect("undo !");
    assert_eq!(undone1, "hello world");

    // Undo " world"
    let (undone2, _, _) = history.undo(&undone1).expect("undo world");
    assert_eq!(undone2, "hello");

    // Undo "hello"
    let (undone3, _, _) = history.undo(&undone2).expect("undo hello");
    assert_eq!(undone3, "");
}
