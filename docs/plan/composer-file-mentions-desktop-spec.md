# Desktop composer file chips

## Goal

When a user chooses a file from the desktop composer’s `@` autocomplete, show the
selection as a compact file chip, like the reference UI:

```text
┌──────────────────────────────┐  |
│  [file icon]  roadmap.md     │  |
└──────────────────────────────┘
```

The chip is a visual editing affordance. The composer continues to store and send
plain text, for example `@roadmap.md`. This is a desktop-only UI change.

## Scope

### In scope

- Render an accepted file mention as a chip inside the desktop composer.
- Show a file-type icon and the selected file’s relative path or filename.
- Keep the existing `@relative/path` text as the source of truth.
- Treat the chip as one item when it is deleted with Backspace or Delete.
- Preserve normal typing, cursor movement, selection, copy, paste, undo, redo, and
  send behavior.

### Out of scope

- No server, API, wire-format, or prompt-expansion changes.
- No filesystem validation, stale-file state, or file-existence requests.
- No tooltip or resolved-path display.
- No mobile changes.
- No changes to autocomplete search, ranking, or result contents.
- No basename-only replacement in the underlying text. The existing relative path
  remains available to the agent even if the chip display is shortened.
- No true embedded GPUI widget or rich-text document model. The current custom text
  field remains the editor.

## Existing behavior

The relevant flow already exists:

1. `detect_trigger`, `filter_items`, and `AutocompleteView` in
   `crates/console-ui/src/common/autocomplete.rs` show file suggestions.
2. Accepting a file calls `accept_autocomplete_for_pane` in
   `apps/desktop/src/state/autocomplete.rs`.
3. `AutocompleteItem::File::insert_text()` inserts `@relative/path `.
4. The text is stored in `ComposerInput` and sent as ordinary composer content.
5. `InputElement::request_layout` builds the displayed text using `StyledText` and
   `input_text_runs` in `crates/console-ui/src/input.rs`.

The implementation should build on this flow rather than change the message
payload. Lightweight UI metadata for accepted chips is allowed; it must never be
serialized or sent separately from the composer text.

## User experience

### Selecting a file

- Typing `@` continues to open the existing file autocomplete popup.
- Selecting a result inserts the same text as today: `@relative/path `.
- The inserted `@relative/path` portion is displayed as a chip.
- The separator space inserted after the mention is not part of the chip.
- The caret remains after the separator, ready for continued typing.

### Chip appearance

- The chip has a subtle rounded background and border matching the existing desktop
  theme.
- It contains the existing file-type icon used by autocomplete file rows, followed
  by the file path text.
- The visible label should prefer the filename when space is constrained, but the
  complete relative path must remain the underlying text and must be recoverable by
  normal copy/select behavior. If truncation is needed, use the composer’s existing
  text overflow behavior rather than changing the stored value.
- The chip should remain legible in focused, unfocused, dark, and light themes.
- Multiple chips in one draft should be visually distinct and separated by the
  existing text spacing.
- A chip that wraps at the composer boundary must not corrupt the underlying text.
  If the current text layout cannot keep a chip together, a rectangular highlight
  for that wrapped segment is acceptable for v1; do not redesign the text editor.

### Editing and deletion

- The chip is presentation-only while the caret is moving: the underlying content is
  still plain text and cursor offsets remain byte offsets into that content.
- Backspace immediately after a chip deletes the entire mention in one action.
- Forward Delete immediately before a chip deletes the entire mention in one action.
- If the caret is after the autocomplete-added separator, the first Backspace keeps
  its current normal behavior and removes the separator. The next Backspace removes
  the chip. This preserves the existing typing flow and avoids treating ordinary
  whitespace as part of the chip.
- A partial selection behaves like normal text selection. Replacing or deleting a
  selection removes exactly the selected text; the remaining mention range is
  recomputed afterward.
- Copying a chip copies its plain text representation, including the `@` prefix.
- Undo and redo operate on the existing text history and restore the visual chip
  automatically from the resulting content.

### Manually typed references

V1 only needs to chip mentions created by accepting a file autocomplete result. A
manually typed `@path` may remain ordinary text. If the implementation instead
chooses to paint all valid `@path` tokens, that is acceptable, but it must not change
what is sent and must use the same range/deletion behavior consistently.

