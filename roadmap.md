# Console Agent Engine — Architecture & Implementation Roadmap

> **Target Architecture:** Headless Server Engine (Hono HTTP + Real-Time SSE) + Remote Client (Mobile / Desktop App)
> **Reference:** `oh-my-pi/packages/coding-agent` & `oh-my-pi/packages/agent`

---

## 🎯 Architecture Overview

```
┌────────────────────────────────────────────────────────┐
│               Remote Client (Mobile / Desktop UI)       │
│  - Project Explorer  - Session List  - Chat & Tools    │
│  - Diff Viewer       - Terminal UI   - Mobile OAuth    │
└───────────────────────────┬────────────────────────────┘
                            │ HTTP REST + Real-Time SSE
┌───────────────────────────▼────────────────────────────┐
│              Console Agent Server (Hono Node.js)       │
│                                                        │
│ ┌──────────────────┐  ┌──────────────────────────────┐ │
│ │  Project & FS    │  │  Remote Headless OAuth       │ │
│ │  Explorer API    │  │  (URL Gen + Code Exchange)   │ │
│ └────────┬─────────┘  └──────────────┬───────────────┘ │
│          │                           │                 │
│ ┌────────▼─────────┐  ┌──────────────▼───────────────┐ │
│ │  SQLite Storage  │  │  Agent Loop & Tool Harness   │ │
│ │  (Sessions & DB) │  │  (13 Tools + Security Modes) │ │
│ └──────────────────┘  └──────────────┬───────────────┘ │
│                                      │                 │
│                       ┌──────────────▼───────────────┐ │
│                       │ Provider Layer (CCA / REST)  │ │
│                       │ (Gemini & Antigravity SSE)   │ │
│                       └──────────────────────────────┘ │
└────────────────────────────────────────────────────────┘
```

---

## ✅ Completed Phases (100% DONE)

### Phase 1 — Agent Harness Core

- [x] **Types** — `AgentMessage`, `AgentTool`, `AgentSessionEvent`, `Model`, `SessionHeader`, `ThinkingPart`
- [x] **EventStream** — self-contained async event queue (`push`, `fail`, `for-await-of`, `result()`)
- [x] **AgentLoop** — provider-agnostic turn & tool execution cycle (`Promise.all` concurrent tools)
- [x] **Agent Class** — stateful history management, `run()`, `abort()`, `clearHistory()`, `loadHistory()`
- [x] **StreamFn Transport Abstraction** — clean separation between LLM streaming and agent loop

### Phase 2 — Core Tool Suite (13 Local Tools)

- [x] `readFile` — line range selection & line numbering
- [x] `writeFile` — file creation & overwrite with recursive directory creation
- [x] `editFile` — exact-string replacement with single-match assertion
- [x] `batchWrite` — atomic concurrent multi-file writer ⭐ custom
- [x] `listDir` — formatted directory tree with file sizes
- [x] `glob` — `@ff-labs/fff-node` Rust-native pattern matcher
- [x] `grep` — `@ff-labs/fff-node` Rust-native regex/plain text search
- [x] `bash` — shell execution with timeout & 50KB output cap
- [x] `webSearch` — DuckDuckGo / Brave search API
- [x] `fetch` — HTTP fetching with HTML to Markdown conversion & JSON formatting
- [x] `todo` — multi-step task list tracking (`init`, `start`, `done`, `append`, `view`)
- [x] `task` — subagent task runner executing child loops in isolated memory context
- [x] `ask` — interactive multiple-choice question tool

### Phase 3 — Context Discovery & System Prompt Builder

- [x] **Context File Discovery** — walk up from `cwd` collecting `AGENTS.md`, `CLAUDE.md`, `.cursorrules`
- [x] **Rule & Skill Discovery** — `.agent/rules/*.md` (always-apply vs domain rules) & `.agent/skills/`
- [x] **Slash Command Discovery** — `.agent/commands/*.md` user-defined slash commands
- [x] **SystemPromptBuilder** — layered assembly (Identity → Plan Mode → Skills → Rules → Tools → Commands → Repo Context → Tree → Workstation Env → Append Prompt)

### Phase 4 — Session Persistence (SQLite)

- [x] **Database Schema** — `sessions` table (`id`, `title`, `cwd`, `model_id`, `provider`, `created_at`, `updated_at`) & `messages` table (`id`, `session_id`, `role`, `content` JSON, `created_at`)
- [x] **SqliteSessionStorage** — `createSession()`, `appendMessage()`, `appendMessages()`, `loadSession()`, `listSessions()`, `deleteSession()`, `updateTitle()`, `updateModel()`
- [x] **WAL Mode & In-Memory Mode** — zero-latency synchronous `better-sqlite3` storage

### Phase 5 — OAuth Providers & User-Agent Centralization

- [x] **Gemini CLI StreamFn** — targets `cloudcode-pa.googleapis.com` endpoint
- [x] **Antigravity StreamFn** — targets `daily-cloudcode-pa.googleapis.com` endpoint with session envelope (`agentId`, `trajectoryId`, `sessionId`, `stepIndex`)
- [x] **Constants & User-Agent Centralization** — shared headers, client IDs, base URLs, and runtime secret decoding in `constants.ts`

