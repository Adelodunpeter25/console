# Native macOS Text Input for Desktop Composer

## 0. Problem Statement & Motivation

On macOS (especially MacBook Pro models with a Touch Bar such as the 2020 13"/16" MacBook Pro):
1. **No Touch Bar QuickType Suggestions**: Typing in the chat composer does not trigger native AppKit Touch Bar text suggestions or word completions.
2. **No Autocorrect & Continuous Spellcheck**: Misspelled words do not get red underline markers, and the native macOS autocorrect popup bubble does not appear.
3. **No Native macOS Context Menu**: Right-clicking the composer displays a minimal custom GPUI popup (Cut, Copy, Paste, Select All) instead of the native macOS `NSMenu` with Look Up, Spelling and Grammar, Substitutions, Transformations, Speech, and Services.

This happens because the desktop app is built with Zed's GPUI framework in Rust, which renders text into GPU quads using custom text-layout logic. While GPUI connects to `NSTextInputClient` for basic IME (CJK and accent dead keys), it does not connect with `NSSpellChecker`, `NSTouchBarProvider`, or native AppKit context menus.

The desktop app already successfully embeds native macOS views into GPUI for the browser and device tabs (`WKWebView` via `objc2-app-kit` in `apps/desktop/crates/console-ui/src/browser/host.rs` and `native_utils.rs`). We use this exact proven architecture to embed a native AppKit `NSTextView` for the chat composer.

---

## 1. Goals & Non-Goals

### Goals
- **Native Touch Bar Integration**: Full QuickType word completions and candidate suggestions on Apple Touch Bar keyboards out of the box.
- **Native Autocorrect & Spellcheck**: Automatic spell checking while typing, red squigglies, inline correction bubbles, smart quotes, smart dashes, and text replacement.
- **Native macOS Context Menu**: Right-click opens the standard AppKit `NSMenu` including Spelling and Grammar, Substitutions, Transformations, Look Up, Search with Google, Share, Speech, and Services.
- **Seamless Chat Integration**:
  - `Enter` submits the prompt; `Shift+Enter` inserts a newline.
  - `Up` and `Down` arrows at the edges navigate prompt history.
  - `@` file mentions and `/` slash commands detect triggers and synchronize with GPUI's autocomplete popover.
  - Staged comments, attachments, and annotations remain supported.
- **Clean Cross-Platform Fallback**: Linux and Windows continue using the existing pure GPUI `ComposerInput` without changes or regressions.

### Non-Goals
- Re-architecting small single-line utility fields (e.g., search bars, rename fields, port inputs). These remain pure GPUI fields where multiline autocorrect and Touch Bar QuickType are not relevant.
- Rewriting GPUI's core text engine upstream.

---

## 2. Architecture & Design

### 2.1 View Hierarchy & Z-Ordering
In macOS GPUI, the window is backed by an `NSWindow` containing GPUI's Metal render surface as its root `NSView`.
- We create an AppKit `NSScrollView` containing a configured `NSTextView`.
- The `NSScrollView` is added as a subview of the window's content view.
- We call `lower_below_scene_overlay(&scroll_view)` (from `native_utils.rs`), which orders the native view below `GPUIOverlayView`. This ensures that all floating GPUI elements (autocomplete popovers, model selection menus, file mention pickers, tooltips, and dialogs) paint cleanly **above** the native text field.

