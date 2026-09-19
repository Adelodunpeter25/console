# Architecture Plan: Session Changes & Turn-by-Turn Diffs

## 1. Goal & Motivation
Currently, the Changes tab in the right inspector attempts to display Git working tree changes (`git status --porcelain`) alongside a session-level file changes tracker. This presents several problems:
1. **Index Lock Issues**: Frequent git status/diff operations risk encountering `.git/index.lock` contention if the user or background tasks run git operations concurrently.
2. **Non-Git Repositories**: Folders without git show "No working tree changes" even when the agent has modified or created files.
3. **Commit Volatility**: As soon as a user (or agent) commits, git working changes vanish completely.
4. **Lack of Turn-Level Granularity**: Users cannot view what changed *specifically in the last prompt* versus *across the entire session*.

### The Solution
Migrate the primary Changes view to an in-database **Session & Turn-by-Turn Diff Engine** stored in SQLite, capturing unified patch deltas directly when tool actions execute.

---

## 2. Architecture & Data Model

### SQLite Schema: `session_file_changes`
Update the existing table in the session database (or introduce a new migration in `session-changes.ts`):

```sql
CREATE TABLE IF NOT EXISTS session_file_changes (
  path TEXT NOT NULL,
  turn_index INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL, -- 'added' | 'modified' | 'deleted'
  additions INTEGER NOT NULL DEFAULT 0,
  deletions INTEGER NOT NULL DEFAULT 0,
  diff_text TEXT,       -- Unified diff representation (--- a/... +++ b/...)
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (path, turn_index)
);

CREATE INDEX IF NOT EXISTS idx_session_file_changes_turn 
  ON session_file_changes (turn_index, updated_at DESC);
```

By keying on `(path, turn_index)`:
- We can query all changes for a specific turn (e.g. `WHERE turn_index = :latest_turn`).
- We can aggregate or roll up changes across the whole session (e.g. `SELECT DISTINCT path ...`).

---

## 3. Server-Side Diff Generation (`run-file-changes.ts`)

When an agent executes file-modifying tools:

### A. `edit_file` / `replace_file_content`
- **Zero I/O**: `TargetContent` (before) and `ReplacementContent` (after) are already present in the tool call arguments.
- Compute unified patch using a lightweight diff algorithm (e.g., `diff` package or simple line-by-line unified diff formatter):
  ```typescript
  import { createPatch } from "diff";
  const patch = createPatch(fileName, targetContent, replacementContent);
  ```
- Store `patch` directly in `diff_text`.

### B. `write_file` / `writeFile`
- If creating a brand new file:
  - `additions` = line count of content.
  - `diff_text` = unified diff against an empty file (`/dev/null` -> `b/path`).
- If overwriting an existing file:
  - Read existing file content prior to overwrite.
  - Generate unified patch `createPatch(fileName, oldContent, newContent)`.
  - Store `patch` in `diff_text`.

### C. `delete_file` (if applicable)
- Mark status as `deleted`, `diff_text` representing the removal of lines.

---

## 4. API Endpoints

### 1. `GET /api/sessions/:id/changes`
Query params:
- `turnIndex` (optional): If provided, returns changes for that turn only. If omitted, returns all session changes aggregated by file.

Response payload:
```typescript
export interface SessionFileChangeDto {
  path: string;
  turnIndex: number;
  status: "added" | "modified" | "deleted";
  additions: number;
  deletions: number;
  diffText?: string;
  updatedAt: number;
}
```

### 2. `GET /api/sessions/:id/changes/diff?path=...&turnIndex=...`
Returns the cached `diff_text` directly as text/plain or JSON, avoiding any git subprocess invocation.

---

## 5. Desktop Client Integration

### A. Inspector Right Sidebar (`right_sidebar.rs` & `changes_list.rs`)
1. **Toggle/Filter Header**:
   - `[ This Turn ]` vs `[ All Turns ]` segment toggle at the top of the Changes list.
2. **List Rendering**:
   - Render `self.session_changes` directly in `ChangesListView`.
   - Display file name, path, status badge (`A`, `M`, `D`), and `+N / -N` line stats.
3. **Diff Viewing**:
   - Clicking a file entry opens a `Diff` tab in the workspace pane.
   - The diff tab consumes `diff_text` fetched via `/api/sessions/:id/changes/diff`.
   - Non-git folders render diffs identically to git repos.

---

## 6. Implementation Steps

1. **Step 1 (Schema & DB)** ✅ COMPLETED:
   - Updated `apps/server/agent/src/session/schema.ts` to support `diff_text` column and composite primary key `(path, turn_index)`.
   - Updated `apps/server/agent/src/session/session-changes.ts` to handle new schema and `turnIndex` parameter.
   - Updated `packages/types/src/session.ts` to include `diffText` in `SessionFileChange` interface.

2. **Step 2 (Diff Generator)** ✅ COMPLETED:
   - In `apps/server/api/src/services/run/run-file-changes.ts`, integrated diff generation using the `diff` package for `write_file`, `replace_file_content`, and `batch_write`.
   - Added 1MB size limit for diff storage to prevent large file issues.
   - Updated `apps/server/api/src/services/run.service.ts` to pass the active `turnIndex` into `extractAndRecordFileChange`.

3. **Step 3 (API Updates)** ✅ COMPLETED:
   - Updated `apps/server/api/src/routes/sessions.ts` to support `turnIndex` query param and return `diffText`.
   - Added `GET /api/sessions/:id/changes/diff` endpoint for fetching specific diff text.
   - Updated `apps/server/api/src/services/session.service.ts` to support turn-based queries.

4. **Step 4 (Desktop UI Updates)** ⏳ PENDING:
   - In `apps/desktop/crates/console-ui/src/inspector/changes_list.rs`, render the `session_changes` list with proper file row styling.
   - Wire `open_diff_tab` to display the session diff rather than relying solely on `git diff`.
   - Add toggle for "Latest Turn" vs "All Turns".

5. **Step 5 (Verification)** ⏳ PENDING:
   - Test in an empty non-git directory:
     - Run prompt: *"Create a hello.txt file and write 3 lines"*.
     - Verify Changes tab shows `1` badge with `+3` additions.
     - Click file -> verify Diff viewer renders file addition patch.
   - Run prompt 2: *"Modify hello.txt to change line 2"*.
     - Verify "This Turn" shows only the 1 line change.
     - Verify "All Turns" shows the full cumulative session changes.
