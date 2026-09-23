# Architecture Plan: Session Changes & Turn-by-Turn Diffs

## 1. Goal & Motivation
Currently, the Changes tab in the right inspector attempts to display Git working tree changes (`git status --porcelain`) alongside a session-level file changes tracker. This presents several problems:
1. **Index Lock Issues**: Frequent git status/diff operations risk encountering `.git/index.lock` contention if the user or background tasks run git operations concurrently.
2. **Non-Git Repositories**: Folders without git show "No working tree changes" even when the agent has modified or created files.
3. **Commit Volatility**: As soon as a user (or agent) commits, git working changes vanish completely.
4. **Lack of Turn-Level Granularity**: Users cannot view what changed *specifically in the last prompt* versus *across the entire session*.
5. **No Review Workflow**: There is no way to page through every changed file like a GitHub PR review, mark files as reviewed, or track review progress across a large multi-file turn.

### The Solution
Migrate the primary Changes view to an in-database **Session & Turn-by-Turn Diff Engine** stored in SQLite, capturing unified patch deltas directly when tool actions execute, and add a GitHub-style **"View all" review tab** for paging through every changed file with per-file mark-as-reviewed state.

---

## 2. Current Status (as of this plan revision)

Backend (Steps 1–3 of the original plan) is **implemented and working**:
- `session_file_changes` table exists with `(path, turn_index)` composite key, `status`, `additions`, `deletions`, `diff_text`, `updated_at`.
- Diff generation on `write_file` / `replace_file_content` / `batch_write` is wired in `run-file-changes.ts`, with a 1MB cap on stored diff text.
- `GET /api/sessions/:id/changes` (optional `turnIndex` query param) and `GET /api/sessions/:id/changes/diff?path=&turnIndex=` are live.

Desktop UI (Step 4) is **partially implemented**:
- `ChangesListView` (`apps/desktop/crates/console-ui/src/inspector/changes_list.rs`) renders a flat, non-collapsible list combining `working_changes` (git) and `session_changes` (DB-backed), each row showing status letter, icon, filename, short dir, and `+N/-N`. Clicking a row calls `on_select_file`, which currently opens the single-file Diff tab in the main workspace pane (`viewer/diff_viewer.rs`).
- `RightSidebar` (`inspector/right_sidebar.rs`) hosts this as the "Changes" primary tab, with a live badge count and a refresh button. No "This Turn" / "All Turns" toggle exists yet, and there's no aggregate `N files changed +X -Y` header row.
- `DiffView` (`chat/diff_view.rs`) already renders a single file's diff nicely (file-type icon + name header, `+N -M` summary badge, scrollable colored diff body with gutter, capped at 500 rendered lines) — currently only used inline inside chat tool-call bubbles, not reused by the Changes tab or a review tab yet. This component is the right building block to reuse for per-file rows in the new review tab.

None of the "view all" / review-flow / reviewed-state work below has started. This revision defines that work precisely so it can be implemented next.

---

## 3. Data Model

### 3.1 Existing schema (`session_file_changes`) — keep as-is
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

### 3.2 New column: `reviewed`
Reviewed state must persist (confirmed requirement), and is naturally scoped to the same `(path, turn_index)` row it belongs to — if a file changes again in a later turn, that's a new row and starts unreviewed. This is the correct behavior: new changes always need a fresh look, even if an earlier version of the same file was already reviewed.

```sql
ALTER TABLE session_file_changes ADD COLUMN reviewed INTEGER NOT NULL DEFAULT 0;
```

Migration notes:
- Additive, backward compatible; existing rows default to `reviewed = 0`.
- No new table needed — this keeps reviewed-state colocated with the diff it describes, and it naturally resets per-turn without extra logic.

---

## 4. API Endpoints

### 4.1 `GET /api/sessions/:id/changes` (existing, unchanged shape)
Query params:
- `turnIndex` (optional): If provided, returns changes for that turn only. If omitted, returns all session changes aggregated by file.

Response payload (add `reviewed`):
```typescript
export interface SessionFileChangeDto {
  path: string;
  turnIndex: number;
  status: "added" | "modified" | "deleted";
  additions: number;
  deletions: number;
  diffText?: string;
  reviewed: boolean;
  updatedAt: number;
}
```

### 4.2 `GET /api/sessions/:id/changes/diff?path=...&turnIndex=...` (existing, unchanged)
Returns the cached `diff_text` directly, avoiding any git subprocess invocation.

### 4.3 New: `POST /api/sessions/:id/changes/reviewed`
Marks (or unmarks) a single file's row as reviewed. Toggling is idempotent and scoped to the exact `(path, turnIndex)` row so it can't accidentally mark the wrong turn's version of a file.

Request body:
```typescript
{
  path: string;
  turnIndex: number;
  reviewed: boolean;
}
```
Response: `204 No Content` (or the updated `SessionFileChangeDto` for convenience — desktop client just needs the ack).

### 4.4 Future (not this phase): commit-history scope
The planned filter dropdown (§6) includes a "Git commit history" option. That option is **out of scope for this implementation phase** — it requires real `git diff <commit>` subprocess calls, which is exactly the volatility/lock-contention problem this whole migration was created to avoid. Ship "This Turn" / "All Turns" / "Previous Turn" first (all servable from `session_file_changes` with no git calls), and revisit commit-history as a distinct, clearly-labeled phase 2 once the turn-based flow is solid.

