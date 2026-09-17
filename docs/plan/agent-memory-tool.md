# Agent Memory Tool

Adds persistent, cross-session memory to the agent, built entirely in
TypeScript inside the agent server layer — no external service (no
Supermemory-style API), no vector DB, no embeddings. Modeled on two patterns
that already exist in this codebase:

- **Tool shape**: `todo.ts` — one tool, an `op` enum, not N separate tools.
- **Persistence shape**: `session/storage.ts` + `session/projects.ts` —
  SQLite (`bun:sqlite`), a real `projects` table keyed by UUID `id` against
  `dir`, per-project storage directories.

Target layout: `apps/server/agent/src/memory/`

---

## 0. Non-Goals

- No semantic/embedding-based search. Recall is keyword/tag matching only.
  Revisit only if plain matching proves insufficient in practice.
- No automatic/background memory writes. The agent calls `store` explicitly,
  same trigger model as Claude Code's own memory system (explicit user ask or
  a clear confirmation/correction in conversation) — the *policy* for when to
  write lives in the tool's description/prompt guidance, not in code that
  scans conversations for candidate facts.
- No automatic prompt injection of memories at session start. Recall happens
  only when the agent calls `op: "recall"`. (A future phase could add a small
  always-on summary block, but it's out of scope here — see §7.)
- No cross-tool refactor. `todo.ts` and other existing tools are untouched.

---

## 1. Design Principles

1. **One tool, not five.** A single `memory` tool with
   `op: store | recall | list | edit | delete`, matching `todo`'s `op` enum
   shape. Keeps the per-request tool-list token cost down and matches
   convention.
2. **Persisted, not session-scoped.** Unlike `todo` (in-memory, cleared per
   session), memory must survive process restarts. Backed by SQLite, not a
   closure.
3. **One `memory.db` file per project, plus one real global store.** Reuse the
   existing `projects` table's `id` to locate
   `<storage>/projects/<projectId>/memory.db`, mirroring how session messages
   already get one file per session under that same project directory. A
   separate `<storage>/memory-global.db` (added in the last phase, §5 Phase 6)
   holds facts that are genuinely cross-project (e.g., user identity,
   general preferences) — not project-code-specific. Both `recall` and `list`
   accept a `scope` filter (`"project" | "global"`) so the model can query
   either store explicitly.
4. **Isolated module boundary.** All memory logic lives under
   `agent/src/memory/`; the only external touchpoint is one import line in
   `agent/src/tools/index.ts`. No other module reaches into memory internals.
5. **Fail loud, not silent.** Missing required fields per `op` return
   `isError: true` with a specific message, mirroring `todo.ts`'s validation
   style — no silent no-ops.

---

## 2. Data Model

```ts
// apps/server/agent/src/memory/types.ts
export type MemoryScope = "project" | "global";

export interface MemoryEntry {
  id: string;            // crypto.randomUUID()
  scope: MemoryScope;     // which DB file this row lives in (see §3.1)
  content: string;
  tags: string[];
  createdAt: number;
  updatedAt: number;
}

export interface RecallMatch {
  entry: MemoryEntry;
  score: number;         // tag-match weight + substring-match weight
}
```

No separate "type" field (user/feedback/project/reference) for v1 — that
taxonomy is a Claude Code memory-system concept, not a requirement here. Tags
are freeform and cover the same need without a fixed enum. Can be added later
if the agent's own tagging proves too unstructured to filter well.

---

## 3. Module Layout

```
apps/server/agent/src/memory/
  types.ts       # MemoryEntry, MemoryScope, RecallMatch
  schema.ts      # CREATE TABLE + migration, mirrors session/schema.ts style
  storage.ts     # SqliteMemoryStorage: CRUD against the DB
  search.ts      # recallMemories(): tag/substring scoring, no embeddings
  index.ts       # barrel export — the only import surface for tools/
```

### 3.1 `schema.ts`
- **Per-project file** for `scope: "project"` rows, mirroring the existing
  per-project session layout (`<storage>/projects/<projectId>/sessions/<sessionId>.db`):
  ```
  <storage>/projects/<projectId>/memory.db     # project-scoped (Phase 1)
  <storage>/memory-global.db                   # cross-project (Phase 6)
  ```
  Each project's memories live in their own SQLite file, so deleting a
  project's directory (existing behavior when a project is removed)
  automatically deletes its memories with zero extra cleanup code. The
  global file lives at the storage root, alongside `console-global.db`, and
  is untouched by project deletion.
- Same table shape in both files — the schema doesn't need to know which
  file it's in:
  ```sql
  CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,        -- 'project' | 'global' (matches the file, kept for clarity/filtering)
    content TEXT NOT NULL,
    tags TEXT NOT NULL,         -- JSON array, e.g. '["auth","preference"]'
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
  ```
  No `project_id` column needed — the file location *is* the project scoping,
  same principle `session/storage.ts` already uses for session DBs.

### 3.2 `storage.ts`
- `class SqliteMemoryStorage`, constructed against a single file path — the
  caller decides which file (`<project>/memory.db` or the root
  `memory-global.db`) rather than the class knowing about scoping itself.
  Opens via `bun:sqlite`, `mkdirSync(..., { recursive: true })` for the
  parent dir first — separate connection from the global index DB and from
  any session DB, matching how `SqliteSessionStorage` already opens one file
  per session.
- Lazily created/opened on first use per project (same lazy-open + eviction
  pattern `session-helpers.ts` uses for cached session DBs), not eagerly for
  every project at startup. The global instance is a single long-lived
  singleton (only one file, no eviction needed).
- Methods (identical regardless of which file the instance targets):
  - `store(input: { content: string; tags: string[]; scope: MemoryScope }): MemoryEntry`
  - `get(id: string): MemoryEntry | null`
  - `list(filter?: { scope?: MemoryScope }): MemoryEntry[]`
  - `update(id: string, patch: { content?: string; tags?: string[] }): MemoryEntry | null`
  - `remove(id: string): boolean`
- A thin `memory/registry.ts` (added in Phase 6) resolves `scope` to the
  right `SqliteMemoryStorage` instance for a given `projectId`, so
  `tools/memory.ts` never opens files directly — it asks the registry for
  "the store for scope X on project Y" and calls methods on whatever it
  returns.

### 3.3 `search.ts`
- `recallMemories(entries: MemoryEntry[], query: { text?: string; tags?: string[] }, limit = 10): RecallMatch[]`
- Scoring (simple, deterministic, no external deps):
  - Exact tag match: +3 per matching tag.
  - Case-insensitive substring match of `query.text` in `content`: +1.
  - Sort descending by score, cap at `limit`, drop zero-score entries.
- Pure function, unit-testable without touching SQLite.

---

## 4. The `memory` Tool

`apps/server/agent/src/tools/memory.ts`. Imports only
`from "@/agent/src/memory/index.js"` — never touches `storage.ts` internals
or raw SQL directly.

```ts
const inputSchema = z.object({
  op: z.enum(["store", "recall", "list", "edit", "delete"])
    .describe("Operation: 'store', 'recall', 'list', 'edit', or 'delete'"),
  content: z.string().optional()
    .describe("Memory content (required for 'store'; new content for 'edit')"),
  tags: z.array(z.string()).optional()
    .describe("Tags for filtering (used with 'store', 'edit', 'recall', or 'list')"),
  scope: z.enum(["project", "global"]).optional()
    .describe(
      "Defaults to 'project'. 'project' = this project's own memory.db; " +
      "'global' = cross-project memory.db shared by every project. " +
      "Applies to 'store', 'recall', and 'list'.",
    ),
  query: z.string().optional()
    .describe("Free-text search term (used with 'recall')"),
  id: z.string().optional()
    .describe("Memory id (required for 'edit' and 'delete')"),
});
```

Behavior per `op` (validation errors follow `todo.ts`'s `isError: true`
pattern):

| op | required fields | behavior |
|---|---|---|
| `store` | `content` | Inserts a row into the store for `scope` (default `"project"`), returns the new `id` |
| `recall` | `query` and/or `tags` | Runs `search.ts` scoring over the store for `scope` (default `"project"`), returns matched entries with ids and content |
| `list` | — | Returns all entries in the store for `scope` (default `"project"`), optionally filtered by `tags`; no scoring, just a plain listing (mirrors `todo`'s `view` op) |
| `edit` | `id`, plus `content` and/or `tags` | Patches the row, bumps `updatedAt` |
| `delete` | `id` | Removes the row |

`edit`/`delete` take `scope` too (default `"project"`) since an `id` alone
doesn't say which file it lives in — the model must know (or the tool
searches both stores) since ids are only unique within a single file, not
globally. Simplest: require `scope` to already be known from a prior
`recall`/`list` call in the same turn; document this in the tool description
rather than adding cross-file id lookup.

The tool needs the current `projectId` at construction time (passed in by
whatever wires up the agent's tool list per session, same as how `todoTool`'s
controller is constructed per-session with `initialItems`) so project-scoped
ops know which project's `memory.db` to use without the model having to pass
it explicitly. The global store needs no such construction-time argument —
it's the same file for every session.

Tool description text should explicitly instruct the model on **when** to use
`store` — e.g. "Use when the user explicitly asks you to remember something,
or confirms/corrects a non-obvious approach. Do not store routine task details
or anything derivable from the code." This is the same policy Claude Code's
own memory instructions use, and it's the main lever available in a
tool-driven (not automatic) design — get this description right and iterate
on it based on observed over/under-triggering.

---

## 5. Phased Implementation

### Phase 1 — Schema & Storage
- Implement `memory/schema.ts` — `initMemoryDatabase(db)` creating the
  `memories` table (§3.1), called on first open of a project's `memory.db`.
- Implement `memory/types.ts` and `memory/storage.ts` (`SqliteMemoryStorage`),
  opening `<storage>/projects/<projectId>/memory.db` per project, with
  lazy-open + eviction following `session-helpers.ts`'s cached-DB pattern.
- **Verification**: `apps/server/tests/memory-storage.test.ts` — CRUD against
  a temp-dir-backed instance (same `:memory:`-adjacent pattern
  `SqliteSessionStorage` tests use via a temp storage dir), plus a check that
  deleting a project's directory removes its `memory.db` with it.

### Phase 2 — Recall Scoring
- Implement `memory/search.ts` as a pure function, independent of storage.
- **Verification**: `apps/server/tests/memory-search.test.ts` — tag-match
  scoring, substring scoring, combined scoring, limit truncation, zero-score
  exclusion. No DB involved.

### Phase 3 — The Tool
- Implement `tools/memory.ts` per §4, constructed with `{ storage, projectId }`.
- Wire into `apps/server/agent/src/tools/index.ts`:
  - `export * from "./memory.js";`
  - Add `memoryTool` (or a `createMemoryTool(...)` factory, since it needs
    per-session `projectId` — check whether other per-session tools like
    `todoTool` are instantiated per-session elsewhere and follow that wiring
    point rather than adding it to the static `allTools` array if it needs
    construction-time args).
- **Verification**: `apps/server/tests/memory-tool.test.ts` — each `op`'s
  success path and each validation-error path (missing `content`, missing
  `id`, missing `query`/`tags`).

### Phase 4 — Prompt Guidance
- Write the tool `description` field carefully per §4's last paragraph.
- No changes to `systemprompt/builder.ts` — deliberately out of scope per §0.
- **Verification**: manual — run a session, ask the agent to remember
  something, start a fresh session in the same project, ask a related
  question, confirm the agent calls `recall` and surfaces the fact. Also
  verify it does *not* call `store` for routine, non-memorable requests.

### Phase 5 — Cleanup & Docs
- Confirm cascade delete works end-to-end when a project is deleted via the
  existing project-deletion path.
- Add a short section to any relevant internal docs describing the `memory`
  tool for future contributors (not user-facing docs unless requested).

### Phase 6 — Global Memory (last)
Deliberately last: ship project-scoped memory first, prove the tool/recall
loop works, then extend to a second store rather than building both at once.

- Implement `memory/registry.ts`: resolves `scope` + `projectId` to the right
  `SqliteMemoryStorage` instance —
  - `scope: "project"` → lazily opened `<storage>/projects/<projectId>/memory.db`
    (same instance Phase 1 already built).
  - `scope: "global"` → a single lazily-opened singleton at
    `<storage>/memory-global.db`, reusing the same `schema.ts`/`storage.ts`
    with no code changes, just a different path.
- Update `tools/memory.ts` to route `store`/`recall`/`list`/`edit`/`delete`
  through the registry instead of a single hardcoded project store.
- Update the tool description to explain the project/global distinction
  clearly enough that the model defaults to `"project"` and only reaches for
  `"global"` for genuinely cross-project facts (mirrors the user-memory vs.
  project-memory distinction Claude Code's own memory docs draw).
- **Verification**: `apps/server/tests/memory-registry.test.ts` — resolves to
  distinct storage instances per scope, global instance is shared across
  different `projectId`s, global file is unaffected by project deletion.
  Extend `memory-tool.test.ts` with `scope: "global"` cases for each op.

---

## 6. Open Questions (resolve before/at Phase 1)

- **Recall result cap**: is 10 the right default limit, or should it be
  tunable per call via the tool input?
- **Cross-scope `edit`/`delete`**: per §4, `id`s are only unique within a
  single file. Is "the model must know/pass the correct `scope`" acceptable,
  or should `edit`/`delete` fall back to checking both stores when `scope` is
  omitted (slightly friendlier, slightly more code)? Recommend requiring
  explicit `scope` for v1 (simpler) and revisit if this causes real friction.
- **Tag vocabulary**: freeform tags (per §2) vs. a constrained enum — freeform
  is simpler now but may fragment (`"auth"` vs `"authentication"`) over time.

---

## 7. Explicitly Deferred (not part of this plan)

- Embedding-based semantic recall.
- Automatic memory-write triggers (scanning conversation for candidate facts
  without an explicit tool call).
- Always-on memory summary injected into every system prompt.
- A memory taxonomy (`user`/`feedback`/`project`/`reference` types).
- Any UI for browsing/editing memories outside of the agent tool itself.
- Blending project + global results into a single `recall`/`list` call —
  each call targets exactly one scope (§4); the model issues two calls if it
  needs both.

---

## 8. Verification Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| **Storage CRUD** | `bun tests/memory-storage.test.ts` | store/get/update/delete round-trip correctly against a temp-dir instance |
| **Cascade delete** | same file | deleting a project removes its `memory.db` |
| **Recall scoring** | `bun tests/memory-search.test.ts` | tag/substring/combined scoring ranks and caps correctly |
| **Tool validation** | `bun tests/memory-tool.test.ts` | each `op` errors cleanly on missing required fields |
| **Tool success paths** | same file | `store` returns id; `recall`/`list` return matches; `edit`/`delete` mutate correctly |
| **Cross-session recall** | manual | fact stored in session A is recalled in session B, same project |
| **Store-trigger discipline** | manual | agent does not call `store` for routine, non-memorable requests |
| **Registry resolution** | `bun tests/memory-registry.test.ts` | `scope: "project"` and `scope: "global"` resolve to distinct, correctly-pathed stores |
| **Global persistence** | manual | fact stored with `scope: "global"` in project A is recalled with `scope: "global"` in project B |
| **Global survives project deletion** | manual | deleting project A does not remove global memories |
