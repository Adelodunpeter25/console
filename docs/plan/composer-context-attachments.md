# Composer Context Attachments — Linked Directories first, Terminal later

Adds two Conductor-style composer attachments, sequenced: `add directory` (link an external folder read-only) ships first; `terminal` (attach current terminal output) is deferred to a later phase. Both surface as pinned items at the top of the desktop `@` autocomplete, above file results.

Target: `session.header.extraDirs`, `internal/agent/systemprompt/builder.go`, `tools` cwd guard, desktop `state/attachments.rs`.

---

## 0. Non-Goals

- No writable external dirs in v1. Linked dirs are read-only. Writes outside session `cwd` are denied.
- No live terminal streaming. Terminal attach is a point-in-time snapshot (last N lines), not a subscription.
- No persistence of terminal snapshots beyond the message that carries them.
- No cross-tool refactor. Existing `cwd` tools untouched except for a shared read-guard.

---

## 1. Design Principles

1. **Single cwd stays canonical.** `session.header.cwd` remains the only writable root. `extraDirs` is an additive read allowlist, same shape as existing `cwd` plumbing (`internal/services/session_service.go`, `internal/run/service.go`).
2. **Directories first, terminal later.** All directory work (Phases 1–3) ships before any terminal work (Phase 4+). Terminal stays specced but unscheduled.
3. **Pinned `@` actions.** Desktop `@` autocomplete (`state/autocomplete.rs` + `console_ui::filter_items`) always pins `add directory` first and `terminal` second at the top, regardless of query — file results rank below. Selecting one runs its action instead of inserting a mention.
3. **Attachments ride existing message shape.** Extend current `loop.ImageAttachment` flow (`internal/agent/loop/message.go`) with two new kinds (`DirLink`, `TerminalSnapshot`) rather than a new API. Desktop `attachments.rs` + `execution.rs` already stage per-pane; server already expands refs into prompt text.
4. **Fail loud.** Unknown dir, path escape, oversized terminal dump → `isError: true` with specific message.

---

## 2. Data Model

```go
// internal/types/session.go (extend)
type LinkedDir struct { Path string; Readonly bool }
type TerminalAttachment struct { Kind string; Content string; Truncated bool }
```

- Session header: `extraDirs: LinkedDir[]` (max 5, absolute paths only).
- Prompt injection blocks in `internal/agent/systemprompt/builder.go`:
  - `<linked_dirs>` — list of linked roots + shallow tree (depth 2).
  - `<terminal_output>` — fenced block, capped at ~8KB, `truncated: true` flag when cut.

---

## 3. Phased Implementation

### Phase 1 — Server: linked dirs (read-only)
- Add `extraDirs` to session header/schema (`internal/services/session/` ops, `internal/services/session_service.go` facade): `AddLinkedDir(sessionId, path)`, `RemoveLinkedDir`, validate exists + absolute + under allowlist, cap 5.
- Add `isPathReadable()` guard; wire into the tool executor (`internal/agent/loop/executor.go`, `internal/agent/tools/registry.go`) (allow reads from `cwd + extraDirs`, deny writes outside `cwd`) and `internal/routes/assist.go` search/files (resolve `@` against all roots).
- Extend `internal/agent/systemprompt/workspacetree.go` / `builder.go` with `<linked_dirs>` block.
- **Verification**: `cd apps/server-go && go test ./tests/sessions/ -run TestLinkedDirs -v` — add/remove validation, read allowed, write denied, escape (`..`) rejected, prompt contains block.

### Phase 2 — Server: terminal snapshot endpoint
- Add `GET /api/sessions/:id/terminal-output?lines=200` reading PTY manager scrollback for the session (or active terminal), capped, plain text.
- Include as `<terminal_output>` block when the message carries a `TerminalAttachment`.
- **Verification**: `cd apps/server-go && go test ./tests/sessions/ -run TestTerminalAttach -v` — returns last N lines, caps at 8KB with `truncated:true`, empty-terminal returns clean error.

### Phase 3 — Desktop: `@` pinned actions + DirLink chips (directories first)
- Pin `add directory` (first) and `terminal` (second, disabled/“soon” or hidden until Phase 4) at the top of `@` autocomplete in `composer_autocomplete_for_pane` / `filter_items`; empty query still shows them.
- Extend `state/attachments.rs` with `DirLink(path)` alongside images; per-pane staging already exists.
- `add directory` → native folder picker → `addLinkedDir` call; render chip + removable; `submit_prompt` sends with existing run call.
- **Verification**: manual — `@` with empty query shows `add directory` pinned at top; link external repo, ask agent about its files (reads OK, write attempt refused).

### Phase 4 — Terminal snapshot (deferred, after directories)
- Server `GET /api/sessions/:id/terminal-output?lines=200` + `<terminal_output>` block; desktop `TerminalSnapshot(content)` chip wired to the pinned `terminal` `@` row.
- **Verification**: same `go test ./tests/sessions/ -run TestTerminalAttach -v` + manual attach-after-failed-build.

### Phase 5 — Mobile parity (deferrable) + cleanup
- Mirror DirLink/Terminal chips in the native Android composer (`apps/android`), or explicitly defer.
- Docs: short section in `docs/harness-features.md`.
- **Verification**: existing relevant test file passes; no full-suite run.

---

## 4. Open Questions

- Terminal source: session PTY vs. currently focused terminal pane? Recommend focused pane (matches Conductor UX).
- Should `extraDirs` persist per-session or per-project? Recommend per-session v1 (simpler, matches `cwd` lock model).
- Cap sizes: 5 dirs / 8KB terminal — tunable later.

---

## 5. Verification Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| Link dir validation | `go test ./tests/sessions/ -run TestLinkedDirs -v` | rejects relative/missing/duplicate/>5 |
| Read allowed, write denied | same package | grep/read OK in linked dir; edit/write outside cwd errors |
| Prompt block | same package | `<linked_dirs>` lists roots + shallow tree |
| Terminal snapshot | `go test ./tests/sessions/ -run TestTerminalAttach -v` | last N lines, 8KB cap + truncated flag |
| End-to-end desktop | manual | link + terminal chips reach agent, no paste needed |