---

## 5. Desktop Client — Changes Tab (sidebar, incremental changes only)

Keep almost everything about the existing flat list. Add:

1. **Aggregate header row**: `N files changed   +X -Y` (matches the reference screenshot), computed client-side by summing the current filtered result set's `additions`/`deletions`.
2. **Scope dropdown**: `This Turn | All Turns | Previous Turn` (Git commit history added later, see §4.4). Selecting a scope re-queries `GET /changes` with the appropriate `turnIndex` (or omits it for "All Turns"; "Previous Turn" resolves to `latestTurn - 1` client-side).
3. **"View all" button**, next to the scope dropdown. Opens a new workspace tab (§6) — a variant of the existing Diff tab — scoped to whatever the dropdown currently has selected.
4. Individual file rows: **unchanged behavior** — clicking still opens the single-file Diff tab via `on_select_file`. No inline accordion in the sidebar; the narrow width isn't a great home for full diffs.

---

## 6. Desktop Client — "View all" Review Tab (new)

A new tab type in the workspace pane (sibling to the existing single-file Diff tab, likely `DiffViewerTab::All { turn_scope: ... }` or similar — exact enum shape TBD at implementation time in `viewer/diff_viewer.rs`).

Layout, top to bottom:
- Tab-level header restating the current scope (e.g. "All Turns — 2 files changed +9 -5") so the user has context without needing to look back at the sidebar dropdown.
- One block per changed file, in a single continuously scrollable column:
  - **File header row**: file-type icon, filename + full path (reusing `base_name`/path formatting already used in `changes_list.rs` and `diff_view.rs`), and a **"Mark as reviewed"** button/checkbox on the right (GitHub-style).
  - **Diff body**: reuse `DiffView` (`chat/diff_view.rs`) as the renderer for each file's colored unified diff — it already has the header/summary/body shape needed, just needs to be driven by fetched `diffText` per file instead of a live tool-call `DiffResult`.
  - **Accordion behavior**: each file block is individually collapsible/expandable (chevron toggle). Marking a file as reviewed **collapses/dims it** automatically (GitHub-style — shrinks to just the header bar, filename dimmed, checkbox filled/checked). Un-reviewing re-expands it.
- User scrolls down through the whole list top to bottom to page through the full review, same flow as a GitHub PR "Files changed" tab.

State needed on the desktop side:
- `reviewed: HashSet<(path, turn_index)>` (or just keyed by path if scope is a single turn) mirrored from the `reviewed` field returned by the API, updated optimistically on click and confirmed via `POST /changes/reviewed`.
- `expanded: HashSet<path>` — purely local UI state (not persisted), defaulting to "expanded unless reviewed."

Data fetching: since this tab needs every file's diff text upfront (not lazy per-row), it fetches once via `GET /changes` with the resolved scope (this already returns `diffText` per row per §4.1) — no need for N separate `/changes/diff` calls.

---

## 7. Implementation Steps

1. **Step 1 (Schema & DB)** ✅ COMPLETED — see §2.
2. **Step 2 (Diff Generator)** ✅ COMPLETED — see §2.
3. **Step 3 (API — turn-scoped changes + diff endpoint)** ✅ COMPLETED — see §2.
4. **Step 4 (API — reviewed state)** ⏳ PENDING:
   - Add `reviewed` column migration (§3.2).
   - Add `reviewed` to `SessionFileChangeDto` and the `GET /changes` query/response path.
   - Add `POST /api/sessions/:id/changes/reviewed` endpoint.
5. **Step 5 (Desktop — Changes tab polish)** ⏳ PENDING:
   - Add aggregate `N files changed +X -Y` header row to `ChangesListView`.
   - Add scope dropdown (This Turn / All Turns / Previous Turn) driving the existing `turnIndex` query param.
   - Add "View all" button that opens the new review tab (§6) with the current scope.
6. **Step 6 (Desktop — View all review tab)** ⏳ PENDING:
   - New tab variant in `viewer/diff_viewer.rs` (or a new file, e.g. `viewer/review_tab.rs`) rendering the stacked, accordion, mark-as-reviewed flow described in §6.
   - Reuse `DiffView` per file instead of building a new diff renderer.
   - Wire `POST /changes/reviewed` on toggle, with optimistic local update.
7. **Step 7 (Verification)** ⏳ PENDING:
   - Empty non-git directory: run *"Create a hello.txt file and write 3 lines"* → Changes tab shows `1` badge, `+3` in aggregate header.
   - Run a second prompt modifying 2 files → "This Turn" shows only those 2; "All Turns" shows the cumulative set with correct aggregate totals.
   - Open "View all" → verify every file renders with correct diff, mark one as reviewed → verify it collapses/dims and the `reviewed` flag persists across closing and reopening the tab (re-fetch reflects prior state).
   - Verify marking reviewed on a turn-scoped file, then triggering a new turn that edits the same file again, produces a fresh unreviewed row (new `turn_index`) rather than silently staying marked.
8. **Step 8 (Future phase, not scheduled)**: Git commit-history scope in the dropdown (§4.4), requiring real git subprocess diffing — deliberately deferred to avoid reintroducing the lock/volatility problems this whole plan exists to solve.
