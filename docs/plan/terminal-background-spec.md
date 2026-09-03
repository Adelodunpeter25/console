# Terminal Background Execution Spec

## Goal
Allow terminal tabs (PTYs) to keep running when hidden/backgrounded, with detach/reattach. Most concerned: long `bun`/`npm`/`cargo` commands. Subagent background is out of scope for this spec (see §7).

## Current State
* `apps/server/api/src/terminal/pty.manager.ts` owns PTYs — `spawn`, `attach`, `write`, `resize`, `kill`, `pause`/`resume`. `close` kills the PTY (`socket.route.ts:149-154`). No background concept; losing the WS kills the shell.
* `apps/desktop/crates/console-core/src/types/workspace.rs:23` `WorkspaceTabConfig::Terminal { terminal_id, title }` + `apps/desktop/src/state/workspace_panes.rs:465` creates terminal tabs. Desktop renders via WS to `/api/terminals`.
* Agent runs block the pane (`ConsoleDesktopApp::running_sessions`); terminals are orthogonal but share the same `workspace_root` tree.

## Desired UX
* User starts command in terminal tab → can switch to chat/file tab without killing the PTY.
* Backgrounded terminal shows running indicator (tab spinner/badge) and survives pane close — optional "move to background" action.
* On reattach, scrollback/history is intact. On exit while backgrounded, desktop shows toast/notification and tab switches to `exit code` state, not lost output.
* Multiple backgrounded terminals allowed.

## Design

### 1) Server: Detach, Not Kill
* Add `detach(id)` to `TerminalPtyManager` — clears `callbacks` but keeps `session`/`proc`/`terminal` alive, retains `pending`/`pausedBuffer`. Counter to `kill(id)` which tears down.
* Extend WS protocol (`@console/types/src/terminal.ts: TerminalClientMessage`): add `{type:"detach"}`. `socket.route.ts:149` `close` should detach if the client sent `detach` or if `?background=1` was negotiated; otherwise kill as today. Simpler: new REST endpoint `POST /api/terminals/:id/detach` + `POST /api/terminals/:id/attach` that re-binds `callbacks` to a new WS without spawning a new PTY.
* Keep `PAUSED_BUFFER_LIMIT_BYTES` (8 MiB) but when detached and no `callbacks`, buffer to `pending` (already capped at 100 entries) — increase cap or spill to temp file for background scrollback. Prefer cap 10k lines ring buffer.

### 2) Server: Reattach + Exit Retention
* Store `exitCode` + `exitedAt` on `PtySession` after `proc.exited` instead of deleting the entry immediately. Keep entry for e.g. 10 min after exit so reattach can read the final code. `killAll` semantics unchanged on shutdown.
* On `attach(id, callbacks)`, replay `pending` + `pausedBuffer` and immediately send `{type:"exit",code}` if already exited.

### 3) Desktop: Workspace Model
* Add `TerminalState { terminal_id, title, cwd, detached: bool, exit_code: Option<i32> }` keyed by `terminal_id` in `ConsoleDesktopApp` (similar to `workspace_pane_states`). Don't store in `StorageDocument` — ephemeral, but survive pane closes.
* `WorkspaceTabConfig::Terminal` already carries `terminal_id`; closing the last tab hosting a detached terminal should *not* auto `kill` — move tab to hidden `background_terminals` list instead. Show in command palette / terminal picker.
* Tab rendering: when `detached && !exited`, show spinner/badge via `WorkspacePane` tab strip (like `running_sessions` badge). When `exited` while detached, set `exit_code` and surface via `notify_agent_event` toast: `"Terminal 'cargo build' exited with code 1"`.

### 4) Desktop: WS Lifecycle
* On pane hide/tab switch, send `{"type":"detach"}` before closing WS, or just close and let server detach if `terminal_id` is in `background_terminals`. On re-select, open new WS `GET /api/terminals/:id/attach?...` (or re-`spawn` with `?attachId=`), call `terminalPtyManager.attach`.
* Handle `resize` after reattach to sync cols/rows.

### 5) Persistence / Limits
* No SQLite persistence for now — in-memory `sessions` map is enough; PTYs die on server restart (document). Future: rehydrate via `cwd`/`shell` replay not needed.
* Cap concurrent background PTYs (e.g. 8) to bound FDs; `TerminalPtyManager.size` already tracks.

### 6) Security / Approval
* Terminal spawns remain outside agent approval flow (`approval_mode`). Backgrounding doesn't change that. No new auth.

### 7) Out of Scope
* Subagent backgrounding — blocked on `AgentMessage`/`ToolCall` correlation (`apps/server/agent/src/session/session-subagents.ts`, `upsertToolResult`). Would require run-level detach. Track separately.

## Alternatives Considered
* Reuse agent run backgrounding (`RunService`) for terminals — rejected: over-couples PTY to run state machine, leaks `running_sessions` semantics.

## Implementation Steps
1. `packages/types/src/terminal.ts` — add `detach`/`attach` messages, `TerminalExitEvent` retention.
2. `apps/server/api/src/terminal/pty.manager.ts` — add `detach()`, retained exit, ring buffer for detached output.
3. `apps/server/api/src/terminal/socket.route.ts` + new `routes/terminals.ts` (`POST /:id/detach`, `/:id/attach`) — wire WS reattach.
4. `apps/desktop/crates/console-core/src/types/workspace.rs` — no change (id already stable).
5. `apps/desktop/src/state/workspace_panes.rs` + `app.rs` — `background_terminals` map, tab close → detach, tab select → attach.
6. UI: tab badge + notification, palette picker for backgrounded terminals.

## Verification
* `bun --watch` in terminal tab → switch to chat → command continues, output visible on return.
* Close last tab with running PTY → still listed in palette, reattach restores scrollback.
* Backgrounded `sleep 5 && exit 1` → toast on exit, tab shows exit code.
* `cargo check` + `bun tests/terminal.test.ts` + manual WS detach/reattach.

## Risks
* FD leak if detach never reaped — add idle timeout (e.g. 30 min) + `kill` on palette dismiss.
* Buffered output growth — enforce ring buffer limit.