### 2.2 AppKit View Configuration
```rust
// NSTextView configuration
text_view.setContinuousSpellCheckingEnabled(true);
text_view.setAutomaticSpellingCorrectionEnabled(true);
text_view.setAutomaticQuoteSubstitutionEnabled(true);
text_view.setAutomaticDashSubstitutionEnabled(true);
text_view.setAutomaticTextReplacementEnabled(true);
text_view.setDrawsBackground(false);
text_view.setBackgroundColor(NSColor::clearColor());
```
- **Typography & Theme**: The `NSFont` is set to the system font at 13.5pt (matching GPUI's `px(13.5)`), and `textColor` tracks the active GPUI theme's primary text color (dark/light mode).
- **Placeholder**: When text is empty, a themed placeholder string is drawn or layered underneath.

### 2.3 Keyboard & Command Handling (`NSTextViewDelegate`)
We implement a custom delegate using `objc2::define_class!` conforming to `NSTextViewDelegate`:
1. **`textView:doCommandBySelector:`**:
   - `insertNewline:`:
     - Check `NSApplication::sharedApplication().currentEvent().modifierFlags()`.
     - If Shift, Option, or Ctrl is pressed: pass through to `insertNewlineIgnoringFieldEditor:`.
     - If no modifier: trigger GPUI prompt submission callback.
   - `moveUp:` / `moveDown:`:
     - If autocomplete popover is open: route event to autocomplete selection (previous/next).
     - If at boundary (offset 0 for up, end for down): trigger GPUI prompt history navigation.
   - `cancelOperation:` (Esc):
     - If autocomplete popover is open: dismiss autocomplete.
   - `insertTab:`:
     - If autocomplete popover is open: confirm current autocomplete item.
2. **`textDidChange:`**:
   - Notify GPUI of updated content.
   - Check cursor position and trailing token for `@` and `/` triggers, opening or updating GPUI's autocomplete popup.
   - Recompute required content height (`usedRectForTextContainer`) and notify GPUI to dynamically expand the composer height from 24px up to `COMPOSER_MAX_HEIGHT` (280px).

### 2.4 Lifecycle & Visibility Synchronization
- In `ComposerView::render`:
  - An element prepaint callback (`on_prepaint`) measures the layout bounds of the composer input container and invokes `host.sync_bounds(bounds, scale_factor)`.
  - When the workspace pane or tab is hidden, or the window is inactive, `host.set_visible(false)` hides the `NSScrollView`.
  - When switching panes or restoring drafts, `host.set_text(content)` keeps native text in sync with GPUI state.

---

## 3. Implementation Steps

### Phase 1: Cargo Dependencies & AppKit Feature Flags
- Update `apps/desktop/crates/console-ui/Cargo.toml` to enable necessary `objc2-app-kit` features:
  - `NSTextView`, `NSScrollView`, `NSTextContainer`, `NSLayoutManager`, `NSTextStorage`, `NSClipView`, `NSColor`, `NSFont`, `NSMenu`, `NSMenuItem`, `NSText`, `NSEvent`, `NSResponder`.

### Phase 2: Native Text Host Module (`console-ui/src/common/input/native_host.rs`)
- Create `NativeTextHost` (macOS-only) encapsulating:
  - `NSScrollView` + `NSTextView` instantiation and configuration.
  - Sizing, bounds syncing (`sync_bounds`), and visibility management.
  - Custom `NativeTextDelegate` implementing `NSTextViewDelegate` via `objc2::define_class!`.
  - Callbacks into GPUI: `on_change`, `on_submit`, `on_navigate_history`, `on_autocomplete_action`.
  - Fallback stub for non-macOS platforms.

### Phase 3: Bridge Native Text Host with `ComposerInput` / `ComposerView`
- Add an optional native host handle to `ComposerInput` / `ComposerView`.
- In `ComposerView::render`:
  - On macOS: render the layout container for the native text view with `on_prepaint` bounds syncing, and attach `NativeTextHost`.
  - On Linux/Windows: render `InputElement` with pure GPUI rendering as before.

### Phase 4: Autocomplete & Mention Synchronization
- Forward `@` mention inserts and `/` slash command inserts from GPUI's autocomplete popovers directly into `NSTextView` via `replaceCharactersInRange:withString:`.
- Forward staged comment removal or attachment clear events to update the text field.

### Phase 5: Verification & Polish
- Verify Touch Bar QuickType suggestion behavior on 2020 MacBook Pro.
- Verify native context menu actions (Look Up, Spelling, Substitutions).
- Verify dark/light theme switching dynamically updates font color and cursor.
- Verify multiline auto-expand and scrollbar activation at 280px cap.
- Verify clean compilation on both macOS and Linux.
