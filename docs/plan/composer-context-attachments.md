# Composer Context Attachments — Linked Directories first, Terminal later

Adds two Conductor-style composer attachments, sequenced: `add directory` (link an external folder read-only) ships first; `terminal` (attach the current terminal) is deferred until the `shell.read` tool exists. Both surface as pinned items at the top of the desktop `@` autocomplete, above file results.

**An attachment is a pointer, never payload.** Attaching a terminal does *not* paste terminal output into the conversation, the setup message, or the system prompt. It records a *handle* (`term_abc123`), and the agent pulls content on demand with `shell.read`. This is exactly how file attachments already behave today: the desktop resolves `@path` client-side, the server appends a bare path to the message (`contextFileRefs`, `internal/agent/loop/message.go:115`), and the agent calls `read_file`. Terminals join that same model instead of introducing a second, prompt-polluting one.

Target: `session.header.extraDirs`, `internal/agent/loop/message.go`, `internal/agent/tools/shell_read_tool.go`, `internal/services/pty_manager.go`, desktop `src/state/attachments.rs`.

---

## 0. Non-Goals

- **No terminal content in the system prompt, setup message, or message body.** Not a snapshot, not a summary, not an 8KB cap. The handle is the entire attachment. (This replaces the earlier "point-in-time snapshot" design.)
- No writable external dirs in v1. Linked dirs are read-only. Writes outside session `cwd` are denied.
- No live terminal streaming into the model. `shell.read` is pull-only; the agent asks, it is not pushed to.
- No new chat-transcript storage for shell output. Terminal scrollback is a live runtime resource, not a message.
- No cross-tool refactor of existing tools, except where explicitly sequenced below.

---

## 1. Design Principles

1. **Single cwd stays canonical.** `session.header.cwd` remains the only writable root. `extraDirs` is an additive read allowlist, same shape as existing `cwd` plumbing (`internal/services/session_service.go`, `internal/run/service.go`).
2. **References over payload.** Every attachment kind resolves to "call this tool to read it":
   | Kind | Agent reads via |
   | :--- | :--- |
   | file mention | `read_file` / `glob` / `grep` |
   | linked directory | `read_file` / `glob` / `grep` (path allowlisted by `extraDirs`) |
   | terminal | `shell.read` (`source: terminal`) |
   | script run | `shell.read` (`source: scriptRun`) |
   | background job | `shell.read` (`source: bashJob`) |
3. **Structure yes, content no.** A linked dir contributes its *shallow tree* (depth 2) to setup — that is structure, it is small, and it is cache-stable. It never contributes file contents. A terminal contributes *no* structure at all, only a handle plus a label the user sees on the chip.
4. **Keep `StableSystem` byte-stable.** `harness-token-efficiency.md` (Phase 2) splits the prompt so `StableSystem` holds no per-session values and the provider's prompt cache survives across turns. Per-turn attachment context belongs in `Setup` (the leading user message) or in tool results — never in the system prompt. This is the reason principle 2 is a correctness rule and not a preference.
5. **Directories first, terminal later — but `shell.read` lands first.** Phase 0 is `shell.read`. The terminal *attachment* (Phase 4) is a chip over a handle that Phase 0 already provides; without Phase 0 the chip points at nothing.
6. **Attachments ride a widened message shape.** `loop.ImageAttachment` is image-only today — `{Data, MimeType}`, no `kind` discriminator — and every provider adapter converts it to a native image block (`providers/claude/convert.go:174`, `providers/codex/stream.go:65`, `providers/antigravity/convert.go:111`, `providers/opencode/messages.go:224`). New kinds therefore need a real union, not a new field on `ImageAttachment` (see §2).
7. **Fail loud.** Unknown dir, path escape, dead terminal handle → `isError: true` with a specific message, never a silent empty result.

---

## 2. Data Model

The old draft proposed adding `Kind string` to `ImageAttachment`. That is wrong: every adapter branches on `MimeType` to emit an image content block, so a non-image payload would either be dropped or sent as a broken image. Widen the union instead, keeping the image variant byte-identical so existing provider paths and token accounting (`agent/compaction/tokens.go:36`) keep working.

```go
// internal/agent/loop/message.go (extend)

type AttachmentKind string

const (
    AttachmentImage   AttachmentKind = "image"   // existing base64 behavior, unchanged
    AttachmentDirLink AttachmentKind = "dirLink" // read-only root; read via read_file/glob/grep
    AttachmentShell   AttachmentKind = "shell"   // handle only; read via shell.read
)

// ShellSource names the runtime a shell handle points at. One enum for
// terminal panes, console.toml script runs, and background bash jobs, so
// the agent learns a single reader instead of three.
type ShellSource string

const (
    ShellTerminal  ShellSource = "terminal"
    ShellScriptRun ShellSource = "scriptRun"
    ShellBashJob   ShellSource = "bashJob"
)

type Attachment struct {
    Kind AttachmentKind

    // AttachmentImage
    Data     string // base64
    MimeType string

    // AttachmentDirLink
    Path string // absolute, must be inside a session extraDir entry

    // AttachmentShell — pointer only, no Content field by design
    Source ShellSource
    Ref    string // terminal id / run_id / job_id; empty means "focused terminal"
    Label  string // chip label only, e.g. "vite dev", "build output"
}

type UserMessage struct {
    // ...unchanged fields...
    Attachments []Attachment `json:"attachments,omitempty"`
}
```

