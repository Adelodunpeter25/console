# Modularization Plan: Desktop Input Subsystem (`crates/console-ui/src/input/`)

## 1. Overview & Motivation
The text input component in `crates/console-ui/src/input.rs` has grown to over 2,700 lines. It currently combines keybindings, undo/redo transaction history, Unicode grapheme/word boundary math, IME composition protocols, custom GPUI `InputElement` rendering, text run token styling, and mention chip tracking into a single monolithic file.

This modularization plan decomposes `input.rs` into a clean, cohesive directory module `src/input/` with zero breaking changes to external crates.

---

## 2. Target Directory Structure

```
crates/console-ui/src/input/
├── mod.rs               # Module entrypoint, ComposerInput definition & public re-exports
├── actions.rs           # GPUI action definitions & keybinding registrations
├── boundaries.rs        # Grapheme, word boundary, and UTF-16 index conversion utilities
├── history.rs           # EditHistory, EditStep, undo/redo transaction engine, PromptHistory
├── mentions.rs          # ComposerMention struct, range shifting, and reconciliation logic
├── text_runs.rs         # input_text_runs, search wash styling, mention chip quad styling
├── ime.rs               # EntityInputHandler implementation for macOS / IME marked text
└── element.rs           # Custom GPUI InputElement (request_layout, prepaint, paint)
```

---

## 3. Subsystem Breakdown & Responsibilities

### A. `actions.rs` (~150 lines)
- GPUI actions macro: `Backspace`, `Delete`, `Left`, `Right`, `Home`, `End`, `SelectAll`, `Undo`, `Redo`, `SubmitSteer`, etc.
- `pub fn init_input_keybindings(cx: &mut App)` containing all standard and macOS-specific key mappings.

### B. `boundaries.rs` (~200 lines)
- Pure Unicode text manipulation functions using `unicode_segmentation`.
- `previous_boundary`, `next_boundary`, `previous_word_boundary`, `next_word_boundary`.
- `range_to_utf16`, `range_from_utf16`, `offset_to_utf16`, `offset_from_utf16`.

### C. `history.rs` (~250 lines)
- `EditHistory`, `EditStep`, `Transaction` records with edit coalescing timeouts.
- `PromptHistory` for navigating recalled prompt drafts (Up/Down history).

### D. `mentions.rs` (~120 lines)
- `ComposerMention` data struct.
- Range adjustment math (`adjust_mentions`) across insertions and deletions.
- Reconcile validation (`reconcile_mentions`) verifying text slices against mention paths.

### E. `text_runs.rs` (~200 lines)
- `input_text_runs`: computes layout text runs with syntax highlighting, search match washes, and mention styling.
- `SearchPaint` state container.

### F. `element.rs` & `ime.rs` (~500 lines)
- `InputElement`: Custom GPUI `Element` implementing `request_layout`, `prepaint` (caret blinking / follow), and `paint`.
- `EntityInputHandler`: IME marked text, composition ranges, and input method integration.

### G. `mod.rs` (~300 lines)
- `ComposerInput` entity state and high-level methods (`replace_range`, `insert_file_mention`, `clear`, `set_content`, action dispatchers).
- Re-exports all public types so external imports (`console_ui::ComposerInput`, `console_ui::init_input_keybindings`) remain 100% backward compatible.

---

## 4. Phased Execution Steps

### Phase 0: Test Harness & Backward Compatibility Baseline (Pre-Refactor)
Before moving or refactoring any code, build comprehensive unit tests in dedicated files under `crates/console-ui/tests/input_test.rs` covering all core input logic against the current implementation:
1. **Word & Grapheme Boundaries**:
   - Single and multi-byte UTF-8, emoji, alphanumeric, punctuation, whitespace, and newline boundaries.
   - `previous_boundary`, `next_boundary`, `previous_word_boundary`, `next_word_boundary`.
2. **UTF-16 / UTF-8 Offset Conversions**:
   - Ascii text, multi-byte sequences, surrogate pairs (emoji), and out-of-bounds clamps.
3. **Undo/Redo & Edit History**:
   - Sequential character typing, typing coalescing within debounce interval, backspace coalescing.
   - Distinct undo boundaries on programmatic insertion / paste / replace.
   - Sealed transactions and composition finalization.
4. **Prompt History Navigation**:
   - Navigating Up/Down through submitted prompts, draft preservation when navigating back to present, reset behavior on edit.
5. **Mention Chip Range Math**:
   - Insertion, shifting on prefix edits, middle edits, suffix edits, invalidation on interior mutation, atomic deletion ranges.
6. **Execution**:
   - Run `cargo test --package console-ui --test input_test` and ensure 100% pass rate before touching source files.

### Phase 1: Pure Utilities & History Extraction
- Extract `boundaries.rs`, `history.rs`, `mentions.rs`, and `actions.rs` from `input.rs` into `src/input/`.
- Re-run `cargo test --package console-ui --test input_test` to confirm all pure helpers pass with zero regressions.

### Phase 2: Text Runs & Element Split
- Extract `text_runs.rs` and `element.rs` / `ime.rs`.
- Place `ComposerInput` in `mod.rs` and re-export all public types.
- Re-run full test suite and `cargo check`.

### Phase 3: Final Verification & Smoke Testing
- Run all tests: `cargo test --package console-ui`.
- Run desktop build check: `cargo check --manifest-path apps/desktop/Cargo.toml`.
- Manual verification in desktop app: typing, copy/paste, undo/redo, file chip autocompletion, search box input.

---

## 5. Verification Checklist
- [ ] Phase 0 test harness (`input_test.rs`) created and passing before any file split.
- [ ] No external files require import path adjustments (`use console_ui::ComposerInput` remains unchanged).
- [ ] All unit tests in `crates/console-ui/tests/` pass.
- [ ] Zero regressions in composer typing, mention chips, undo/redo, or IME input.
