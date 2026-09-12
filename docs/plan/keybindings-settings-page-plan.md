# Plan: Keybindings & Keyboard Shortcuts Settings Page

## 1. Overview & Goal

As Console grows with features across Chat, Code Viewing, Terminal, Browser, and Subagents, the list of keyboard shortcuts has expanded significantly.

This document defines the plan for adding a dedicated, read-only **"Keyboard Shortcuts"** page inside the Console Desktop Settings window (`apps/desktop/src/settings_window.rs` and `apps/desktop/crates/console-ui/src/settings/keybindings_page.rs`), reflecting the exact action names and bindings registered in code.

> **Scope Boundary**:
> - **In Scope**: A clean, comprehensive, categorized, searchable catalog of all existing keybindings.
> - **Out of Scope (Deferred)**: Custom keybinding remapping and user shortcut overrides.

## 2. UI & Interaction Design

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Settings                                                                                    │
├─────────────────┬───────────────────────────────────────────────────────────────────────────┤
│ • Accounts      │ Keyboard Shortcuts                                                        │
│ • Models        │ ┌───────────────────────────────────────────────────────────────────────┐ │
│ • Projects      │ │ Filter shortcuts (e.g., "browser", "composer", "cmd+r", "switch")... │ │
│ • Environments  │ └───────────────────────────────────────────────────────────────────────┘ │
│ • Connection    │                                                                           │
│ • Usage         │ ── Global & Window (apps/desktop/src/keybindings.rs) ─────────────────── │
│ • Keybindings ◄ │ Open Command Palette (`ToggleCommandPalette`)         [ ⌘ ] [ K ]         │
│ • Deleted Chats │ Quick Open File (`QuickOpenFile`)                     [ ⌘ ] [ P ]         │
│                 │ Open Project Browser (`AddProject`)                   [ ⌘ ] [ O ]         │
│                 │ New Desktop Window (`NewWindow`)                      [ ⇧ ] [ ⌘ ] [ N ]   │
│                 │ New Chat Session (`NewChat`)                          [ ⌘ ] [ N ]         │
│                 │ Close Active Tab (`CloseTab`)                         [ ⌘ ] [ W ]         │
│                 │ Toggle Left Sidebar (`ToggleLeftSidebar`)             [ ⌘ ] [ B ]         │
│                 │ Toggle Right Sidebar (`ToggleRightSidebar`)           [ ⇧ ] [ ⌘ ] [ B ]   │
│                 │ Open Settings Window (`OpenSettings`)                 [ ⌘ ] [ , ]         │
│                 │ Focus Composer Input (`FocusComposer`)                [ ⌘ ] [ L ] *       │
│                 │ Toggle Model Picker (`ToggleModelPicker`)             [ ⌘ ] [ / ]         │
│                 │ Focus Model Search (`FocusModelSearch`)               [ / ] (menu open)   │
│                 │ Cycle Approval Mode (`CycleApprovalMode`)             [ ⇧ ] [ Tab ] **    │
│                 │ Switch Session 1–9 (`SwitchSession1..9`)              [ ⌘ ] [ 1..9 ]      │
│                 │ Switch Workspace Tab 1–9 (`SwitchWorkspaceTab1..9`)   [ ⌥ ] [ 1..9 ]      │
│                 │                                                                           │
│                 │ ── Chat & Composer (common/input/actions.rs) ─────────────────────────── │
│                 │ Submit Prompt / Send (`Enter`)                        [ Enter ]           │
│                 │ Insert Newline (`Newline`)                            [ ⇧ ] [ Enter ]     │
│                 │ Submit Steer / Queue Prompt (`SubmitSteer`)           [ ⌘ ] [ Enter ]     │
│                 │ Select All / Copy / Cut / Paste                       [ ⌘ ] [ A/C/X/V ]   │
│                 │ Undo / Redo                                           [ ⌘ ] [ Z ] / [ ⇧⌘Z]│
│                 │ Text navigation (words, line ends, macOS Emacs keys)  see details         │
│                 │                                                                           │
│                 │ ── Autocomplete (common/autocomplete.rs) ──────────────────────────────── │
│                 │ Next / Previous suggestion                            [ ↓ ] / [ ↑ ]       │
│                 │ Confirm Autocomplete (`AutocompleteConfirm`)          [ Tab ] / [ Enter ] │
│                 │ Dismiss Autocomplete (`AutocompleteDismiss`)          [ Esc ]             │
│                 │ Trigger File Mention                                  [ @ ]               │
│                 │ Trigger Slash Command                                 [ / ]               │
│                 │                                                                           │
│                 │ ── Browser Surface (browser/actions.rs) ───────────────────────────────── │
│                 │ Focus Address Bar (`FocusBrowserAddress`)             [ ⌘ ] [ L ] *       │
│                 │ Reload Page (`BrowserReload`)                         [ ⌘ ] [ R ]         │
│                 │ Hard Reload (`BrowserHardReload`)                     [ ⇧ ] [ ⌘ ] [ R ]   │
│                 │ Navigate Back (`BrowserBack`)                         [ ⌘ ] [ [ ]         │
│                 │ Navigate Forward (`BrowserForward`)                   [ ⌘ ] [ ] ]         │
│                 │ Stop Loading (`BrowserStop`)                          [ Esc ]             │
│                 │ Toggle Web Inspector / DevTools (`BrowserDevtools`)   [ ⇧ ] [ ⌘ ] [ I ]   │
│                 │ Cancel Address Edit (`BrowserAddressCancel`)          [ Esc ]             │
│                 │ Copy / Cut / Paste / Select All in webview            [ ⌘ ] [ C/X/V/A ]   │
│                 │                                                                           │
│                 │ ── Terminal & Code Viewer ─────────────────────────────────────────────── │
│                 │ Terminal Tab / Shift+Tab to PTY (`TerminalTab`)       [ Tab ] / [ ⇧Tab ]  │
│                 │ Copy Selection / Select All (`CopySelection`)         [ ⌘ ] [ C ] / [ A ] │
│                 │                                                                           │
│                 │ * `⌘L` is context-scoped: composer vs browser address bar win by focus.   │
│                 │ ** `Shift+Tab` cycles approval mode only in composer focus context.       │
└─────────────────┴───────────────────────────────────────────────────────────────────────────┘
```

## 3. Data Model from Codebase Sources

Verified against code on 2026-09-12. All paths rooted at `apps/desktop/`:

1. `src/keybindings.rs` — `init()` lines 74-128 (global window actions, `secondary` = Cmd on macOS / Ctrl elsewhere, `alt` = Option/Alt).
2. `crates/console-ui/src/browser/actions.rs` — `init_browser_keybindings()` lines 28-61 (`Browser` + `BrowserAddress` contexts, plus `WebviewCopy/Cut/Paste/SelectAll`).
3. `crates/console-ui/src/terminal/actions.rs` — `init_terminal_keybindings()` lines 15-19 (only `TerminalTab`, `TerminalShiftTab` in `Terminal` context; there are NO `NewTerminal` / `CloseActiveTerminal` / `NextTerminalTab` / `ToggleTerminalBottom` / `ClearTerminal` actions).
4. `crates/console-ui/src/viewer/code_viewer.rs` — `init_code_viewer_keybindings()` lines 40-45 (only `CopySelection`, `SelectAll` in `CodeViewer` context; there is NO find-bar binding).
5. `crates/console-ui/src/common/input/actions.rs` — `init_input_keybindings()` lines 42-123 (`ComposerInput` context: `Enter`, `Newline` (Shift+Enter), `SubmitSteer` (secondary+Enter), editing, clipboard, undo/redo, word/line navigation + macOS Emacs keys).
6. `crates/console-ui/src/common/autocomplete.rs` — `init()` lines 36-53 (`ComposerAutocomplete > ComposerInput` context: `AutocompleteNext/Previous` (Up/Down), `AutocompleteConfirm` (Tab/Enter), `AutocompleteDismiss` (Escape)).

Corrections vs the previous draft of this plan:
- `ToggleRightSidebar` is `secondary-shift-b` (Cmd+Shift+B), NOT Cmd+J.
- Removed invented `NewTerminal` / `CloseActiveTerminal` / `Next/PrevTerminalTab` / `ToggleTerminalBottom` / `ClearTerminal` / `ToggleSearch` / `NextMatch` / `PrevMatch` / `CloseSearch` entries — none exist in code.
- Added missing `WebviewCopy/Cut/Paste/SelectAll`, autocomplete bindings, and composer editing bindings.
- Fixed crate paths (`apps/desktop/crates/console-ui/...`, not `crates/console-ui/...`).

```rust
pub struct KeybindingEntry {
    pub action_name: &'static str,
    pub description: &'static str,
    pub macos_keys: &'static [&'static str],
    pub other_keys: &'static [&'static str],
    pub context: &'static str,
}