Wire/DTO mirrors to keep in step: `proto/console/v1/message.proto` (`ImageAttachment` → `Attachment`), `internal/run/service.go` (`run.Attachment`), `internal/types/queue.go` (`types.QueuedAttachment`), `internal/routes/run.go` (`runPromptBody`), and the desktop DTO `RunPromptDto.attachments` in `apps/desktop/crates/console-core/src/types/events.rs`.

### Rendering (setup message, not system prompt)

`DirLink` and `Shell` render to exactly one short line each in a single `<attachments>` block appended to `Setup`, beside the existing `contextFileRefs` expansion. Typical result — the full shape of what the model sees before it calls anything:

```
<attachments>
- directory /Users/x/api-server (read-only root)
- terminal "vite dev" — read with shell.read source=terminal ref=term_abc123
</attachments>
```

Two lines. No content, no snapshot, no tree of the terminal, no prompt-injection surface from untrusted scrollback.

---

## 3. Phase 0 — Server: `shell.read` (new, and first)

One tool, source-agnostic, that pulls scrollback from any shell-like runtime. This is the piece the old plan was missing: the terminal attachment is a handle, and `shell.read` is the only thing that resolves it.

```go
// internal/agent/tools/shell_read_tool.go
type shellReadInput struct {
    Source string // terminal | scriptRun | bashJob
    Ref    string // terminal id / run_id / job_id; omit for the focused terminal
    Cursor int    // byte offset from a previous call; 0 = from start of retained buffer
    Tail   int    // alternative to Cursor: last N lines (mutually exclusive)
    Limit  int    // max chars per stream (default 8000, max 50000)
    WaitMs int    // block up to N ms for new output before returning (max 120000)
}
```

Result shape mirrors `bashJob`'s existing contract (`renderJobOutputBody`, `bash_job_tool.go:44`) so there is one output format to learn: `cursor → nextCursor`, `truncated`, `stdout`, `stderr`, and a trailing `Poll again with cursor=N`.

Per-source backing store — note that only two of the three exist today:

| Source | Backing store | Status |
| :--- | :--- | :--- |
| `bashJob` | `BashJobManager.Output(jobID, session, cursor, limit)` | **Exists.** Already cursor-paginated; `shell.read` delegates verbatim. |
| `scriptRun` | `ProjectScriptsService` | **Exists, but un-paginated.** `GetRun` returns full `Stdout`/`Stderr`. |
| `terminal` | `PtySession` | **Does not exist. Must be built.** |

Two gaps to close:

- **Add cursor pagination to script runs.** `appendOutput`/`capOutput` (`project_scripts_service.go:61-85`) retain only the last `scriptMaxOutput` = 256KB per stream. `project_scripts` `status` dumps all of it every call, so polling a chatty `dev` server re-sends up to 512KB per poll. `shell.read` must add `Cursor`/`NextCursor` over the retained ring before any UI starts polling it.
- **Add a scrollback ring to `PtySession`.** `PtySession` (`pty_manager.go:87-115`) has only *transient* coalescing buffers — `outputQueue`, `pending`, `pausedBuffer` — which are flushed and dropped. The scrollback exists solely inside the client's xterm.js instance. Server-side retention must be added as a bounded ring (propose 256KB, matching `scriptMaxOutput`) plus a monotonic byte cursor so `shell.read` can page it. This is the single largest piece of Phase 0.

