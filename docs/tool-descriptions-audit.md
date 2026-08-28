# Tool Descriptions Audit & Minimal Implementation Guide

**Date:** 2026-08-28  
**Scope:** `apps/server/agent/src/tools/*` — all Agent-exposed tools  
**Goal:** Streamline tool descriptions to be minimal, task-focused, and free of harness internals (fff, Firecrawl, Rust, DuckDuckGo, rate limits, token coaching).

---

## 1. Principles for Minimal Descriptions

1. **Task-Focused & Intent-Driven**: One concise sentence describing *what the tool does* + *when to choose it*.
2. **Hide Harness Internals**: Never expose underlying vendors, engines, or fallback chains (`fff`, `Rust-powered`, `Firecrawl`, `DuckDuckGo`, `Brave`, etc.) to the model.
3. **No Redundant Limit Coaching**: Do not duplicate parameter constraints (e.g. `timeoutMs`, `maxEntries`, `maxBytes`, `line numbers`) in the high-level description when they are already defined in the Zod `inputSchema`.
4. **Preserve Essential Behavioral Cross-References**: Retain clear operational boundaries (e.g. `editFile` vs. `writeFile`, and using dedicated file tools rather than shell `cat`/`sed` in `bash`).

---

## 2. Audit & Specification Table

| Tool Name | Source File | Current Description (Summary) | Proposed Minimal Description | Rationale |
|---|---|---|---|---|
| **`readFile`** | `read/index.ts` | `Read the contents of a file... Lines are prefixed " 42: "... capped at 50 lines / 512 KiB... Prefer reading line ranges... For directories, use listDir instead.` | `"Read a file. Optional startLine/endLine for large files. Use for files only — use listDir for directories."` | Removes line-prefix formatting math and token coaching; preserves directory boundary guardrail. |
| **`listDir`** | `list-dir.ts` | `List files and directories... Returns tree-like structure showing names, sizes, nesting... Skips ignored trees... truncates at maxEntries.` | `"List files and directories at a path. Use recursive for subdirectories."` | Hides ignored segments and truncation ceilings (handled by input schema and backend). |
| **`glob`** | `glob.ts` | `Find files matching a glob pattern using fff — a high-performance Rust-powered file finder... 100% compatible... Background index...` | `"Find files matching a glob pattern (e.g. 'src/**/*.ts', '**/*.json')."` | Drops `fff`, Rust engine mentions, and long internal benchmarks. |
| **`grep`** | `grep.ts` | `Search file contents using fff — a Rust-powered content search engine. Faster than ripgrep... Supports plain-text, regex, fuzzy...` | `"Search file contents by pattern. Use for finding definitions, usages, or references."` | Hides engine names and mode details (the `mode` enum in `inputSchema` handles options). |
| **`writeFile`** | `write-file.ts` | `Write content to a file, fully replacing its contents... Parent directories are created automatically... For targeted changes use editFile instead...` | `"Create or overwrite a file with new content. For targeted changes, use editFile instead."` | Concise; preserves critical cross-reference to prevent accidental overwrites. |
| **`editFile`** | `edit-file.ts` | `Edit a file by replacing one specific string... oldContent must match EXACTLY... Always call readFile first... If match appears 0 times...` | `"Edit a file by replacing an exact string. Read the file first to get the exact match. For complete rewrites, use writeFile."` | Shortens while preserving exact-match requirement and read-first invariant. |
| **`batchWrite`**| `batch-write.ts` | `Write multiple files in a single call... All writes are executed concurrently... ideal for scaffolding... stopOnError...` | `"Write multiple files at once. Useful for project scaffolding or multi-file creation."` | Strips internal execution details and concurrency notes. |
| **`bash`** | `bash.ts` | `Execute a shell command... Returns stdout, stderr, exit code... Runs in /bin/sh -c... Do NOT use for file reads/writes... Capped at 50KB...` | `"Run a shell command and return output. Use for builds, tests, or git; prefer dedicated tools for reading/editing files."` | Removes shell path and buffer cap numbers; retains tool-selection boundaries. |
| **`todo`** | `todo.ts` | `Manage the session task list (TODOs) for multi-step feature development... Use 'init' with tasks:... Use 'start'... Use 'done'...` | `"Manage the session task list for multi-step work. Operations: init, start, done, append, view."` | Compact syntax summary; Zod schema describes arguments. |
| **`ask`** | `ask.ts` | `Ask the user a question to clarify ambiguous requirements... Provide a clear question and, optionally, a list of distinct option choices...` | `"Ask the user a question to clarify requirements. Optionally provide distinct choices."` | Simplifies to core interaction. |
| **`askMany`** | `ask.ts` | `Ask the user multiple questions at once, presented one at a time...` | `"Ask the user multiple questions in sequence."` | Minimal. |
| **`readSkill`** | `read-skill.ts` | `Read the full content of a skill by name. The system prompt only lists skill names... If called without a name, returns the list...` | `"Read a skill's full instructions by name. Omit name to list available skills."` | Retains optional listing feature without extra narrative. |
| **`subagent`** | `subagent.ts` | `Spawn an isolated subagent to execute a dedicated sub-task... Runs in its own memory context and returns its final summary...` | `"Delegate a focused sub-task to an isolated subagent."` | Hides memory architecture details. |
| **`webSearch`**| `web-search.ts` | `Search the web and return a list of relevant results with full markdown. Uses Firecrawl keyless search... falls back to DuckDuckGo...` | `"Search the web for up-to-date documentation and information."` | Hides Firecrawl, DuckDuckGo, and vendor fallback pipelines. |
| **`fetch`** | `fetch.ts` | `Fetch content from a URL via HTTP... For GET web pages, uses Firecrawl keyless scrape... HTML pages are automatically converted...` | `"Fetch content from a URL or web page as markdown or text."` | Hides scraping engine internals; clearly states output format. |