pub struct KeybindingCategory {
    pub title: &'static str,
    pub entries: Vec<KeybindingEntry>,
}
```

Static catalog lives in `crates/console-ui/src/settings/keybindings_page.rs` as `pub fn keybinding_categories() -> Vec<KeybindingCategory>`. Single source of truth for the page; update it when `bind_keys` calls change.

## 4. Implementation Steps

1. **UI Component (`apps/desktop/crates/console-ui/src/settings/keybindings_page.rs`)**:
   - New `KeybindingsPage: RenderOnce` with props `{ filter_query: String, search_input: Option<Entity<ComposerInput>> }` following the `DeletedChatsPage` / `ModelsPage` `RenderOnce` pattern (stateless page; state lives in `SettingsWindow`).
   - Build with search filter input (`ComposerInput::search_field()`, see `common/input/mod.rs:394`).
   - Group rows by section with clean separator headers.
   - Render keyboard chip badges (`theme.surface`, `theme.border`, monospace font).
   - Pure helper `pub fn filter_keybindings(query: &str) -> Vec<...>` or method for tests.
2. **Settings crate (`apps/desktop/crates/console-ui/src/settings/mod.rs`)**:
   - Add `pub mod keybindings_page;` + re-export.
   - Add `SettingsTab::Keybindings` variant + `title() -> "Keyboard Shortcuts"`.
3. **Settings shell (`apps/desktop/crates/console-ui/src/settings/settings_shell.rs:35`)**:
   - Add `(SettingsTab::Keybindings, IconName::Keyboard, "Keybindings")` tab row (`IconName::Keyboard` exists at `primitives/icons.rs:52`).
4. **Settings Window (`apps/desktop/src/settings_window.rs`)**:
   - Add `keybindings_search: Entity<ComposerInput>` field, constructed via `ComposerInput::search_field()` with placeholder, plus subscription on `ComposerEvent::Edited/Focus` to `cx.notify()` (same pattern as `model_searches` lines 138-145).
   - Add `SettingsTab::Keybindings` match arm rendering `KeybindingsPage { filter_query, search_input }`.
5. **Responsive Search Filtering**:
   - Match query against the description (e.g. "reload"), action identifier (e.g. "BrowserReload"), and shortcut representation (e.g. "cmd+r", "ctrl+r").
   - Case-insensitive, trim; empty query shows all categories; hide empty categories; show "No shortcuts match" empty state.
6. **Verification**:
   - `cargo check -p console-ui` (or workspace check from `apps/desktop`).
   - No new backend/async work; no persistence.