Guardrails: reads scoped to the owning session (a session must not read another session's terminal), and reads are `TierRead` — `shell.read` never writes to a terminal, never runs a command. Writing to a terminal is a separate, higher-tier concern and is deliberately out of scope here.

- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestShellRead -v` — per-source dispatch, cursor round-trip, limit cap, wait timeout, cross-session read denied, unknown ref → clean tool error.

---

## 4. Phased Implementation

### Phase 1 — Server: linked dirs (read-only)
- Add `extraDirs` to session header/schema (`internal/services/session/` ops, `internal/services/session_service.go` facade): `AddLinkedDir(sessionId, path)`, `RemoveLinkedDir`, validate exists + absolute + under allowlist, cap 5.
- Add `isPathReadable()` guard; wire into the tool executor (`internal/agent/loop/executor.go`, `internal/agent/tools/registry.go`) — allow reads from `cwd + extraDirs`, deny writes outside `cwd` — and `internal/routes/assist.go` search/files (resolve `@` against all roots).
- Extend `internal/agent/systemprompt/workspacetree.go` / `builder.go` with a `<linked_dirs>` block listing roots + shallow tree (depth 2). **Setup only, never `StableSystem`** (principle 4).
- **Verification**: `cd apps/server-go && go test ./tests/sessions/ -run TestLinkedDirs -v` — add/remove validation, read allowed, write denied, escape (`..`) rejected, block present in Setup and absent from `StableSystem`.

### Phase 2 — Server: attachment union + terminal handle resolution
- Widen `loop.UserMessage.Attachments` to `[]Attachment` (oneof), keeping the image variant byte-identical; update the four provider adapters, `agent/loop/proto.go`, `agent/compaction/{tokens,llm}.go`, and the queue DTOs listed in §2.
- Render the `<attachments>` block into `Setup` from `DirLink`/`Shell` kinds (§2). No content path exists for a shell attachment, so none can leak.
- Resolve a terminal `Ref` against `PtyManager.Get(id)` at send time; unknown/dead ref → tool error naming the handle, never a silent drop.
- **Verification**: `cd apps/server-go && go test ./tests/sessions/ -run TestAttachments -v` — image attachments still render as image blocks on every adapter; dir link renders as one line; shell renders handle only; the word "vite" from a fake scrollback must not appear anywhere in prompt or setup.

### Phase 3 — Desktop: `@` pinned actions + DirLink chips
- Extend the item model: `AutocompleteItem` currently has only `Command(SlashCommandInfo)` and `File(FileSearchResult)` (`crates/console-ui/src/common/autocomplete.rs:70`) — no pinned/static variant exists. Add `Action(PinnedAction)` and have `filter_items` (same file, `:181`) emit `add directory` then `terminal` ahead of file results for every query, including the empty one.
- Extend `state/attachments.rs` with a `DirLink(path)` variant alongside images; per-pane staging already exists (`state/types.rs:136`).
- `add directory` → native folder picker → `addLinkedDir`; render chip + removable; `submit_prompt` sends it with the existing run call (`state/run.rs:319`).
- **Verification**: manual — `@` with empty query shows `add directory` pinned at top; link an external repo, ask the agent about its files (reads OK, write attempt refused).

### Phase 4 — Desktop: terminal chip (a pointer, not a paste)
- Chip is a *label + handle*: `terminal "vite dev" · term_abc123`. It carries no content across the wire and nothing about it changes between attach and send.
- Selecting the pinned `terminal` row attaches the currently focused pane; if no pane is focused, the row is disabled with a reason rather than guessing.
- Content arrives only when the agent calls `shell.read`. Attach-after-a-failed-build should therefore cost the model *zero* tokens until it decides the log matters.
- **Verification**: manual — attach terminal, ask a question answerable without the log (model must not have read it), then ask "what failed?" (model calls `shell.read`, gets the tail).

### Phase 5 — Mobile parity (deferrable) + cleanup
- Mirror DirLink/Shell chips in the native Android composer (`apps/android`), or explicitly defer.
- Docs: a short section describing the `<attachments>` block and the `shell.read` tool (`docs/harness-features.md` referenced by the earlier draft does not exist; add it or fold into an existing doc).
- **Verification**: existing relevant test file passes; no full-suite run.

---

## 5. Open Questions

- **Terminal retention size.** 256KB per pane mirrors `scriptMaxOutput`, but a full-screen `npm install` tree exceeds that. Propose 256KB with a byte cursor so dropped bytes are visible as a gap rather than silently corrupting offsets.
- **Which ref does a bare `terminal` attach resolve to?** Recommend: focused pane at send time, snapshotted to a concrete id when the chip is created — otherwise a chip silently retargets when the user changes focus.
- **`shell.read` `Tail` vs `Cursor` mutual exclusion.** Recommend allowing both (`tail` sets the initial cursor) rather than erroring, since "last 200 lines" is the overwhelmingly common ask.
- Should `extraDirs` persist per-session or per-project? Recommend per-session v1 (simpler, matches the `cwd` lock model).
- Cap sizes: 5 dirs — tunable later. The terminal 8KB cap is **gone**; it was an artifact of the snapshot design and is replaced by `shell.read`'s `limit`.

---

## 6. Verification Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| `shell.read` per source | `go test ./tests/tools/ -run TestShellRead -v` | terminal / scriptRun / bashJob all resolve; unknown ref → clean error |
| `shell.read` pagination | same | cursor round-trips; `limit` honored; poll re-sends nothing already seen |
| Terminal scrollback | same | pane output retained server-side and pageable (new ring) |
| Cross-session read | same | session A cannot read session B's terminal |
| Attachment union | `go test ./tests/sessions/ -run TestAttachments -v` | image still an image block on all 4 adapters |
| **No content leak** | same | attached terminal text appears in **no** prompt, setup, or message body |
| Link dir validation | `go test ./tests/sessions/ -run TestLinkedDirs -v` | rejects relative/missing/duplicate/>5 |
| Read allowed, write denied | same package | grep/read OK in linked dir; edit/write outside cwd errors |
| Prompt block location | same package | `<linked_dirs>` in `Setup`, absent from `StableSystem` |
| End-to-end desktop | manual | link + terminal chips reach agent with no paste and no injected content |