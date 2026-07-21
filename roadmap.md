# Console Agent Harness — Roadmap

> **Reference:** `oh-my-pi/packages/coding-agent` and `oh-my-pi/packages/agent`

---

## ✅ Done

### Phase 0 — Foundation
- [x] TypeScript 7.0.2 (Go-native compiler), tsx 4.23.1
- [x] `tsconfig.json` migrated (removed `baseUrl`, added `"types": ["node"]`)

### Phase 1 — Agent Harness Core
- [x] **Types** — `AgentMessage`, `AgentTool`, `AgentSessionEvent`, `Model`, `SessionContext`
- [x] **EventStream** — self-contained async event queue (push / fail / for-await-of / result())
- [x] **AgentLoop** — `agentLoop()` / `agentLoopContinue()` — provider-free turn + tool cycle
- [x] **Agent class** — stateful history, run(), abort(), loadHistory(), AgentBusyError
- [x] **StreamFn interface** — injected LLM streaming abstraction (no provider code in loop)

### Phase 2 — Tools (10 tools)
- [x] `readFile` — file reading with numbered lines + line ranges
- [x] `listDir` — recursive directory tree with sizes
- [x] `writeFile` — create/overwrite files, auto-creates dirs
- [x] `editFile` — exact-string replacement (enforces 1 match)
- [x] `batchWrite` — concurrent multi-file write with per-file results ⭐ custom
- [x] `glob` — fff-powered (Rust native, warm-indexed, npm-glob compatible)
- [x] `grep` — fff-powered (plain / regex / fuzzy modes)
- [x] `bash` — shell execution with timeout + 50KB output cap
- [x] `webSearch` — DuckDuckGo (no key) or Brave (BRAVE_SEARCH_API_KEY)
- [x] `fetch` — HTTP with HTML→text, JSON pretty-print, 512KB cap

---

## 🔜 Next Up

### Phase 3 — Context Loading & System Prompt Builder
*Mirrors `oh-my-pi/packages/coding-agent/src/discovery/` and `system-prompt.ts`*

The system prompt is not static. It is assembled at run-time from layered sources:

- [ ] **AGENTS.md discovery** — walk up from `cwd` to repo root, collect all `AGENTS.md` files
  - User-level: `~/.agent/AGENTS.md`, `~/.agents/AGENTS.md`
  - Project-level: `.agent/AGENTS.md`, `.agents/AGENTS.md` at each ancestor
- [ ] **SYSTEM.md discovery** — same walk-up for `.agent/SYSTEM.md` overrides
- [ ] **Rules loading** — `.agent/rules/*.md` (always-apply behaviour rules injected into prompt)
- [ ] **Skills loading** — `.agent/skills/*.md` (instruction sets loaded on demand or always)
- [ ] **Slash commands loading** — `.agent/commands/*.md` (user-defined `/commands`)
- [ ] **SystemPromptBuilder** — assemble final prompt from:
  1. Core identity / personality block
  2. Current date, cwd, git branch, OS
  3. Workspace tree summary (top-level files + dirs)
  4. Collected AGENTS.md context files (deduplicated)
  5. Always-apply rules
  6. Custom SYSTEM.md overrides

### Phase 4 — Session Persistence (SQLite)
*Mirrors `oh-my-pi/packages/coding-agent/src/session/sql-session-storage.ts` and `session-storage.ts`*

`better-sqlite3` is already installed.

- [ ] **Schema** — `sessions` table (id, title, created_at, updated_at, cwd, model)
          `messages` table (id, session_id, role, content JSON, created_at)
- [ ] **SessionStorage** — `createSession()`, `appendMessage()`, `loadSession()`, `listSessions()`, `deleteSession()`
- [ ] **Agent integration** — `Agent.run()` auto-persists each message to the active session
- [ ] **Session resumption** — `Agent.loadSession(id)` restores history from DB

### Phase 5 — Gemini / Antigravity StreamFn Client
*The last piece needed before the agent can actually call an LLM*

- [x] **GeminiStreamFn** — implements `StreamFn` using REST CCA API
  - Converts `AgentMessage[]` → Gemini `Content[]` format
  - Maps Gemini tool declarations from `AgentTool[]`
  - Streams deltas as `LLMDelta` (`{ type: 'text' }` or `{ type: 'toolCall' }`)
  - Handles `AbortSignal`
  - OAuth2 flow with auto-login if no credentials
- [x] **Antigravity StreamFn** — same interface, targets the Antigravity API
  - Session envelope (stable state per StreamFn)
  - Antigravity-specific system instruction
  - OAuth2 flow with auto-login if no credentials
- [x] **OAuth Config** — centralized constants, token refresh, credential storage in `~/.console/`

### Phase 6 — Slash Commands & Conversation Commands
*Mirrors `oh-my-pi/packages/coding-agent/src/slash-commands/builtin-registry.ts`*

- [ ] **SlashCommandRegistry** — parse `/command [args]` from user input before it hits the loop
- [ ] **Built-in commands:**
  - `/clear` — clear conversation history
  - `/compact` — summarise and compress history (context window management)
  - `/model <id>` — switch model mid-session
  - `/tools` — list available tools
  - `/sessions` — list saved sessions
  - `/resume <id>` — resume a saved session
  - `/export` — dump session as JSON or Markdown

### Phase 7 — Context Window Management (Compaction)
*Mirrors `oh-my-pi/packages/agent/src/compaction/`*

- [ ] **Token counting** — estimate token usage of current `messages[]`
- [ ] **Compaction trigger** — when context exceeds N% of model's `contextWindow`, summarise
- [ ] **Summary injection** — replace old turns with a `[Summary]` assistant message
- [ ] **Configurable threshold** — per-model, defaults to 80% of contextWindow

### Phase 8 — Hono HTTP API Layer
*Wire the harness into the existing Hono server*

- [ ] `POST /sessions` — create session
- [ ] `GET /sessions` — list sessions
- [ ] `POST /sessions/:id/run` — stream run (SSE or NDJSON)
- [ ] `GET /sessions/:id/messages` — get history
- [ ] `DELETE /sessions/:id` — delete session
- [ ] `POST /sessions/:id/abort` — abort current run

---

## Deferred / Future

- [ ] **MCP server support** — expose tools as an MCP server for other agents to call
- [ ] **Eval harness** — run agent against test cases, score outputs
- [ ] **Memory/Autolearn** — surface important facts back into system prompt across sessions
- [ ] **Streaming compaction** — `oh-my-pi`-style `snapcompact` for lossless context compression
- [ ] **Multi-agent / Swarm** — parallel sub-agents with shared tool registry

---

## Replace via Roadmap

- [x] `glob` and `grep` → replaced with `@ff-labs/fff-node` (Rust native)