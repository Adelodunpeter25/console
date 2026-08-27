# Right Sidebar & Workspace Inspector — Technical Implementation Plan

## 1. Overview & Scope

This plan details the end-to-end architecture and implementation for the **Right Workspace Inspector Panel** in the Desktop GPUI application, following a **backend-first execution order**.

### Scope Focus:
* **Included**: Top Inspector panel with **`All files`** (repository tree browser) and **`Changes`** (git diffs & per-turn file mutation list with `+X` / `-Y` stats).
* **Deferred**: Bottom runner / terminal dock is postponed to a later phase to keep this deliverable focused and lean.

---

## 2. Component & Feature Breakdown

### 2.1 Top Inspector Tabs
1. **`All files` Tab**:
   * Hierarchical project directory tree viewer with search filtering, collapsible folders, and file-type icons.
   * Single click opens a preview tab in the workspace editor pane; drag or secondary action inserts `@path/to/file` into composer.
2. **`Changes (N)` Tab**:
   * Live list of files modified during the chat session and uncommitted git working tree changes.
   * Displays status tag (Modified, Added, Deleted, Untracked) along with green `+X` additions and red `-Y` deletions counts.
   * Selecting a file opens a diff preview in the center workspace pane.

### 2.2 Titlebar Integration
* Collapsible panel toggle button (`IconName::PanelRightOpen` / `IconName::PanelRightClose` or `◨`) in the top window titlebar.
* Resizable left border with width persistence (`280px` default, `220px`–`500px` bounds).

---

## 3. Backend Architecture (`apps/server`)

We build and verify all backend surfaces first before touching the frontend.

### 3.1 Per-Turn File Mutation Tracking & Database Storage
* **Lightweight Storage Design**:
  * To prevent database bloat, the database does **not** store entire raw file contents or verbose unified diffs.
  * Instead, we store a structured, lightweight record:
    ```typescript
    interface SessionFileChange {
      path: string;           // Relative file path
      status: "modified" | "added" | "deleted";
      additions: number;      // Line count added
      deletions: number;      // Line count removed
      turnIndex: number;      // Turn index when change occurred
      updatedAt: number;      // Timestamp
    }
    ```
  * Persisted in a dedicated SQLite table `session_file_changes (session_id, path, status, additions, deletions, turn_index, updated_at)` indexed by `(session_id, path)`.
* **Agent Run Integration**:
  * During agent execution turns, tool calls modifying the filesystem (`write_to_file`, `edit_file`, `replace_file_content`, etc.) compute the line delta and upsert the session change record.
  * Emitted in SSE run stream events and queryable via API.

### 3.2 Git Status & Changes Endpoints
* **`GET /api/git/changes?cwd=<path>`**:
  * Returns git working tree status and line counts for all staged and unstaged changes:
    ```typescript
    interface GitWorkingChange {
      path: string;
      status: "modified" | "added" | "deleted" | "untracked";
      additions: number;
      deletions: number;
      staged: boolean;
    }
    ```
* **`GET /api/git/diff?path=<file>&cwd=<path>`**:
  * Returns unified diff text for a specific file on demand for frontend diff rendering.
* **`GET /api/sessions/:id/changes`**:
  * Returns all accumulated file mutations for the specified session.

### 3.3 Scalable Recursive FS Tree Endpoint
* **`GET /api/fs/tree?cwd=<path>&depth=3`**:
  * Recursive directory listing with smart ignores (`.git`, `node_modules`, `.turbo`, `target`, `dist`, `.DS_Store`).
  * On-demand lazy-loading when expanding deeply nested folders.

---

## 4. Frontend & Desktop Client (`apps/desktop`)

### 4.1 Core Client & Shared Types (`console-core`)
* Add DTOs: `GitWorkingChange`, `SessionFileChange`, `DirectoryTreeNode`, `FileDiffResponse`.
* Add services: `GitService.getChanges(cwd)`, `GitService.getFileDiff(path, cwd)`, `FsService.getTree(cwd, depth)`.

### 4.2 State Management (`apps/desktop/src/state/`)
* **`right_sidebar.rs`**:
  * `right_sidebar_visible: bool` (persisted in layout config).
  * `right_sidebar_width: Pixels` (resizable with drag handle).
  * `active_inspector_tab: InspectorTab` (`AllFiles` | `Changes`).
  * `project_file_tree: Option<Rc<DirectoryTreeNode>>`.
  * `working_changes: Option<Rc<Vec<GitWorkingChange>>>`.
  * `session_changes: Option<Rc<Vec<SessionFileChange>>>`.

### 4.3 UI Components (`crates/console-ui/src/inspector/`)
* `RightSidebar`: Segmented header control (`All files` vs `Changes`), search input, and scrollable content.
* `FileTreeView`: Hierarchical tree with folder toggles, search filtering, and file extension icons.
* `ChangesListView`: List of changed files with color-coded diff badges (`+12`, `-4`) and git status dots.

---

## 5. Phased Roadmap (Backend-First)

### Phase 1: Backend Persistence & Endpoints (`apps/server`)
1. Create `session_file_changes` SQLite table and lightweight storage operations in `SqliteSessionStorage`.
2. Hook tool execution to record per-turn file mutations and emit changes.
3. Implement `GET /api/git/changes` and `GET /api/git/diff`.
4. Implement `GET /api/fs/tree` with smart ignore filtering.
5. Verify all endpoints with automated Bun tests.

### Phase 2: Core Client & Shared Types (`console-core`)
1. Implement DTOs and client methods in `console-core`.
2. Verify serialization and error handling with unit tests.

### Phase 3: Desktop Right Sidebar Shell & Toggle (`apps/desktop`)
1. Add `right_sidebar_visible` and `right_sidebar_width` state with persistent layout storage.
2. Add toggle button (`IconName::PanelRightOpen` / `IconName::PanelRightClose`) in the top titlebar.
3. Render resizable right sidebar container alongside workspace panes.

### Phase 4: `All Files` & `Changes` UI Views (`console-ui`)
1. Build `FileTreeView` component with folder collapse/expand, search filtering, and click-to-preview.
2. Build `ChangesListView` component with git status badges and line diff counts.
3. Connect live refresh on turn completion and tab switching.
