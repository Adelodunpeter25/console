# Composer Large Paste — Collapse Into a File-Mention Chip

A 5,000-line log pasted into the composer turns the whole input into a wall of text (and would carry every line into the prompt and the bubble). When a paste crosses a threshold, the composer collapses it into one inline file-mention chip instead. **Collapse is not truncate: nothing is cut.** The full text is written to a private file on the *server* and injected as an `@path` mention, exactly like `@file` selections today — the composer shows a chip, the bubble shows the same chip, and the agent reads or greps the file on demand.

The core principle matches `composer-context-attachments.md`: **an attachment is a pointer, never payload.** A pasted blob is a pointer to a server-side file, not a second prompt-polluting content channel.

Target: `internal/routes/pastes.go` (new), `internal/utils/paths.go`, `crates/console-ui/src/common/input/mod.rs`, `crates/console-ui/src/common/file_mention_chip.rs` (unchanged, reused), `src/state/workspace_tabs/chat.rs`, `crates/console-core/src/services/fs.rs`, Android `Composer.kt`, `MentionParser.kt`, `OkHttpConsoleApi.kt`.

No async decision: the paste decision is made from clipboard text size before anything hits the buffer, so no post-hoc buffer surgery.

---

## 0. Non-Goals

- **No literal truncation of pasted content.** The agent must always be able to see every pasted line. "Truncate" in the user request means "collapse the display", not "drop bytes".
- **No pasted content in the message body.** The stored user message keeps only the chip label; the content lives server-side. This is the same correctness rule as `StableSystem`: history must stay small.
- **No new proto message.** `context_files` on `RunPrompt` / `AgentUserMessage` already carries the path; `contextFileRefs` (`internal/agent/loop/message.go:109`) already appends it for the model. The paste route is plain hand-written JSON like `/answer`.
- **No clickable inline chips.** The inline chip is painted from text-layout geometry (`InlineFileMentionText`), not an interactive element. A preview modal is a later, separate piece.
- **No per-turn storage churn.** If the user drops the chip or never sends, the file waits for the sweeper; garbage collection is time-based, not referential.

---

## 1. Design Principles

1. **Server-side storage, always.** The desktop client can point at a remote daemon, so a file written on the Mac would be invisible to the agent. The paste file is created wherever the server runs, under `utils.ConsoleStorageDir()`, so the path in `context_files` is always readable by the model's tools.
2. **Reuse the mention pipeline end to end.** Top-down: composer chip → `ComposerMention` record → `Submit(value, context_files)` → `RunPrompt { context_files }` → `contextFileRefs` appended → `read_file` / `grep`. No new wire field anywhere.
3. **The chip label says what happened, the path says where.** Chip label is `pasted 5,000 lines` (human); the underlying mention path is `~/.console/pastes/<session>/pasted-20261029-142233-x7k2.log` (unique, sortable). Existing bubble mention matching finds the label in the content and renders the pill automatically.
4. **One threshold, checked at paste time.** Before the text enters the buffer, if `lines > 200 || bytes > 64 KiB`, intercept and collapse instead of inserting. Small pastes keep today's exact behavior.
5. **Fail soft.** If the upload fails (offline, oversized, server old), paste inline with today's behavior and surface a small error notice. The collapse is a UX improvement, never a hard dependency.
6. **Composers are per-pane; the upload target is per-session.** Follow the established capture pattern: `ComposerAttachmentPaste` listeners capture `pane_id` at subscribe time so async results land in the pane the user pasted into (`src/state/workspace_tabs/chat.rs:190-199`).

---

## 2. Server — `POST /api/sessions/:id/pastes` (Phase 0)

New `internal/routes/pastes.go`, one endpoint, plain JSON (no proto, no codegen):

- In: `{ "text": "..." }`. Out: `{ "name": "pasted-20261029-142233-x7k2.log", "path": "/Users/…/.console/pastes/<sessionId>/pasted-….log", "lines": 5214, "bytes": 384112 }`
- Storage root: `utils.PastesDir() = ConsoleStorageDir()/pastes/<sessionId>/` — dir `0700`, file `0600`, mirroring the credential-file precedent. Dev installs land in `~/.console-dev/pastes/…` automatically via `ConsoleStorageDir()`.
- Name: `pasted-YYYYMMDD-HHMMSS-<4 rand chars>` + `.log` when the content looks log-shaped (≥2 timestamp-prefixed lines) else `.txt`. Timestamps keep filenames unique without a counter.
- Guards: session id validated (no separators/case beyond existing ids), text non-empty, **2 MB cap** (413 with a clear message), body limit already bounded by Fiber. Rejecting oversized pastes loudly beats silently dropping lines.
- Cleanup, two hooks, both cheap:
  - On session delete, `os.RemoveAll(PastesDir()/sessionID)`.
  - A sweeper started with the daemon (hourly) deleting files older than **7 days**. No per-file refcounting.
- **Verification**: `cd apps/server-go && go test ./tests/api/ -run TestPastes -v` — write + respond, dev-dir override, caps (empty text, oversized), name uniqueness/collision retry, delete hook, sweeper respects 7-day/0600/Mac paths. `go build ./...`.

---

## 3. Phase 1 — Desktop: paste interception + staged chip

### Composer input (`crates/console-ui/src/common/input/mod.rs`)

- In `ComposerInput::paste` (field mode `Composer` only — search/code fields keep raw behavior): read the clipboard text, normalize `\r\n → \n` as today, then apply the threshold. Over the threshold → emit a new event and return *without* inserting:

```rust
/// A clipboard text paste too large to inline. The composer inserts nothing;
/// the owning app uploads the text and inserts a mention chip over `range`
/// (usually the whole selection/caret) when the server responds.
#[derive(Clone)]
pub struct ComposerLargePaste {
    pub text: String,
    pub range: Range<usize>,
}
```

