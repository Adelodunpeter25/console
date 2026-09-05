# Composer file-mention pills — desktop spec

## Goal

Give the desktop composer the inline `@file` pill from the screenshot, without
repeating its mistake. The screenshot shows a clean atomic pill (icon + name,
one-backspace-to-delete) but hides the path: `roadmap.md` alone is ambiguous
the moment two files share a basename (`index.ts`, `route.ts`, `page.tsx`).
The desktop pill must keep the **full relative path visible inline**, styled as
a pill, so what you see is what the agent receives.

Wire format stays `@relative/path` plain text end-to-end. The pill is
presentation only. No backend change.

## Non-goals

- No inline widget / embedded icon elements inside the text flow for v1. The
  composer is a custom GPUI text field (`crates/console-ui/src/input.rs`,
  `ComposerInput`, ~2500 lines: one `SharedString`, custom `TextLayout`,
  caret, mouse, undo). True inline widgets (icon + separate layout) would mean
  custom shaping, token-aware caret/selection/copy-paste on top of that. Not
  worth it now.
- No change to what is sent: `AutocompleteItem::File::insert_text()`
  (`crates/console-ui/src/common/autocomplete.rs`) keeps inserting
  `@relative/path `, and the server keeps expanding it in
  `apps/server/api/src/services/assist.service.ts` (`expandPromptRefs`,
  `/(^|\s)@([^\s@]+)/g` resolved against the session cwd, left as `@raw` when
  the file does not exist).
- No mobile work in this spec (native `TextInput` cannot host inline pills;
  mobile gets a mention chip strip above the input — separate spec).
- No existence / stale-file error state in v1 (see Phase 3).

## Current state (where things live)

- Trigger + popup: `detect_trigger` / `filter_items` / `AutocompleteView` in
  `crates/console-ui/src/common/autocomplete.rs`. File rows already render the
  full `relativePath` with a file-type icon — disambiguation already happens
  here. Insert path: `accept_autocomplete_for_pane`
  (`apps/desktop/src/state/autocomplete.rs`) → `ComposerInput::replace_range`.
- Painting: `InputElement::request_layout` builds a `StyledText` from the raw
  content via `input_text_runs` (`input.rs:~2160-2235`), layering selection,
  IME mark, syntax `highlight`, and find-match washes (`SearchPaint`,
  `search_matches` / `active_search_match`, set via `set_search_matches`).
  This is the pattern to copy for mentions.
- Deletion: `ComposerInput::backspace` (`input.rs:1313`) deletes one
  grapheme/word boundary; no token awareness.
- Mouse: `index_for_mouse_position` (`input.rs:1626`) already maps a point to
  a byte offset (used for click/drag selection).
- Tooltip primitive exists (`crates/console-ui/src/primitives/tooltip.rs`,
  used via `InteractiveElement::tooltip` in `markdown/render.rs`), but
  `InputElement` is a raw `Element`, not an `InteractiveElement`, so it cannot
  `.tooltip()` directly — hover UI needs parent-level handling (see Phase 2).
- Related work: clickable transcript paths (`docs/plan/clickable-file-paths-plan.md`,
  `crates/console-ui/src/markdown/file_links.rs` with `is_file_link` /
  `resolve_file_link`) covers opening files from chat, not the composer.

## UX spec

### V1 — painted pill, full path inline

- Any `@token` in the draft that parses as a file mention (grammar below)
  paints as a pill: rounded-rect background wash + accent/foreground text
  color, exactly like the find-match wash but with pill colors. The text
  itself stays `@relative/path` — full path, never basename-only.
- Pill participates in wrapping/shaping like normal text (it *is* normal text
  with a background run; `TextRun.background_color` already exists).
- Selection wash wins over the pill wash when the range is selected (same
  precedence rule as search matches: active > selection > match > none).
- Placeholder (empty composer) never shows pills.
- Autocomplete popup is unchanged in v1 except it must keep showing the full
  `relativePath` per row (it already does). Do not "simplify" rows to
  basenames.

### V2 — atomic deletion + hover identity

