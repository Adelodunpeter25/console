# Plan: Keybindings & Keyboard Shortcuts Settings Page

## 1. Overview & Goal

As Console grows with features across Chat, Code Viewing, Terminal, Browser, and Subagents, the list of keyboard shortcuts has become substantial. 

This document defines the plan for adding a dedicated, read-only **"Keyboard Shortcuts"** page inside the Console Desktop Settings window (`apps/desktop/src/settings_window.rs` and `crates/console-ui/src/settings/`).

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
│ • Projects      │ │ 🔍 Filter shortcuts (e.g., "browser", "composer", "cmd+r")...          │ │
│ • Environments  │ └───────────────────────────────────────────────────────────────────────┘ │
│ • Connection    │                                                                           │
│ • Usage         │ ── Global & Navigation ────────────────────────────────────────────────── │
│ • Keybindings ◄ │ Command Palette                                       [ ⌘ ] [ K ]         │
│ • Deleted Chats │ Quick Open File                                       [ ⌘ ] [ P ]         │
│                 │ Open Project Browser                                  [ ⌘ ] [ O ]         │
│                 │ Toggle Left Sidebar                                   [ ⌘ ] [ B ]         │
│                 │ Toggle Right Sidebar                                  [ ⌘ ] [ J ]         │
│                 │ Settings                                              [ ⌘ ] [ , ]         │
│                 │ New Chat Session                                      [ ⌘ ] [ N ]         │
│                 │ Close Active Tab                                      [ ⌘ ] [ W ]         │
│                 │                                                                           │
│                 │ ── Chat & Composer ────────────────────────────────────────────────────── │
│                 │ Submit Prompt                                         [ Enter ]           │
│                 │ New Line                                              [ ⇧ ] [ Enter ]     │
│                 │ Steer / Send Now                                      [ ⌘ ] [ Enter ]     │
│                 │ Focus Composer Input                                  [ ⌘ ] [ L ]         │
│                 │ Trigger File Mention                                  [ @ ]               │
│                 │ Trigger Slash Command                                 [ / ]               │
│                 │ Autocomplete Next / Previous                          [ ↓ ] / [ ↑ ]       │
│                 │                                                                           │
│                 │ ── Browser Surface ────────────────────────────────────────────────────── │
│                 │ Focus Address Bar                                     [ ⌘ ] [ L ]         │
│                 │ Reload Page                                           [ ⌘ ] [ R ]         │
│                 │ Hard Reload (Bypass Cache)                            [ ⇧ ] [ ⌘ ] [ R ]   │
│                 │ Navigate Back                                         [ ⌘ ] [ [ ]         │
│                 │ Navigate Forward                                      [ ⌘ ] [ ] ]         │
│                 │ Stop Loading                                          [ Esc ]             │
│                 │ Toggle Web Inspector / DevTools                       [ ⌥ ] [ ⌘ ] [ I ]   │
│                 │                                                                           │
│                 │ ── Terminal & Code Viewer ─────────────────────────────────────────────── │
│                 │ New Terminal Instance                                 [ ⌘ ] [ T ]         │
│                 │ Find in Code Viewer                                   [ ⌘ ] [ F ]         │
│                 │ Find Next Match                                       [ F3 ]              │
│                 │ Find Previous Match                                   [ ⇧ ] [ F3 ]        │
└─────────────────┴───────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Shortcut Registry Structure

In `apps/desktop/crates/console-ui/src/settings/keybindings_page.rs`:

```rust
pub struct ShortcutCategory {
    pub title: &'static str,
    pub items: Vec<ShortcutItem>,
}

pub struct ShortcutItem {
    pub description: &'static str,
    pub macos_keys: &'static [&'static str],
    pub other_keys: &'static [&'static str],
    pub context: &'static str, // e.g. "Global", "Composer", "Browser"
}
```

### Keybinding Catalog

1. **Global & Workspace**:
   - `⌘K` / `Ctrl+K`: Open Command Palette
   - `⌘P` / `Ctrl+P`: Quick Open File
   - `⌘O` / `Ctrl+O`: Open Project Picker
   - `⌘B` / `Ctrl+B`: Toggle Left Sidebar
   - `⌘J` / `Ctrl+J`: Toggle Right Sidebar
   - `⌘,` / `Ctrl+,`: Open Settings Window
   - `⌘N` / `Ctrl+N`: New Chat Session
   - `⌘W` / `Ctrl+W`: Close Current Tab
   - `⌘1..9` / `Ctrl+1..9`: Switch Workspace Tabs

2. **Chat & Composer**:
   - `Enter`: Submit Draft / Send Prompt
   - `Shift+Enter`: Insert Newline
   - `⌘Enter` / `Ctrl+Enter`: Submit Steer / Queue Prompt
   - `⌘L` / `Ctrl+L`: Focus Composer
   - `@`: Trigger File Mention Autocomplete
   - `/`: Trigger Slash Command Autocomplete
   - `Tab` / `Enter`: Confirm Autocomplete Suggestion
   - `Esc`: Dismiss Autocomplete / Mention

3. **Browser Surface**:
   - `⌘L` / `Ctrl+L`: Focus Address Bar
   - `⌘R` / `Ctrl+R`: Reload Page
   - `⇧⌘R` / `Ctrl+Shift+R`: Hard Reload
   - `⌘[` / `Ctrl+[`: Navigate Back
   - `⌘]` / `Ctrl+]`: Navigate Forward
   - `Esc`: Stop Page Loading
   - `⌥⌘I` / `Ctrl+Shift+I`: Toggle Web Inspector / DevTools

4. **Terminal & Code Viewer**:
   - `⌘T` / `Ctrl+T`: New Terminal Tab
   - `⌘F` / `Ctrl+F`: Find in File
   - `F3` / `Enter`: Next Search Match
   - `Shift+F3` / `Shift+Enter`: Previous Search Match
   - `Esc`: Close Search Bar

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
   - Match query against both the description (e.g. "reload") and the shortcut representation (e.g. "cmd+r", "ctrl+r").
