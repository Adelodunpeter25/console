# Right Sidebar & Workspace Inspector — Technical Implementation Plan

## 1. Overview

This plan details the end-to-end architecture and implementation for a **Conductor-style Right Sidebar** in the Desktop GPUI application, supported by backend file mutation tracking, git status streaming, and an integrated bottom terminal/dev-run dock.

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ Top Window Titlebar (Left toggles ◧, Tab Bar, Right Sidebar Toggle ◨)                  │
├──────────────┬───────────────────────────────────────────┬─────────────────────────────┤
│ Left Sidebar │ Main Workspace (Split Panes)              │ Right Sidebar               │
│ (Sessions &  │ ┌───────────────────────┬───────────────┐ │ ┌─────────────────────────┐ │
│  Projects)   │ │ Active Chat Tab       │ Editor Tab    │ │ │ All files │ Changes (4) │ │
│              │ │                       │               │ │ ├─────────────────────────┤ │
│              │ │                       │               │ │ │ 📁 apps/desktop/        │ │
│              │ │                       │               │ │ │   ├── 📄 main.rs        │ │
│              │ │                       │               │ │ │   └── 📁 state/         │ │
│              │ └───────────────────────┴───────────────┘ │ ├─────────────────────────┤ │
│              │                                           │ │ Run | Terminal (1)  ▶ ■ │ │
│              │                                           │ │ $ bun run dev           │ │
│              │                                           │ └─────────────────────────┘ │
└──────────────┴───────────────────────────────────────────┴─────────────────────────────┘
```

---

## 2. Component Breakdown

### 2.1 Top Section: File & Changes Inspector
* **Segmented Control Tabs**:
  * **`All files`**: Hierarchical project directory tree viewer with search filtering, collapsible folders, and file-type icons.
  * **`Changes (N)`**: Live list of files modified during the current session / uncommitted git working tree, showing additions (`+X` green) and deletions (`-Y` red) badges.
* **File Interaction**:
  * **Single Click**: Opens the file in a preview tab inside the workspace split pane or opens inline diff view.
  * **Secondary Action / Drag**: Inserts `@path/to/file` mention into the active composer.

### 2.2 Bottom Section: Integrated Run & Terminal Dock
* **Tab Bar**: `Run` (dev runner) + `Terminal` tabs + `+` (new terminal tab) + Quick Action controls (Play `▶` / Stop `■`).
* **Terminal Docking**: Support for moving/dragging terminal tabs between the main workspace panes and the right sidebar runner dock.
* **One-Click Script Runner**: Preset button for detected project scripts (e.g. `npm run dev`, `cargo run`, `bun test`).

---

## 3. Backend Changes (`apps/server`)

### 3.1 Per-Turn File Mutation Tracking
* **After each agent run turn**:
  * Capture touched files from agent tool executions (`write_to_file`, `edit_file`, `replace_file_content`, bash git operations).
  * Compute line diff stats (`additions`, `deletions`, `status: "modified" | "created" | "deleted"`).
  * Stream updated `filesChanged` in the session run SSE events and persist in session metadata.

### 3.2 Git Status & Changes Endpoints
* **`GET /api/git/changes?cwd=<path>`**:
  * Returns structured working tree git status:
    ```typescript
    interface GitFileChange {
      path: string;
      status: "modified" | "added" | "deleted" | "untracked";
      additions: number;
      deletions: number;
      staged: boolean;
    }
    ```
* **`GET /api/sessions/:id/changes`**:
  * Returns all files modified within the scope of the given chat session.

### 3.3 Recursive FS Tree Endpoint
* **`GET /api/fs/tree?cwd=<path>&depth=3`**:
  * Fast recursive directory listing for the `All files` tree explorer, ignoring `.git`, `node_modules`, and `.turbo`.

---

## 4. Desktop Client Architecture (`apps/desktop`)

### 4.1 State Management (`src/state/`)
* **`src/state/right_sidebar.rs` (New)**:
  * `right_sidebar_visible: bool` (default `false` or toggleable via titlebar button `◨` / `Cmd+Shift+B`).
  * `right_sidebar_width: Pixels` (resizable, persisted in local store, default `280px`, bounds `220px`–`500px`).
  * `active_inspector_tab: InspectorTab` (`AllFiles` vs `Changes`).
  * `project_file_tree: Option<Rc<DirectoryTreeNode>>`.
  * `session_changes: HashMap<String, Vec<FileChangeEntry>>`.
  * `docked_terminals: Vec<Entity<TerminalView>>`.
  * `active_dock_tab: DockTab` (`Run` vs `Terminal(id)`).

### 4.2 UI Components (`crates/console-ui/`)
* **`crates/console-ui/src/inspector/` (New Module)**:
  * `right_sidebar.rs`: Main right container with left-border resize drag handle, segmented header, file list, and bottom terminal dock.
  * `file_tree.rs`: Recursive file tree component with search input and folder expand/collapse.
  * `changes_list.rs`: Git changes list with diff badges (`+12`, `-4`) and status indicators.
  * `dock_terminal.rs`: Embedded mini-terminal runner and dev script launcher.

### 4.3 Titlebar & Layout Integration
* Add right sidebar toggle button in the top right window titlebar next to the environment/connection indicator.
* Update `apps/desktop/src/layout.rs` workspace root layout to render `[ Left Sidebar | Main Split Workspace | Right Sidebar ]`.

---

## 5. Phased Implementation Roadmap

### Phase 1: Right Sidebar Shell & Toggle (Desktop)
1. Add `right_sidebar_visible` and `right_sidebar_width` with persistence and resize handle.
2. Add toggle button in the titlebar (`IconName::PanelRightOpen` / `IconName::PanelRightClose`).
3. Render basic right sidebar container alongside the main workspace split panes.

### Phase 2: All Files Tree Browser
1. Connect `GET /api/fs/tree` in `console-core`.
2. Build `FileTree` component in `console-ui` with file icons, search bar, and folder expand/collapse.
3. Wire click action to open preview tabs in the main pane or copy file paths.

### Phase 3: Git & Session Changes Inspector
1. Add `GET /api/git/changes` endpoint on the server with line diff calculation (`additions`/`deletions`).
2. Add per-turn file mutation capture in the agent run loop.
3. Build `Changes` tab view in the right sidebar with color-coded diff badges.

### Phase 4: Bottom Terminal & Dev Runner Dock
1. Implement collapsible bottom dock in the right sidebar.
2. Add dev script launcher (`Start Dev ⌘R`) for `package.json` / `Cargo.toml` scripts.
3. Enable docking / moving terminal tabs into the right sidebar dock.