---

## 3. Exact Implementation Map for Agents

An implementing agent should update the `description` property on the exported `AgentTool` object in each of the following files:

### 1. `apps/server/agent/src/tools/read/index.ts`
```typescript
description: "Read a file. Optional startLine/endLine for large files. Use for files only — use listDir for directories.",
```

### 2. `apps/server/agent/src/tools/list-dir.ts`
```typescript
description: "List files and directories at a path. Use recursive for subdirectories.",
```

### 3. `apps/server/agent/src/tools/glob.ts`
```typescript
description: "Find files matching a glob pattern (e.g. 'src/**/*.ts', '**/*.json').",
```

### 4. `apps/server/agent/src/tools/grep.ts`
```typescript
description: "Search file contents by pattern. Use for finding definitions, usages, or references.",
```

### 5. `apps/server/agent/src/tools/write-file.ts`
```typescript
description: "Create or overwrite a file with new content. For targeted changes, use editFile instead.",
```

### 6. `apps/server/agent/src/tools/edit-file.ts`
```typescript
description: "Edit a file by replacing an exact string. Read the file first to get the exact match. For complete rewrites, use writeFile.",
```

### 7. `apps/server/agent/src/tools/batch-write.ts`
```typescript
description: "Write multiple files at once. Useful for project scaffolding or multi-file creation.",
```

### 8. `apps/server/agent/src/tools/bash.ts`
```typescript
description: "Run a shell command and return output. Use for builds, tests, or git; prefer dedicated tools for reading/editing files.",
```

### 9. `apps/server/agent/src/tools/todo.ts`
```typescript
description: "Manage the session task list for multi-step work. Operations: init, start, done, append, view.",
```

### 10. `apps/server/agent/src/tools/ask.ts`
```typescript
// For askTool:
description: "Ask the user a question to clarify requirements. Optionally provide distinct choices.",

// For askManyTool (if defined in same file or subagent):
description: "Ask the user multiple questions in sequence.",
```

### 11. `apps/server/agent/src/tools/read-skill.ts`
```typescript
description: "Read a skill's full instructions by name. Omit name to list available skills.",
```

### 12. `apps/server/agent/src/tools/subagent.ts`
```typescript
description: "Delegate a focused sub-task to an isolated subagent.",
```

### 13. `apps/server/agent/src/tools/web-search.ts`
```typescript
description: "Search the web for up-to-date documentation and information.",
```

### 14. `apps/server/agent/src/tools/fetch.ts`
```typescript
description: "Fetch content from a URL or web page as markdown or text.",
```

---

## 4. Verification & Testing Checklist

After updating the descriptions, the agent must run:

1. **Type Check**:
   ```bash
   cd apps/server && bunx tsc --noEmit
   ```
   *Expected: Zero type errors.*

2. **Run Tool Tests**:
   ```bash
   cd apps/server && bun tests/todo.test.ts
   ```
   *Expected: All tests pass (tests validate tool execution schemas, not description string literals).*
