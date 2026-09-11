# Plan: Keybindings & Keyboard Shortcuts Settings Page

## 1. Overview & Goal

As Console grows with features across Chat, Code Viewing, Terminal, Browser, and Subagents, the list of keyboard shortcuts has expanded significantly.

This document defines the plan for adding a dedicated, read-only **"Keyboard Shortcuts"** page inside the Console Desktop Settings window (`apps/desktop/src/settings_window.rs` and `crates/console-ui/src/settings/keybindings_page.rs`), reflecting the exact action names and bindings registered in `apps/desktop/src/keybindings.rs`.

> **Scope Boundary**:
> - **In Scope**: A clean, comprehensive, categorized, searchable catalog of all existing keybindings.
> - **Out of Scope (Deferred)**: Custom keybinding remapping and user shortcut overrides.

---

## 2. UI & Interaction Design

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ Settings                                                                                    │
├─────────────────┬───────────────────────────────────────────────────────────────────────────┤
│ • Accounts      │ Keyboard Shortcuts                                                        │
│ • Models        │ ┌───────────────────────────────────────────────────────────────────────┐ │
│ • Projects      │ │ 🔍 Filter shortcuts (e.g., "browser", "composer", "cmd+r", "switch")...│ │
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
│                 │ Toggle Right Sidebar (`ToggleRightSidebar`)           [ ⌘ ] [ J ]         │
│                 │ Open Settings Window (`OpenSettings`)                 [ ⌘ ] [ , ]         │
│                 │ Focus Composer Input (`FocusComposer`)                [ ⌘ ] [ L ]         │
│                 │ Toggle Model Picker (`ToggleModelPicker`)             [ ⌘ ] [ / ]         │
│                 │ Switch Session 1–9 (`SwitchSession1..9`)              [ ⌘ ] [ 1..9 ]      │
│                 │ Switch Workspace Tab 1–9 (`SwitchWorkspaceTab1..9`)   [ ⌥ ] [ 1..9 ]      │
│                 │                                                                           │
│                 │ ── Chat & Composer ────────────────────────────────────────────────────── │
│                 │ Submit Prompt / Send                                  [ Enter ]           │
│                 │ Insert Newline                                        [ ⇧ ] [ Enter ]     │
│                 │ Submit Steer / Queue Prompt                           [ ⌘ ] [ Enter ]     │
│                 │ Cycle Approval Mode (`CycleApprovalMode`)             [ ⇧ ] [ Tab ]       │
│                 │ Trigger File Mention                                  [ @ ]               │
│                 │ Trigger Slash Command                                 [ / ]               │
│                 │ Focus Model Search (`FocusModelSearch`)               [ / ]               │
│                 │ Confirm Autocomplete                                  [ Tab ] / [ Enter ] │
│                 │ Dismiss Autocomplete / Mention                        [ Esc ]             │
│                 │                                                                           │
│                 │ ── Browser Surface (console-ui/src/browser/actions.rs) ────────────────── │
│                 │ Focus Address Bar (`FocusBrowserAddress`)             [ ⌘ ] [ L ]         │
│                 │ Reload Page (`BrowserReload`)                         [ ⌘ ] [ R ]         │
│                 │ Hard Reload (`BrowserHardReload`)                     [ ⇧ ] [ ⌘ ] [ R ]   │
│                 │ Navigate Back (`BrowserBack`)                         [ ⌘ ] [ [ ]         │
│                 │ Navigate Forward (`BrowserForward`)                   [ ⌘ ] [ ] ]         │
│                 │ Stop Loading (`BrowserStop`)                          [ Esc ]             │
│                 │ Toggle Web Inspector / DevTools (`BrowserDevtools`)   [ ⇧ ] [ ⌘ ] [ I ]   │
│                 │ Cancel Address Edit (`BrowserAddressCancel`)          [ Esc ]             │
│                 │                                                                           │
│                 │ ── Terminal & Code Viewer ─────────────────────────────────────────────── │
│                 │ New Terminal Instance (`NewTerminal`)                 [ ⌘ ] [ T ]         │
│                 │ Close Active Terminal (`CloseActiveTerminal`)         [ ⇧ ] [ ⌘ ] [ W ]   │
│                 │ Next Terminal Tab (`NextTerminalTab`)                 [ ⇧ ] [ ⌘ ] [ ] ]   │
│                 │ Previous Terminal Tab (`PrevTerminalTab`)             [ ⇧ ] [ ⌘ ] [ [ ]   │
│                 │ Toggle Bottom Terminal (`ToggleTerminalBottom`)       [ ⌃ ] [ ` ]         │
│                 │ Clear Terminal (`ClearTerminal`)                      [ ⌘ ] [ K ]         │
│                 │ Find in Code Viewer (`ToggleSearch`)                  [ ⌘ ] [ F ]         │
│                 │ Next Search Match (`NextMatch`)                       [ Enter ] / [ F3 ]  │
│                 │ Previous Search Match (`PrevMatch`)                   [ ⇧ ] [ Enter/F3 ]  │
│                 │ Close Find Bar (`CloseSearch`)                        [ Esc ]             │
└─────────────────┴───────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Data Model from Codebase Sources

The shortcuts are mapped directly from:
1. `apps/desktop/src/keybindings.rs` (Global window actions)
2. `apps/desktop/crates/console-ui/src/browser/actions.rs` (Browser actions)
3. `apps/desktop/crates/console-ui/src/terminal/actions.rs` (Terminal actions)
4. `apps/desktop/crates/console-ui/src/viewer/code_viewer.rs` (Code viewer find actions)
5. `apps/desktop/crates/console-ui/src/common/input/actions.rs` (Text editing & composer actions)

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

---

## 4. Implementation Steps

1. **UI Component (`crates/console-ui/src/settings/keybindings_page.rs`)**:
   - Build `KeybindingsPage` with search filter input (`ComposerInput::search_field()`).
   - Group rows by section with clean separator headers.
   - Render keyboard chip badges (`theme.surface`, `theme.border`, monospace font).
2. **Settings Window Navigation (`apps/desktop/src/settings_window.rs`)**:
   - Add `SettingsSection::Keybindings` with keyboard icon (`IconName::Keyboard`).
   - Mount `KeybindingsPage` when selected.
3. **Responsive Search Filtering**:
   - Match query against the description (e.g. "reload"), action identifier (e.g. "BrowserReload"), and shortcut representation (e.g. "cmd+r", "ctrl+r").