- **Atomic backspace:** with an empty selection and the caret immediately
  after a mention range, the first Backspace selects the whole `@path` range
  instead of deleting one char; the second deletes it. Same for Forward Delete
  at the range start. Word-delete (`alt-backspace`) and cut/copy/selection
  behave as today (no special-casing beyond the single-backspace rule).
- **Hover:** hovering a mention shows the resolved context — full relative
  path plus the resolved absolute path (session cwd → pane project fallback,
  same resolution as `resolve_file_link`) and, when known, a stale/missing
  indicator. Because `InputElement` cannot `.tooltip()`, the hovered mention
  is tracked in `ComposerInput` via mouse-move → `index_for_mouse_position` →
  range hit-test, exposed via a getter, and the parent `ComposerView` renders
  the `Tooltip` (deferred/anchored, like the autocomplete popup).
- **Autocomplete disambiguation:** when two visible rows share a basename,
  emphasize the differing parent directory (muted prefix + prominent
  basename). Full path must always remain visible; this only re-weights it.

### Explicitly not copying from the screenshot

- Basename-only pill text. Ours always shows the relative path inline, so a
  pill can never silently point at the wrong duplicate.
- The `M+` glyph. Our pill carries no fake icon inside the text (no inline
  widgets in v1); identity comes from the pill wash + full path text. A real
  file-type glyph inside the text flow waits on inline-widget support, if ever.

## Architecture

### Mention grammar (single source of truth)

New module, e.g. `crates/console-ui/src/common/composer_mentions.rs`:

```rust
pub fn parse_file_mentions(content: &str) -> Vec<Range<usize>>
```

- Mirror the server contract: `(^|\s)@([^\s@]+)` over byte offsets, char-
  boundary safe, same as `detect_trigger`'s whitespace/`@` rule so the popup
  trigger and the pill agree on what a mention is.
- Token charset: `detect_trigger::valid_file_character` (alnum + `_-. /`);
  strip a single trailing `? ! . , : ;` that is sentence punctuation, not path.
  Keep `:line:col` suffixes inside the range for v1 (server leaves them in
  `@raw`; opening-at-line is already deferred in the clickable-paths plan).
- The trailing space the autocomplete appends (`@path␣`) is outside the range.
- Pure function over `&str`, no fs, no cx — unit-testable in `console-ui`.

### Paint path (v1)

Follow the `search_matches` precedent exactly:

1. `ComposerInput` gains `mentions: Vec<Range<usize>>`, recomputed in
   `refresh_highlight`-adjacent content-change paths (every place that
   touches `content`: typing, `replace_range` from autocomplete accept, undo/
   redo, cut/paste). Composer drafts are short; a linear scan per edit is
   free. Empty content clears it.
2. `input_text_runs` gains a `MentionPaint` argument (parallel to
   `SearchPaint`): each mention range contributes its boundaries to the
   boundary splitter, and windows fully covered by a mention get the pill
   `background_color` + mention text color. Precedence: selection wash beats
   pill; pill beats nothing else (composer has no syntax language, so no
   `highlight` conflict; find matches and mentions may overlap — find wins,
   same as active-match-wins today).
3. `request_layout` threads `&input.mentions` through, with pill colors from
   `Theme` (new theme keys, e.g. `mention_bg` / `mention_text`, or reuse
   `theme.accent` at low opacity + `theme.text` — match the app's existing
   wash style, do not invent a new palette).
4. No change to `prepaint`/`paint`, caret, scrolling, or `last_layout`.

### Atomic deletion (v2)

- In `backspace()`: before the default `previous_boundary` path, if the
  selection is empty and the caret sits exactly at a mention range end,
  `select_to(range.start)` and return (the selection now covers the pill;
  the next Backspace deletes it normally). Mirror in `delete()` for caret at
  range start. Guard with `FieldMode::Composer` only; search/code fields
  unaffected.
- Undo coalescing (`EditHistory`) already treats completion inserts as sealed
  steps; the two-step select-then-delete naturally undoes as select + delete,
  which is the accepted chat-composer idiom. No history changes.

### Hover tooltip (v2)

