# Harness Tool & Schema Improvements

**Status:** Draft
**Date:** 2026-09-09
**Goal:** Minimalist tool set that covers all high-frequency agent workflows while reducing bash reliance.

---

## 1. Current State Assessment

### Tools That Work Well (No Changes Needed)

| Tool | Strength |
|------|----------|
| `readFile` | Line ranges + encoding — covers most needs |
| `writeFile` | Auto-creates directories — clean |
| `editFile` | Exact string replacement — safe for targeted changes |
| `batchWrite` | Parallel writes — good for scaffolding |
| `glob` | Indexed, fast — better than bash find |
| `grep` | Indexed, fast — better than bash grep |
| `listDir` | Recursive + depth control — clean |
| `bash` | Nuclear option — covers everything else |
| `todo` | Task tracking — helps multi-step work |
| `subagent` | Parallel delegation — useful |
| `ask` / `askMany` | Clarification — essential |
| `webSearch` / `fetch` | External info — good |

### Current Gaps

| Gap | Impact |
|-----|--------|
| No `git` tool | Every git operation requires bash — verbose, unstructured output |
| No `fileExists` check | Agents read files just to check if they exist |
| No safe `delete` | `rm` via bash is risky |
| No `rename`/`move` | Agents use `bash mv` for every rename |
| No `diff` | No way to compare files/versions without bash |
| `editFile` fragility | Exact string match fails if whitespace shifts slightly |
| `todo` index-based | Clunky task tracking — no task IDs |
| `bash` no structured output | Raw stdout/stderr — hard to parse |

---

## 2. Proposed New Tools

### 2.1 `git` — Structured Git Operations

**Why:** 80% of agent bash usage is git commands. A structured tool with JSON output would be cleaner, safer, and easier to parse.

**Schema:**

```typescript
git({
  action: "status" | "log" | "diff" | "show" | "branch" | "rev-parse" | "stash",
  // action-specific options:
  ref?: string,           // for log/show/diff
  file?: string,          // for status/diff specific file
  staged?: boolean,       // for diff --staged
  count?: number,         // for log --oneline -n
  message?: string,       // for stash push -m
  json?: boolean,         // default true — return structured JSON
})
```

**Examples:**

```typescript
// git status
git({ action: "status" })
// → { branch: "main", staged: [...], unstaged: [...], untracked: [...] }

// git log
git({ action: "log", count: 5, json: true })
// → [{ hash: "abc123", message: "feat: add X", author: "...", date: "..." }]

// git diff
git({ action: "diff", staged: true })
// → { files: [{ path: "src/foo.ts", additions: 5, deletions: 2, hunks: [...] }] }

// git show
git({ action: "show", ref: "HEAD" })
// → { hash: "abc123", message: "...", diff: "..." }
```

**Safety:** Read-only by default. Commit/push not included — those should remain explicit bash commands or separate actions added later.

---

### 2.2 `fileExists` — Quick Existence Check

**Why:** Agents often check if a file exists before reading it. Currently requires `glob` (overkill) or `bash test -f` (unstructured).

**Schema:**

```typescript
fileExists(path: string) → { exists: boolean, isFile: boolean, isDirectory: boolean }
```

**Examples:**

```typescript
fileExists("apps/server/package.json")
// → { exists: true, isFile: true, isDirectory: false }

fileExists("apps/server/src/nonexistent.ts")
// → { exists: false, isFile: false, isDirectory: false }
```

**Implementation:** Use `fs.stat` with try/catch for the `exists` check.

---

### 2.3 `delete` — Safe File Removal

**Why:** `rm` via bash is irreversible. A dedicated tool with trash support reduces risk.

**Schema:**

```typescript
delete({
  path: string,
  recursive?: boolean,    // for directories — default false
  trash?: boolean,        // move to trash instead of delete — default true
}) → { deleted: boolean, path: string }
```

**Examples:**

```typescript
// Safe delete (moves to trash)
delete({ path: "src/old-file.ts" })
// → { deleted: true, path: "src/old-file.ts" }

// Permanent delete (requires explicit opt-in)
delete({ path: "src/old-file.ts", trash: false })
// → { deleted: true, path: "src/old-file.ts" }

// Delete directory
delete({ path: "src/old-dir", recursive: true })
// → { deleted: true, path: "src/old-dir" }
```

**Safety:** Default to trash. Permanent delete requires `trash: false`.

---

### 2.4 `rename` — File/Directory Rename

**Why:** Every rename currently requires `bash mv`. A structured tool handles errors and edge cases.

