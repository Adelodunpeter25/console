use std::ops::Range;
use std::time::{Duration, Instant};

use super::boundaries::trimmed_splice;

/// How long after the previous edit a new one may still coalesce into the
/// same undo step — Zed's transaction group interval.
pub const UNDO_GROUP_INTERVAL: Duration = Duration::from_millis(300);

/// Undo steps kept before the oldest fall off. Steps are whole coalesced
/// gestures, so this is far more editing than anyone steps back through.
pub const UNDO_HISTORY_CAP: usize = 1000;

/// One undo step: the text at `start` was `old` and is now `new`. Undoing
/// splices `old` back over `new`; redoing reverses that. A step grows in
/// place while a run of typing or deleting coalesces into it, so the stack
/// stays proportional to gestures, not keystrokes.
pub struct EditRecord {
    pub start: usize,
    pub old: String,
    pub new: String,
    /// Selection to restore when the step is undone.
    pub selection_before: Range<usize>,
    pub selection_reversed_before: bool,
    /// When the newest coalesced edit landed, bounding the group interval.
    pub edited_at: Instant,
    /// A sealed step never coalesces with later edits. Set at gesture
    /// boundaries: cut, paste, a completion insert, a finished composition.
    pub sealed: bool,
    /// An IME composition still underway, amended in place as the marked
    /// text changes so the whole entry undoes as one step.
    pub composing: bool,
}

#[derive(Default)]
pub struct EditHistory {
    pub undo: Vec<EditRecord>,
    pub redo: Vec<EditRecord>,
}

impl EditHistory {
    /// Record a splice of `new_text` over `range`, called with the content
    /// the splice has not yet been applied to. Coalesces with the newest
    /// step where the pair reads as one gesture: an insertion continuing at
    /// the end of an insertion, or a deletion extending a deletion run in
    /// either direction.
    pub fn record(
        &mut self,
        content: &str,
        range: &Range<usize>,
        new_text: &str,
        selection: Range<usize>,
        selection_reversed: bool,
        now: Instant,
    ) {
        // A whole-content replace (Replace All) arrives as one huge splice;
        // trimming the shared affixes stores only the span that changed.
        let (trimmed_range, old, new) = trimmed_splice(content, range, new_text);
        let start = trimmed_range.start;
        if old.is_empty() && new.is_empty() {
            return;
        }
        self.redo.clear();
        if let Some(last) = self.undo.last_mut()
            && !last.sealed
            && !last.composing
            && now.duration_since(last.edited_at) < UNDO_GROUP_INTERVAL
        {
            // Typing run: this insertion continues where the last edit's text
            // ended, which also lets typing extend a replaced selection.
            if old.is_empty() && !last.new.is_empty() && start == last.start + last.new.len() {
                last.new.push_str(&new);
                last.edited_at = now;
                return;
            }
            if new.is_empty() && last.new.is_empty() && !last.old.is_empty() {
                // Backspace run: this deletion ends where the last one started.
                if start + old.len() == last.start {
                    last.start = start;
                    last.old.insert_str(0, &old);
                    last.edited_at = now;
                    return;
                }
                // Forward-delete run: this deletion starts where the last did.
                if start == last.start {
                    last.old.push_str(&old);
                    last.edited_at = now;
                    return;
                }
            }
        }
        self.push(EditRecord {
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

    /// Record an IME splice. The whole composition — every marked-text
    /// revision and the final commit — stays one step, amended in place.
    pub fn record_composition(
        &mut self,
        content: &str,
        range: &Range<usize>,
        new_text: &str,
        selection: Range<usize>,
        selection_reversed: bool,
        now: Instant,
    ) {
        self.redo.clear();
        if let Some(last) = self.undo.last_mut()
            && last.composing
        {
            let span = last.start..last.start + last.new.len();
            if range.start >= span.start && range.end <= span.end {
                last.new
                    .replace_range(range.start - last.start..range.end - last.start, new_text);
                last.edited_at = now;
                return;
            }
            // A splice outside the open composition should not happen; close
            // the step rather than corrupt it.
            last.composing = false;
            last.sealed = true;
        }
        self.push(EditRecord {
            start: range.start,
            old: content[range.clone()].to_owned(),
            new: new_text.to_owned(),
            selection_before: selection,
            selection_reversed_before: selection_reversed,
            edited_at: now,
            sealed: false,
            composing: true,
        });
    }

    /// Close the open composition step, if any. A canceled composition that
    /// nets no change records nothing.
    pub fn finalize_composition(&mut self) {
        if let Some(last) = self.undo.last_mut()
            && last.composing
        {
            last.composing = false;
            last.sealed = true;
            if last.old == last.new {
                self.undo.pop();
            }
        }
    }

    /// Stop later edits from coalescing into the newest step — a gesture
    /// boundary such as cut, paste, or a completion insert.
    pub fn seal(&mut self) {
        if let Some(last) = self.undo.last_mut() {
            last.sealed = true;
        }
    }

    pub fn push(&mut self, record: EditRecord) {
        self.undo.push(record);
        if self.undo.len() > UNDO_HISTORY_CAP {
            let excess = self.undo.len() - UNDO_HISTORY_CAP;
            self.undo.drain(..excess);
        }
    }

    /// Apply the newest undo step to `content`, returning the restored
    /// content and the selection to show. The step must still verifiably
    /// describe `content`; on any mismatch the history is corrupt and is
    /// dropped whole rather than applied wrong.
    pub fn undo(&mut self, content: &str) -> Option<(String, Range<usize>, bool)> {
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

    /// Reapply the newest undone step, with the caret after the re-applied
    /// text — where the original edit left it.
    pub fn redo(&mut self, content: &str) -> Option<(String, Range<usize>, bool)> {
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

/// Prompt submission history, recalled with Up/Down while the caret is at the
/// top/bottom of the composer.
#[derive(Default)]
pub struct PromptHistory {
    pub entries: Vec<String>,
    /// Uncommitted draft saved when starting history navigation, restored
    /// when stepping forward past the newest entry.
    pub draft: Option<String>,
    /// Index into `entries` of the entry currently showing. `None` while
    /// editing the draft.
    pub index: Option<usize>,
}

impl PromptHistory {
    pub fn set_entries(&mut self, entries: Vec<String>) {
        self.entries.clear();
        for entry in entries.into_iter().filter(|entry| !entry.trim().is_empty()) {
            if self.entries.last() != Some(&entry) {
                self.entries.push(entry);
            }
        }
        self.reset_navigation();
    }

    pub fn is_navigating(&self) -> bool {
        self.index.is_some()
    }

    pub fn record(&mut self, text: String) {
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

    pub fn reset_navigation(&mut self) {
        self.index = None;
        self.draft = None;
    }

    /// Move up (older, `next = false`) or down (newer, `next = true`) through
    /// history. Returns the text to display.
    pub fn navigate(&mut self, next: bool, current: &str) -> Option<String> {
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
