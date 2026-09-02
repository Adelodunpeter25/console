# Right Sidebar & Workspace Inspector — Technical Implementation Plan

## 1. Overview & Scope

This plan details the end-to-end architecture and implementation for the **Right Workspace Inspector Panel** in the Desktop GPUI application, utilizing our existing server filesystem infrastructure and adding lightweight session file tracking and git diff statistics.

### Scope Focus:
* **Included**: Top Inspector panel with **`All files`** (repository tree browser) and **`Changes`** (git diffs & per-turn file mutation list with `+X` / `-Y` stats).
* **Deferred**: Bottom runner / terminal dock is postponed to a later phase to keep this deliverable focused and lean.

---

## 2. Server Infrastructure Audit & Requirements (`apps/server`)

### 2.1 Already Shipped & Available ✅
* **`GET /api/fs/entries?path=...&depth=6&hidden=false`**: Recursive directory tree listing with children, file sizes, and types.
* **`GET /api/fs/search?root=...&q=...`**: Fuzzy file search scoped to project root.
* **`GET /api/fs/file?path=...`**: Reading and previewing file contents.
* **`GET /api/fs/watch?path=...`**: Real-time SSE stream for filesystem change events.

### 2.2 Server Updates Required
1. **Enrich Git Status with `+X` / `-Y` Line Diffs**:
   * Update `GitService.getGitStatus` in `api/src/services/git.service.ts` to compute line additions and deletions via `git diff --numstat` (for both working tree and staged index):
     ```typescript
     export interface GitFileChange {
       path: string;
       status: "M" | "A" | "D" | "R" | "?";
       additions: number;
       deletions: number;
       staged: boolean;
     }
     ```
2. **Add Git Diff Endpoint**:
   * Add `GET /api/git/diff?path=<file>&cwd=<path>` to return on-demand unified diff text for file inspection.
3. **Session File Mutation Persistence (Zero-Bloat SQLite Table)**:
   * In per-session SQLite databases (`agent/src/session/schema.ts`), add:
     ```sql
     CREATE TABLE IF NOT EXISTS session_file_changes (
       path TEXT PRIMARY KEY,
       status TEXT NOT NULL,
       additions INTEGER NOT NULL DEFAULT 0,
       deletions INTEGER NOT NULL DEFAULT 0,
       turn_index INTEGER NOT NULL DEFAULT 0,
       updated_at INTEGER NOT NULL
     );
     ```
   * Stores only file paths, line metrics, and turn indexes (taking a few kilobytes per session, preventing database bloat).
4. **Agent Tool Execution Hook**:
   * Hook agent tool calls (`write_to_file`, `replace_file_content`, etc.) in `RunService` / `session-ops` to record touched files and stream them.
5. **Session Changes Route**:
   * Add `GET /api/sessions/:id/changes` in `routes/sessions.ts` to return the recorded files touched during that chat session.

---

## 3. Desktop Client Architecture (`apps/desktop`)

### 3.1 Core Client & Shared Types (`console-core`)
* Add DTOs: `GitFileChange`, `SessionFileChange`, `FileDiffResponse`.
* Add methods in `GitService`: `get_status(cwd)` (with diff stats), `get_file_diff(path, cwd)`.
* Add method in `SessionService`: `get_changes(session_id)`.

### 3.2 State Management (`apps/desktop/src/state/`)
* **`right_sidebar.rs`**:
  * `right_sidebar_visible: bool` (persisted in layout config).
  * `right_sidebar_width: Pixels` (resizable with drag handle, bounds `220px`–`500px`).
  * `active_inspector_tab: InspectorTab` (`AllFiles` | `Changes`).
  * `project_file_tree: Option<Rc<DirectoryTreeNode>>`.
  * `working_changes: Option<Rc<Vec<GitFileChange>>>`.
  * `session_changes: Option<Rc<Vec<SessionFileChange>>>`.

### 3.3 UI Components (`crates/console-ui/src/inspector/`)
* `RightSidebar`: Segmented header control (`All files` vs `Changes`), search input, and scrollable content.
* `FileTreeView`: Hierarchical tree with folder toggles, search filtering, and file extension icons.
* `ChangesListView`: List of changed files with color-coded diff badges (`+12`, `-4`) and git status dots.

---

## 4. Phased Roadmap (Backend-First)

### Phase 1: Server Updates (`apps/server`)
1. Add `session_file_changes` table in per-session SQLite schema and add CRUD operations in `SqliteSessionStorage`.
2. Hook tool execution in `RunService` to record touched files on each agent turn.
3. Update `GitService` to compute `additions`/`deletions` via `git diff --numstat`.
4. Add `GET /api/git/diff` and `GET /api/sessions/:id/changes` endpoints.
5. Verify all server changes with focused Bun unit tests.

### Phase 2: Core Client & Shared Types (`console-core`)
1. Implement DTOs and client service methods in `console-core`.
2. Verify serialization with cargo tests.

### Phase 3: Desktop Right Sidebar Shell & Toggle (`apps/desktop`)
1. Add `right_sidebar_visible` and `right_sidebar_width` state with persistent layout storage.
2. Add toggle button (`IconName::PanelRightOpen` / `IconName::PanelRightClose`) in the top titlebar.
3. Render resizable right sidebar container alongside workspace panes.

### Phase 4: `All Files` & `Changes` UI Views (`console-ui`)
1. Connect `GET /api/fs/entries` to `FileTreeView` with folder collapse/expand and search.
2. Build `ChangesListView` with git status badges and line diff counts.
3. Wire click-to-preview and live refresh on turn completion.