**Schema:**

```typescript
rename({
  from: string,
  to: string,
  overwrite?: boolean,    // default false — fail if destination exists
}) → { renamed: boolean, from: string, to: string }
```

**Examples:**

```typescript
rename({ from: "src/old.ts", to: "src/new.ts" })
// → { renamed: true, from: "src/old.ts", to: "src/new.ts" }

rename({ from: "src/a.ts", to: "src/b.ts", overwrite: true })
// → { renamed: true, from: "src/a.ts", to: "src/b.ts" }
```

---

### 2.5 `diff` — Compare Files/Strings

**Why:** No way to see what changed between two versions without bash. Useful for verifying edits.

**Schema:**

```typescript
diff({
  a: string,              // file path or string content
  b: string,              // file path or string content
  format?: "unified" | "side-by-side" | "stat",  // default "unified"
}) → { changes: string, additions: number, deletions: number }
```

**Examples:**

```typescript
// Compare two files
diff({ a: "src/foo.ts", b: "src/bar.ts" })

// Compare current vs previous version
diff({ a: "HEAD:src/foo.ts", b: "src/foo.ts" })

// Just stats
diff({ a: "src/foo.ts", b: "src/bar.ts", format: "stat" })
// → { changes: "...", additions: 10, deletions: 3 }
```

---

## 3. Proposed Improvements to Existing Tools

### 3.1 `editFile` — Add Line-Number Fallback

**Current Problem:** Exact string match fails if whitespace shifts. Agent has to re-read and try again.

**Proposed Change:** Add optional line-number targeting.

```typescript
editFile({
  path: string,
  // Option A: exact string (current)
  oldContent?: string,
  newContent?: string,
  // Option B: line range (new)
  startLine?: number,
  endLine?: number,
  replacement?: string,
})
```

**Priority:** Low — current approach works 95% of the time. Nice-to-have.

---

### 3.2 `bash` — Add Structured Output Option

**Current Problem:** Raw stdout/stderr is hard to parse for simple commands.

**Proposed Change:** Add optional JSON output for known commands.

```typescript
bash({
  command: string,
  json?: boolean,         // attempt to parse output as JSON
  timeoutMs?: number,
  background?: boolean,
})
```

**Priority:** Low — `git` tool covers the most common case. bash remains the escape hatch.

---

### 3.3 `todo` — Add Task IDs

**Current Problem:** Index-based start/done is clunky.

**Proposed Change:** Auto-generate task IDs (e.g., `task-1`, `task-2`).

```typescript
todo({ op: "init", tasks: ["a", "b", "c"] })
// → { tasks: [{ id: "task-1", text: "a", status: "pending" }, ...] }

todo({ op: "start", id: "task-1" })
// → { task: { id: "task-1", text: "a", status: "in-progress" } }

todo({ op: "done", id: "task-1" })
// → { task: { id: "task-1", text: "a", status: "done" } }
```

**Priority:** Medium — current system works but is awkward.

---

## 4. Implementation Priority

| Priority | Tool/Change | Effort | Impact |
|----------|-------------|--------|--------|
| **P0** | `git` | Medium | High — covers 80% of bash usage |
| **P1** | `fileExists` | Low | Medium — prevents unnecessary reads |
| **P1** | `delete` | Low | Medium — safer than bash rm |
| **P2** | `rename` | Low | Low-Medium — cleaner than bash mv |
| **P2** | `diff` | Medium | Low-Medium — useful for verification |
| **P3** | `todo` IDs | Low | Low — quality of life |
| **P3** | `editFile` line numbers | Low | Low — nice-to-have |
| **P3** | `bash` json option | Low | Low — edge case |

---

## 5. Minimalist Recommendation

If you want the smallest possible addition with maximum impact:

**Add only `git` + `fileExists`.**

These two tools cover the most frequent bash calls and would reduce bash usage by ~60-70%. Everything else is incremental improvement.

---

## 6. Open Questions

1. Should `git` include write actions (commit, push, branch create/delete) or remain read-only?
2. Should `delete` default to trash or permanent? (trash is safer but platform-dependent)
3. Should `diff` support comparing strings directly, or only files?
4. Is the `todo` index-based system actually a problem, or just unfamiliar?

---

## 7. Next Steps

- [ ] Review this plan with stakeholders
- [ ] Decide on priority tier (P0 only? P0+P1? All?)
- [ ] Finalize schema definitions
- [ ] Implement selected tools
- [ ] Update AGENTS.md with new tool documentation