## Technical approach

### Track chip ranges

Add lightweight chip metadata to `ComposerInput`, for example:

```rust
struct ComposerMention {
    range: Range<usize>,
    path: String,
    icon: FileIcon,
}
```

The range covers only `@relative/path`, not the trailing separator space.

When autocomplete inserts a file, pass the inserted range or enough information to
`ComposerInput` to register the chip. Do not infer identity from the server or query
the filesystem.

After any content mutation, adjust or recompute the ranges so they remain valid:

- autocomplete insertion
- normal typing
- backspace/delete
- cut/paste
- undo/redo
- IME commit
- programmatic replacement

A small linear scan is sufficient for composer drafts. If the implementation stores
only ranges rather than metadata, the icon can be derived from the inserted file
result at creation time and the path from the underlying content.

### Paint the chip

Extend the existing `input_text_runs` / `StyledText` path or add a nearby paint step.
The visual layer should:

1. Keep the source string unchanged.
2. Split text runs at chip boundaries.
3. Paint the chip background and border behind the mention range.
4. Paint the file icon at the leading edge of the range.
5. Paint the mention text using the normal composer text style.

Selection styling must remain visible when a chip is selected. The chip layer must be
empty for non-composer fields so shared editor/search inputs do not change.

If `TextRun.background_color` cannot provide rounded corners, use the text layout’s
range bounds to paint a rounded background separately. Do not introduce inline
widgets in v1 just to place the icon.

### Chip-aware deletion

In `ComposerInput::backspace` and `ComposerInput::delete`, before falling back to the
existing grapheme-boundary behavior:

- If there is no selection and the caret is at a chip end, replace the chip range with
  an empty string.
- If there is no selection and the caret is at a chip start, replace the chip range
  with an empty string.
- Keep the behavior limited to `FieldMode::Composer`.
- Do not include the trailing separator in the chip range.

After deletion, update the content-derived chip state and keep the caret at the
normal replacement position.

## Implementation phases

### Phase 1 — visual chip

1. Add chip metadata/range tracking to `ComposerInput`.
2. Register the range when a file autocomplete item is accepted.
3. Paint the themed chip background, border, icon, and path text.
4. Recompute or adjust chip ranges after all content mutations.
5. Confirm the sent composer value is unchanged.

### Phase 2 — atomic deletion

1. Add Backspace behavior at a chip end.
2. Add Forward Delete behavior at a chip start.
3. Cover selections, adjacent chips, and the autocomplete-added separator.
4. Confirm undo/redo and cursor behavior remain unchanged.

### Phase 3 — polish, if needed

- Improve long-path display and wrapping.
- Add manually typed mention recognition if it is useful.
- Refine icon and theme colors based on desktop review.

## Verification

### Unit and focused tests

Add tests in the existing dedicated `crates/console-ui/tests/` test location where
appropriate. Cover:

- A file selection creates a range covering exactly `@relative/path`.
- The trailing separator is outside the range.
- Multiple file selections produce independent ranges.
- Ranges remain valid after typing, replacement, paste, undo, and redo.
- Backspace after a chip deletes the chip.
- Forward Delete before a chip deletes the chip.
- Backspace after the separator first deletes only the separator.
- Partial selection deletes only the selected text.
- Non-composer fields have no chip styling or chip deletion behavior.
- Copy and send preserve the plain `@relative/path` text.

### Manual desktop check

- Type `@` and choose `roadmap.md`.
- Confirm the chip visually matches the reference: icon, compact rounded container,
  filename/path, and visible caret after it.
- Continue typing after the chip.
- Add two or more file chips and confirm they remain distinct.
- Select, copy, paste, undo, redo, and delete around a chip.
- Confirm the outgoing prompt still contains the original `@relative/path` text.
- Check dark/light themes and a narrow composer width.

Run only the focused desktop/console-ui test target affected by the implementation,
plus the relevant Rust check. Commit only the files changed for this feature with a
single-line commit message.

## Success criteria

The feature is complete when a selected file looks like a compact file chip in the
desktop composer, can be removed as one item, and remains ordinary `@relative/path`
text for editing, copying, and sending. No server or API behavior changes are
required or expected.