- Mirror the `ComposerAttachmentPaste` `impl EventEmitter` (input/mod.rs:1526) and the `Paste` action wiring.

### App staging (`src/state/workspace_tabs/chat.rs`, `src/state/attachments.rs`, `console-core/src/services/fs.rs`)

- Subscribe next to the existing paste listener; capture `pane_id` at subscribe time (same pattern as `ComposerAttachmentPaste`, lines 190–199).
- Async on the background executor: call the new client method `create_paste(session_id, &text)` (plain JSON POST; the session id resolves via `active_session_for_pane`, so question/edit panes previously holding a paste must not fire — only the composer mode emits it).
- On success: `input.insert_paste_mention(range, path, lines)` → new thin wrapper over the private `insert_mention_chip` body that takes an explicit label (`pasted {lines:,} lines`) instead of deriving it from the filename. Insertion stays **one undo step** (seal → insert → seal, matching the paste path's history rules).
- On failure: `replace_text_in_range(range, &text)` inline (today's behavior) and raise a compact error notice; do not block the composer.
- The mention bubble path needs no change: the label is in the text, so `message_text_with_mentions` renders the pill on send and recall, and `history.rs` already rebuilds chips from recorded `context_files`.

- **Verification**: `cargo check --workspace --all-targets`; manual — 5,000-line log collapses to a chip immediately, small pastes behave exactly as before, remote-server pairing also produces a server-side path (chip label identical), failure path falls back inline.

---

## 4. Phase 2 — Desktop polish (small, optional)

- **Undo affordance**: one-shot toast after the chip lands — "Pasted 5,000 lines as a file · Undo" — Undo restores the inline text and forgets the chip so the file just waits for the sweeper. Avoid refcount cleanup.
- **Threshold feedback**: a manual "Collapse paste" entry next to the `@` autocomplete button could accept the current selection as an explicit collapse of any size. Skip in v1 unless trivial.
- **Busy chip**: while the upload is in flight, insert a disabled-looking spacer label ("saving paste…"), then swap to the chip; avoids the async race where the user keeps typing during upload. If too fiddly, accept the captured-range insert (latency is local-network small).

---

## 5. Phase 3 — Android parity

Android parses mentions from raw text (`MentionParser.kt`), so an absolute path mention needs no new chip plumbing — only large-paste interception plus an API call.

- **API**: `ConsoleApi.createPaste(sessionId, text)` in `OkHttpConsoleApi.kt`, same JSON body/response as desktop's; no Moshi proto involvement.
- **Interception point**: `Composer.kt`'s `onFieldValueChange` — derive a delta (`newText.length - oldText.length`) and apply `lines > 200 || bytes > 64 KiB` on the delta. Compose `BasicTextField` exposes no paste-specific hook, so the length jump is the paste signal (a typed edit never clears it).
- **Insert on success**: splice `@<path>` into `fieldValue` at the delta's offset (space-separated so the parser matches; our generated paths are space-free). `mentionPaths` already picks it up into `contextFiles` with no change to `ChatRepository.kt`.
- **Label parity**: android chips render `path.substringAfterLast('/')`. Add a special case in the chip label mapping for `…/pastes/…` paths → `pasted text`, so the mobile label matches the desktop's intent without a new proto field.
- The mobile user bubble reuses the same parser, so the chip appears there without extra work.
- **Verification**: `cd apps/android && ./gradlew :app:assembleDebug`; manual — same collapse flow with the phone pointed at the daemon.

---

## 6. Open Questions

- **Threshold numbers.** `200 lines / 64 KiB` covers the 5,000-line case comfortably without jumping on ordinary code snippets. Constant, not a setting, in v1.
- **Extension inference (`log` vs `txt`).** Nice icon; pure heuristic; fine to land without it (all `.txt`).
- **Chip label vs filename.** Label `pasted N lines` reads better, but the bubble's mention matcher needs the label as a plain occurrence in content — inserting `{prefix}{label}` does that. Confirm the matcher's whitespace guards tolerate the label's spaces across the `FILE_MENTION_SPACER` insertion path.
- **Model hint.** `contextFileRefs` appends a bare path. Should paste paths get a trailing `(pasted text, N lines)` annotation so the model knows to read it rather than treat it as a normal `@file`? Recommend yes once filenames alone aren't obvious; it's a one-line change in `contextFileRefs`, but needs the server to accept line counts.
- **Preview.** Defer. Inline chips are painted geometry, not elements; a preview would need its own modal fed by `GET /api/fs/file?path=…`.

---

## 7. Verification Matrix

| Test case | Method | Expected outcome |
| :--- | :--- | :--- |
| Paste route writes + responds | `go test ./tests/api/ -run TestPastes -v` | file exists 0600 under dev-dir override, path/lines/bytes match |
| Caps | same | empty text 400, oversized text 413, names unique |
| Session delete cleans up | same | session dir gone after delete |
| Sweeper | same | files older than 7 days removed, fresh files untouched |
| Desktop small paste | manual | byte-identical to today |
| Desktop large paste | manual | chip `pasted 5,000 lines`; full text exists on server; chip in sent bubble; recall rebuilds chip |
| Remote-server desktop | manual | path lives on the daemon host, agent reads it |
| Desktop failure path | manual | inline paste + notice, no stuck composer |
| Android large paste | `./gradlew :app:assembleDebug` + manual | chip in composer + bubble, mention parser resolves path, same server storage dir |
| Agent behavior end-to-end | manual | agent greps/reads the paste file on follow-up, no 5,000 lines in prompt/prompt history |
