# Workspace Architecture & Multi-Tab Layout

## Overview

The Console Desktop application uses a flexible, multi-pane workspace layout powered by `flexlayout-react`. The workspace provides an IDE-like experience where users can open multiple tabs, split panes, and work with isolated project contexts seamlessly.

---

## Core Layout Structure

```
+------------------+-----------------------------------------+------------------+
|                  |              CENTER DOCK                |                  |
|   LEFT SIDEBAR   |            (FlexLayout Dock)            |  RIGHT SIDEBAR   |
|                  | +-------------------------------------+ |                  |
| - Projects       | | [Chat 1]  [Terminal]  [File: main.ts] | | - File Explorer  |
| - Chat Sessions  | +-------------------------------------+ | - Git Changes    |
| - Settings       | |                                     | |   (Staged /      |
|                  | |          Active Tab Content         | |    Unstaged)     |
|                  | |                                     | |                  |
|                  | +-------------------------------------+ |                  |
+------------------+-----------------------------------------+------------------+
```

### 1. Left Sidebar (Navigation & Sessions)
- **Project Selection:** Switch between or add active projects.
- **Chat Session List:** View, create, and open chat sessions per project.
- **Settings & Account Status:** Quick access to backend and provider preferences.

### 2. Center Dock (Multi-Tab Workspace)
Uses `flexlayout-react` to support splitting (horizontal & vertical), tab dragging, reordering, and docking:
- **`chat` Tabs:** Active agent chat screens bound to a `sessionId` and `projectId`.
- **`terminal` Tabs:** Embedded terminal instances.
- **`file` Tabs:** Code/file preview and editor views.
- **`diff` Tabs:** Git diff and code change viewers.

### 3. Right Sidebar (Contextual Tools)
- **File Explorer:** Directory tree of the active workspace project.
- **Git Changes:** Real-time view of modified, staged, and untracked files.

---

## Technology Stack Decisions

- **Layout Engine:** `flexlayout-react` for multi-tab management, pane splitting, dock layout, and tab drag-and-drop.
- **File & Diff Rendering:** `@pierre/diffs` (powered by Shiki syntax highlighting) for:
  - Single file reading/viewing (`"file"` tabs via `<File />` component).
  - Git diff inspection (`"diff"` tabs via `<PatchDiff />` / `<MultiFileDiff />` components).
  - Lightweight, high-performance rendering without Monaco's heavy web worker overhead.
- **Terminal Emulator:** `ghostty-web` (WASM-compiled Ghostty terminal engine) for:
  - `"terminal"` tabs in the center dock.
  - API-compatible with xterm.js but powered by Ghostty's high-fidelity VT parser, Unicode handling, and truecolor support.

---

## Project Context Isolation & Command Palette Scoping

To support multi-project workflows without cross-contamination:

1. **Tab-Level Context Invariant:**
   - Every workspace tab contains explicit context (`projectId` and working directory `cwd`).
2. **Active Tab Scoping:**
   - When a tab is focused, the global workspace state updates its active `projectId` and `cwd`.
   - **Command Palette (`⌘K` / `⌘P`):** Scopes file search and slash commands strictly to the `projectId` and `cwd` of the currently focused tab.
3. **Session Alignment:**
   - Opening a chat session from the left sidebar selects or creates a tab with that session's project scope.

---

## Phased Roadmap

### Phase 1: Multi-Tab & Active Tab Context (Current Foundation)
- [x] Integrate `flexlayout-react` layout engine.
- [x] Open chat sessions as center workspace tabs.
- [ ] Bind `⌘K` / `⌘P` file search and commands dynamically to the active tab's project context.

### Phase 2: Right Sidebar (File Explorer & Git Status)
- [x] Implement collapsible Right Sidebar container with TitleBar toggle.
- [x] Add project file tree explorer component using `@pierre/trees`.
- [ ] Add Git changes & diff status panel (pending backend support).

### Phase 3: Additional Tab Types
- [x] Create File Viewer component using `@pierre/diffs` (`"file"`).
- [x] Create Diff Viewer component using `@pierre/diffs` (`"diff"`).
- [ ] Implement Embedded Terminal tab using `ghostty-web` (`"terminal"`).

### Phase 4: Full Layout Persistence
- [ ] Save and restore the complete `flexlayout-react` layout state to disk (`uiStore` / `tauri-plugin-store`) across app restarts.
