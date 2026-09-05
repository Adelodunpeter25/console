# Desktop Architecture: Project-Centric Workspace & Multi-Window Specification

## 1. Overview & Vision

This document details the architectural plan for Console Desktop's project-centric workspace and multi-window system.

Instead of generic arbitrary pane splits (tiling windows into fragmented sub-panes), Console adopts a **Cockpit Architecture** inspired by modern AI development workflows (such as Conductor), customized to eliminate machine-resource bloat (avoiding redundant build caches and heavy multi-worktree compilers on resource-constrained laptops).

```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Window: Console Monorepo                                                                                │
├───────────────┬────────────────────────────────────────────────────────┬────────────────────────────────┤
│ Left Sidebar  │ Center Cockpit                                         │ Right Sidebar                  │
│ (Our Existing │ ┌────────────────────────────────────────────────────┐ │ ┌────────────────────────────┐ │
│  Clean Date   │ │ Tabs: [Fix session keys] [Optimize mobile] [+]     │ │ │ Top: Changes (14) / Files  │ │
│  Grouping)    │ ├────────────────────────────────────────────────────┤ │ ├────────────────────────────┤ │
│               │ │                                                    │ │ │ Bottom: (Future Feature)   │ │
│ • Today       │ │ Active Chat Transcript                             │ │ │ [>_ Run] [Terminal] [+]    │ │
│   - Chat A    │ │                                                    │ │ │                            │ │
│   - Chat B    │ │                                                    │ │ │  ▶ Start Dev (⌘R)          │ │
│ • Yesterday   │ ├────────────────────────────────────────────────────┤ │ │  Terminal output / shell   │ │
│   - Chat C    │ │ Composer Input Area                                │ │ │                            │ │
│               │ └────────────────────────────────────────────────────┘ │ └────────────────────────────┘ │
└───────────────┴────────────────────────────────────────────────────────┴────────────────────────────────┘
```

---

## 2. Core Concepts

### A. The Project as the Context / Window Boundary
- A workspace window is anchored to an active **Project** (`cwd` and `project_id`).
- Switching or opening chats maintains continuity within that project:
  - All open chat tabs in the window share the project's working directory.
  - The right sidebar's active file changes, diffs, and terminals belong to that project.
  - Switching between chat tabs does **not** terminate or restart the project's background services or terminals.

### B. Left Sidebar: Retaining Console's Native Clean UI
- We do **not** use dense, nested folder-tree groupings on the left.
- Console preserves its clean, chronologically grouped sidebar (Today, Yesterday, 7 Days Ago, etc.) with project badges on each thread.
- **Smart Activation Rule**:
  - When clicking a chat in the sidebar:
    - **Same Project**: If the clicked chat belongs to the *same* `project_id`/`cwd` as the current window, it adds or activates a tab in the top tab bar.
    - **Different Project**: If the clicked chat belongs to a *different* project, it seamlessly switches the workspace context to that project (or opens it in a new window if requested).

### C. Center Cockpit: Single Focused Chat + Project Tabs
- Replaces generic left/right nested split panes.
- Top tab bar displays open tasks/chats for the current project.
- Center view is a focused, high-performance chat transcript and composer without visual clutter.

### D. Right Sidebar: Split Inspector & Execution Cockpit
The right sidebar is divided into two purposeful sections:

1. **Top Section: Code & Changes Inspector (Active)**
   - **`Changes (N)` Tab**: Displays git status, staged/unstaged changes, and diffs for the current project.
   - **`All Files` Tab**: Quick project file tree browser.
   - **`Subagents` Tab**: Subagent activity inspect.

2. **Bottom Section: Project Execution & Terminal (Future Roadmap)**
   - Resizable bottom split in the right sidebar.
   - **`Run` Tab**:
     - Configured one-click run scripts (e.g., `Start Dev ⌘R`, `Build`, `Expo Export`).
     - Visual state indicators: Idle, Running (with elapsed timer), Success, or Failure.
     - Play / Stop action buttons.
   - **`Terminal` Tab**:
     - Embedded interactive terminal session rooted in the project directory.
     - Ability to add multiple terminal tabs with `+`.
   - **Why this matters**: Developers can watch the AI modify code in the center, view the exact diff in the top-right, and trigger/monitor dev servers or compilers in the bottom-right—all within a single cockpit without switching apps or creating heavy worktrees.

---

## 3. Multi-Window Integration

Because the workspace context is cleanly bounded by Project:

1. **Independent Project Windows (`File -> New Window` / `⌘⇧N`)**:
   - Each native OS window hosts a project context.
   - Window 1 can run Project A (`Console`), while Window 2 runs Project B (`Backend`).
   - Both connect to the local `console-server` daemon, sharing memory, auth, and database state without duplicating processes.

2. **Window Management & State Persistence**:
   - Each window maintains its own:
     - Active project ID & cwd.
     - Set of open chat tab IDs.
     - Right sidebar visibility and bottom-split height.
     - Window bounds and screen position.
   - Restoring the desktop app reopens previous project windows in their saved bounds.

---

## 4. Implementation Phasing

### Phase 1: Project-Centric Tabbed Workspace (Immediate Next Step)
- Connect sidebar chat selection to project context detection:
  - If selected chat matches active project: add/select top tab.
  - If selected chat has a different project: switch project context and swap top tabs.
- Streamline center layout: eliminate complex nested split panes in favor of top project tabs.

### Phase 2: Multi-Window Support
- Update GPUI desktop entry point to support opening and tracking multiple `WorkspaceWindow` entities.
- Implement window-level persistence in `persistence::window` (saving an array of open window states).
- Map `⌘⇧N` (New Window) and proper window-closing semantics (`⌘W` closes focused window, `⌘Q` quits app).

### Phase 3: Right Sidebar Bottom Split — Terminal & Run Scripts (Future Roadmap)
- Implement bottom split container in the right sidebar with drag-resize handle.
- Integrate terminal emulator tabs inside the bottom-right panel.
- Add project script configuration (`console.json` or project settings) to surface one-click `Run` play buttons (e.g. dev server, build commands) alongside execution logs.