### Phase 6 — Slash Commands Engine & Dynamic Provider Registry

- [x] **Provider Registry** — catalog of providers (`gemini`, `antigravity`) with dynamic endpoint discovery (`/v1internal:fetchAvailableModels`) and static bundled fallbacks
- [x] **SlashCommandRegistry** — parse `/command [args]` before entering turn loop
- [x] **Built-in Commands** — `/model`, `/provider`, `/mode`, `/new`, `/resume`, `/rename`, `/compact`, `/help`
- [x] **Offline Test Suite** — comprehensive 100% offline unit tests (`npm test`, 0 LLM credits used)

### Phase 7 — Project & Filesystem Exploration API (`/api/fs/*`, `/api/projects`)

- [x] **Task 7.1: Project Directory Explorer (`GET /api/projects`, `POST /api/projects`)**
  - List recent server project folders and select custom system folders as active workspace projects
- [x] **Task 7.2: Filesystem Browser & Tree (`GET /api/fs/browse`, `GET /api/fs/tree`)**
  - System directory navigation with parent path for mobile file pickers and tree summary
- [x] **Task 7.3: File Content & Directory Operations (`GET /api/fs/file`, `POST /api/fs/file`, `DELETE /api/fs/file`, `POST /api/fs/dir`, `DELETE /api/fs/dir`)**
  - Read, write, delete files, and create/delete directories
- [x] **Task 7.4: Decoupled Service Layer (`server/api/src/services/`)**
  - `FsService`, `ProjectService`, `AuthService`, `ProviderService`, `SessionService`, `RunService`

### Phase 8 — Remote Headless OAuth API (`/api/auth/*`)

- [x] **Task 8.1: Auth Status Endpoint (`GET /api/auth/status`)**
  - Check current stored credentials for `gemini` and `antigravity`
- [x] **Task 8.2: Remote OAuth URL Generator (`POST /api/auth/login/url`)**
  - Generates authorization URL + state for remote mobile/desktop browser sign-in
- [x] **Task 8.3: Auth Code Callback & Token Exchange (`POST /api/auth/login/callback`)**
  - Exchanges authorization code for tokens, runs project discovery, saves credentials to disk

### Phase 9 — Hono REST & Real-Time SSE API Layer (`/api/sessions/*`, `/api/providers/*`)

- [x] **Task 9.1: Provider & Model Catalog Routes (`GET /api/providers`, `GET /api/providers/:id/models`)**
  - Expose available providers and dynamic models
- [x] **Task 9.2: Session Management CRUD (`GET /api/sessions`, `POST /api/sessions`, `GET /api/sessions/:id`, `PATCH /api/sessions/:id`, `DELETE /api/sessions/:id`)**
  - Complete CRUD endpoints backed by `SqliteSessionStorage`
- [x] **Task 9.3: Real-Time Agent Run Endpoint (`POST /api/sessions/:id/run`)**
  - Streams lifecycle events over SSE (`text/event-stream`) and auto-saves turns to SQLite
- [x] **Task 9.4: Abort Run Route (`POST /api/sessions/:id/abort`)**
  - Cancels active agent run via `AbortController`

### Phase 10 — Context Window Management & Auto-Compaction

- [x] **Task 10.1: Token Estimator (`estimateMessageTokens`)**
  - Heuristic token counter for `AgentMessage[]` history (~4 chars per token)
- [x] **Task 10.2: Automated Compaction Trigger (`compactHistory`)**
  - Automatically triggers summarization checkpoint when history exceeds context threshold (e.g. 80%)

### Phase 11 — Permissions, Security Modes & Extended Thinking

- [x] **Task 11.1: Security Approval Modes (`always-ask`, `accept-edits`, `plan-mode`, `full-access`)**
  - Tiered capability approval (`read`, `write`, `exec`) with `permissionRequest` lifecycle events
- [x] **Task 11.2: Plan Mode System Prompt & Read-Only Guard**
  - Automatic Plan Mode instruction injection and read-only working tree protection
- [x] **Task 11.3: Extended Thinking Parser**
  - Segregates model thinking deltas (`{ type: "thinking" }`) and `<thinking>...</thinking>` tags from visible text

---

## 📅 Execution Status Summary

```
Phase 1-6 (Core Harness, Tools, Prompt Builder, SQLite, Providers, Commands) ─────► ✅ COMPLETED
Phase 7 (Project & FS Exploration API)                                       ─────► ✅ COMPLETED
Phase 8 (Remote Headless OAuth API for Mobile/Desktop)                        ─────► ✅ COMPLETED
Phase 9 (Hono REST & Real-Time SSE API)                                      ─────► ✅ COMPLETED
Phase 10 (Context Compaction & Token Management)                             ─────► ✅ COMPLETED
Phase 11 (Permissions, Security Modes & Extended Thinking)                  ─────► ✅ COMPLETED
```