- `ComposerInput` tracks `hovered_mention: Option<Range<usize>>` on mouse-move
  (hit-test via `index_for_mouse_position` against `last_layout`), cleared on
  mouse-out / content change. Expose `hovered_mention()` + the token text.
- `ComposerView` (which *is* an element tree that can anchor popups) renders a
  deferred `Tooltip` when the composer reports a hovered mention, showing
  `relative path` + resolved absolute path. Resolution reuses
  `markdown::file_links::resolve_file_link` with the pane's session cwd /
  project path — the same sources the transcript open-file path uses.
- Existence coloring is deferred: resolving "does this file exist" from the
  desktop requires a round-trip through the server fs service (the session cwd
  is a server-side path; the desktop may be remote). V2 tooltip shows the
  resolved path without a live existence claim. A stale/missing error state
  becomes its own phase once a cheap availability channel exists (candidate:
  reuse the file-search backend to validate visible mentions debounced, or
  surface the server's send-time `expandPromptRefs` fallback). Do not block
  v1 on this.

## Implementation phases

### Phase 1 — painted pill (ship it)

1. Add `composer_mentions.rs` with `parse_file_mentions` + unit tests
   (mention at start/middle/end, multiline, email `a@b` ignored, `@` alone
   ignored, trailing-space/punctuation handling, duplicate basenames each
   detected, char-boundary safety). Tests live in `crates/console-ui/tests/`
   per repo rules (never inline `#[cfg(test)]` in source).
2. Add `mentions` field to `ComposerInput`, recompute on every content
   mutation, clear on empty.
3. Thread `MentionPaint` through `input_text_runs` with selection-wins
   precedence; add theme colors.
4. Verify: type `@`, pick a file, pill paints over `@relative/path`; keep
   typing — ranges track; send — backend receives unchanged `@path` text;
   existing `input`/`autocomplete` tests + `cargo check` pass.

### Phase 2 — atomic delete + hover + popup emphasis

1. Backspace/Delete token awareness in `ComposerInput` (composer mode only).
2. Hovered-mention tracking + parent-rendered `Tooltip` with
   relative + resolved absolute path (reuse `resolve_file_link`).
3. Autocomplete row emphasis for colliding basenames (full path stays).
4. Verify: backspace selects whole pill first; hover shows full/resolved
   path; duplicate-basename rows visibly differ; no regression in caret,
   selection, undo, IME, or the autocomplete keyboard flow.

### Phase 3 (future, not this spec's ship bar)

- Live stale/missing state for mentions (needs a server-validated
  availability channel; the send-time `expandPromptRefs` fallback is the
  backstop until then).
- True inline widgets (file-type glyph inside the text) — only if GPUI gains
  cheap embedded-element support in the custom field; re-evaluate then.

## Verification

- Unit: `parse_file_mentions` cases above, all in `crates/console-ui/tests/`.
- Manual desktop: empty-`@` shows recents; pick `apps/server/index.ts`-style
  deep path — pill spans the whole `@path`, wraps correctly, caret moves
  through it char-by-char (v1) / backspace selects it whole (v2); two same-
  basename files are distinguishable both inline and in the popup; send
  delivers `@relative/path` text unchanged (confirm against server logs).
- Regression: `cargo check` (workspace), existing console-ui tests, autocomplete
  keyboard flow (up/down/enter/tab/esc), IME composition, undo/redo, find
  washes in file editors (shared `input_text_runs` — editors must look
  identical when no mentions exist).
- Per repo rules: run only the touched test target, then commit with a
  single-line message staging only the touched files.

## Risks / open questions

- `input_text_runs` is shared with code/search fields: the mention layer must
  be a no-op when `mentions` is empty (it always is outside composer mode),
  or file-editor washes regress.
- Hover tooltip on a raw `Element` is the fiddliest part of Phase 2; if
  anchoring fights the capped composer viewport, ship Phase 1 + atomic delete
  first and let hover follow — the full-path-inline rule means hover is an
  enhancement, not a disambiguation requirement.
- Long duplicate paths can overflow the pill visually; truncation is the
  renderer's existing `truncate` behavior — acceptable, since the full token
  is still the underlying text and the tooltip (v2) shows it whole.
