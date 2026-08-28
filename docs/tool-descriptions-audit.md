# Tool Descriptions Audit — Current vs Minimal

**Date:** 2026-08-28
**Scope:** `apps/server/agent/src/tools/*` — all Agent-exposed tools
**Goal:** Keep descriptions minimal and task-focused; hide harness internals (fff, Firecrawl, Rust, DuckDuckGo, etc.).

## Principles for Minimal Descriptions
- One sentence what it does + when to use it.
- No engine names, no fallback chains, no rate-limit details, no token math.
- No harness paths, no file counts, no engine-specific examples beyond file patterns.

## Audit Table

| Tool | Current (truncated) | Proposed Minimal | Reason |
|---|---|---|---|
| **readFile** | `Read the contents of a file… Supports line range… Lines are prefixed " 42: …" matching cat -n… capped at 50 lines / 512 KiB … Always prefer reading specific line ranges… For directories, use listDir instead.` | `Read a file. Optional startLine/endLine for large files. Use for files only — use listDir for directories.` | Removes cap numbers, cat -n detail and token-economy coaching; keeps core. |
| **listDir** | `List files and directories… Returns tree-like structure showing names, sizes, and nesting… Skips ignored trees (node_modules, .git, dist, etc.) when recursive, truncates at maxEntries.` | `List files and directories at a path. Use recursive for subdirectories.` | Hides ignored-list and truncation — harness detail. |
| **glob** | `Find files matching a glob pattern using fff — a high-performance Rust-powered file finder… 100% compatible… Powered by native background index so repeat calls are near-instant. Common examples: "src/**/*.ts" …` | `Find files by glob pattern (e.g. "src/**/*.ts", "**/*.json").` | Drops fff/Rust/index internals and long examples. |
| **grep** | `Search file contents using fff — a Rust-powered content search engine. Faster than ripgrep … Supports plain-text, regex, fuzzy… Returns matching lines with surrounding context, grouped by file.` | `Search file contents by pattern. Use for finding definitions, usages, or TODOs.` | Hides engine and mode details — inputs still expose `mode` enum. |
| **writeFile** | `Write content to a file, fully replacing its contents… Parent directories are created automatically… Use for creating new files or completely rewriting… For targeted changes… use editFile instead…` | `Create or overwrite a file with new content.` | Keeps contrast with `editFile` in one line. |
| **editFile** | `Edit a file by replacing one specific string… oldContent must match EXACTLY… Always call readFile first… If match appears 0 times or more than once… For complete rewrites, use writeFile.` | `Edit a file by replacing an exact string. Read the file first to get the exact match.` | Shortens but keeps exact-match and read-first guard. |
| **batchWrite** | `Write multiple files in a single call… All writes are executed concurrently… ideal for scaffolding… Reports individual results… Set stopOnError: true if you need atomic-style… Validates that no two entries share same path…` | `Write multiple files at once.` | Removes concurrency/atomic and validation internals — inputs still enforce it. |
| **bash** | `Execute a shell command… Returns stdout, stderr, and exit code… The command runs in a shell (/bin/sh -c), so pipes… Use for: running tests, builds, git… Do NOT use for file reads/writes… Do NOT use for code/file searching… Commands are killed after timeoutMs… stdout and stderr are each capped at 50KB.` | `Run a shell command and return output.` | Drops shell, cap numbers, and “do not use” coaching — keep tool choice in minimal form. |
| **todo** | `Manage the session task list (TODOs) for multi-step feature development… Use 'init' with tasks: ["task 1",…] to initialize… Use 'start' with index: 1 … Use 'done' … Use 'append' … Use 'view' …` | `Manage the task list for multi-step work. Use init/start/done/append/view.` | Shortens to ops only. |
| **ask** | `Ask the user a question to clarify ambiguous requirements… Provide a clear question and, optionally, a list of distinct option choices. Set skippable to false if strictly required.` | `Ask the user a question. Optionally provide choices.` | Removes requirement nuance (input still documents `skippable`). |
| **askMany** | `Ask the user multiple questions at once, presented one at a time… Each question may have optional multiple-choice options…` | `Ask multiple questions at once.` | Minimal. |
| **readSkill** | `Read the full content of a skill by name. The system prompt only lists skill names… Call this tool when a skill matches… If called without a name, returns the list…` | `Read a skill by name. Omit name to list available skills.` | Keeps optional-name behavior. |
| **subagent** | `Spawn an isolated subagent to execute a dedicated sub-task (e.g. searching files… ) The subagent runs in its own memory context and returns its final summary…` | `Delegate a focused sub-task to a subagent.` | Hides memory-context detail. |
| **webSearch** | `Search the web and return a list of relevant results with full markdown. Uses Firecrawl keyless search (no API key) for JS-rendered, clean markdown; falls back to DuckDuckGo Lite on rate limit / network error…` | `Search the web for up-to-date information.` | **Key fix** — hides Firecrawl/DuckDuckGo fallback chain per your request. |
| **fetch** | `Fetch content from a URL… For GET web pages, uses Firecrawl keyless scrape (JS-rendered, clean markdown) with fallback to direct fetch + HTML strip…` | `Fetch content from a URL.` | **Key fix** — hides Firecrawl scrape internals. |
| **readSkill** (already minimal) | — | — | No change needed. |

## Notes
- All `inputSchema` descriptions (e.g., `maxDepth`, `maxEntries`, `timeoutMs`) already convey limits — no need to duplicate in tool `description`.
- `glob`/`grep` examples like `"src/**/*.ts"` can stay in `inputSchema` for `pattern`, not in tool `description`.
- After this audit, the next step is to patch `web-search.ts:158` and `fetch.ts:61` to the minimal lines above and optionally trim the other 13 tools in one pass.

## Verification
- `bunx tsc --noEmit` clean
- `bun tests/tools.test.ts` listDir/readFile etc. still pass — they don't assert on description strings
- New `tests/web-search.test.ts` will assert keyless Firecrawl request and fallback (see spec 5)
