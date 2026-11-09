# Native macOS Context Menu for Desktop Composer (with Deferred Native NSTextView Overlay)

## 0. Executive Summary & Strategy

The desktop application's chat composer (`ComposerInput` in `crates/console-ui/src/common/input/`) is deeply integrated with rich GPUI-specific features:
- Inline file-mention pills (`@file.rs +23-35`) with atomic backspace deletion and line-range highlights.
- Floating autocompletion popovers for `@` files and `/` slash commands with exact pixel alignment.
- Custom undo/redo history (`EditHistory`) coalesced with prompt recall.
- Prompt history navigation (`Up`/`Down` arrow keys).
- Staged comment chips, screenshot attachment pills, and browser element annotations.

To **guarantee zero regression** to this carefully tuned typing and mention pipeline, we adopt a phased approach:
- **Phase 1 (Immediate / Option 1)**: Replace the current custom GPUI-rendered right-click popup with a **real native macOS `NSMenu`**.
- **Phase 2 (Subsequent / Option 2)**: Attach Touch Bar candidate suggestions (`NSCandidateListTouchBarItem`) and spellcheck queries to the GPUI window.
- **Phase 3 (Future / Option 3)**: Implement an opt-in experimental native `NSTextView` overlay behind a settings toggle (`"Use Native macOS Text View (Experimental)"`), preserving the GPUI composer as default.

---

## 1. Option 1: Native macOS Context Menu Implementation

### Current Behavior vs Target Behavior
- **Current**: Right-clicking `ComposerInput` renders a custom GPUI div popup (`primitives::menu::context_menu`) containing only 4 items: `Cut`, `Copy`, `Paste`, `Select All`. It lacks system integration.
- **Target**: Right-clicking `ComposerInput` invokes AppKit's native `NSMenu::popUpContextMenu:withEvent:forView:`.

### What the Native macOS Menu Provides
1. Standard clipboard & selection actions:
   - **Cut** (Cmd+X)
   - **Copy** (Cmd+C)
   - **Paste** (Cmd+V)
   - **Paste and Match Style** (Option+Shift+Cmd+V)
   - **Select All** (Cmd+A)
2. macOS System Features:
   - **Look Up "<selected text>"** (triggers native dictionary / Wikipedia popover)
   - **Search with Google / Default Engine**
   - **Share** submenu (AirDrop, Messages, Mail, Notes)
   - **Speech** ("Start Speaking", "Stop Speaking")
   - **Services** submenu (system-wide text automations, Shortcuts, etc.)

### Technical Architecture for Option 1
1. **Target Crates & Dependencies**:
   - `apps/desktop/crates/console-ui/Cargo.toml`
   - Enable `NSMenu`, `NSMenuItem`, `NSEvent`, `NSApplication`, `NSWindow`, `NSView` in `objc2-app-kit`.
2. **Native Menu Trigger (`crates/console-ui/src/common/input/native_menu.rs`)**:
   - On macOS (`#[cfg(target_os = "macos")]`):
     - When `ComposerInput::on_context_mouse_down` triggers:
     - Check selection and clipboard states.
     - Build an `NSMenu` containing standard editing items with selector actions (`cut:`, `copy:`, `paste:`, `selectAll:`).
     - Wire macOS standard services and share items.
     - Call `NSMenu::popUpContextMenu_withEvent_forView`.
   - On non-macOS platforms (Linux/Windows):
     - Continue using existing GPUI `context_menu(...)`.
3. **Safety & Invariants**:
   - The typing engine, mention bookkeeping, layout, and caret rendering remain 100% untouched.

---

## 2. Roadmap for Deferred Option 3 (Experimental Full Native NSTextView Overlay)

When ready to implement Option 3 in the future:
1. **Settings Toggle**: Add `"use_native_composer_input": bool` (default `false`) under Desktop settings.
2. **Dual-Backend Support**:
   - When enabled: Mount the native `NSTextView` inside an `NSScrollView` positioned under `GPUIOverlayView` via bounds syncing.
   - When disabled: Fall back immediately to the GPUI `ComposerInput`.
3. **Feature Parity Checklist Before Defaulting Option 3**:
   - Mention pills conversion to `NSTextAttachment` or attributed text spans.
   - Floating autocomplete popover positioning.
   - Keyboard command interception (`insertNewline:`, `moveUp:`, `moveDown:`).
