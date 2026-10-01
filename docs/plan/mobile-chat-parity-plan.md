# Mobile Chat Parity Plan

Goal: bring the Android chat (`apps/android/.../feature/chat/`) to full
feature parity with the desktop chat (`apps/desktop/crates/console-ui/src/chat/`).
File-mention pills are done (composer styling, bubble pills, `contextFiles`
send wiring) and are explicitly out of scope below.

Target: Android Kotlin/Compose only. The Go server already exposes everything
listed here — no backend work required except where noted.

---

## 0. Non-Goals

- No desktop changes. Desktop is the reference implementation.
- No `packages/api` involvement — Android re-implements endpoints natively
  (`data/api` + `data/repo`), mirroring the desktop pattern.
- Thinking-level stepper, drag-drop files, floating composer: desktop extras,
  deferrable, not gaps in shared behavior.

---

## 1. Context Usage Display (backend ready)

Desktop shows per-session context occupancy in the footer ring and the top of
the usage popover (`ContextSnapshot`, `usage_meter.rs`, `usage_panel.rs`).
Mobile shows nothing in chat — only per-model context sizes in the model
picker rows and quota cards under Settings → Usage.

- Fetch `GET /api/sessions/:id/context` on session open into per-session
  state (same lifecycle as the existing quota fetch in
  `data/repo/UsageRepository.kt` / `UsageStateHolder`).
- Handle the `contextUpdate` stream frame in `ChatRepository.handleEvent`
  (add alongside `askQuestion`/`permissionRequest`) so the number moves live
  after every turn without polling.
- Render a compact ring or bar near the composer (placement open — footer
  strip or composer header), fed by the same snapshot shape desktop uses
  (`usedTokens`, `contextWindow`, `percentUsed`, `thresholdRatio`).
- **Verification**: open a session, confirm the value matches the desktop
  popover for the same session; send a prompt, confirm it moves on the
  `contextUpdate` frame with no manual refresh.

## 2. Clickable Links in Mobile Markdown

Desktop linkifies file paths and `http` URLs inside assistant messages
(`markdown/file_links.rs`); mobile's hand-rolled `MarkdownText.kt` renders
links as cyan underlined decoration only — not clickable.

- Make link spans open the file viewer / browser intent.
- **Verification**: manual — tap a file path and an `http` link in an
  assistant message.

## 3. Queued-Prompt Card

Desktop shows an explicit queued card (Edit/Delete/Steer) while a run is
active (`common/queued_prompt_card.rs`). Mobile has the queue API calls but no
visible card, so a queued follow-up is invisible until it runs.

- Render the pending queued prompt above the composer while running, with
  edit/delete at minimum.
- **Verification**: manual — queue a follow-up mid-run, confirm the card,
  edit it, delete it.

## 4. Compaction Notice

Desktop renders a compaction notice when context is compacted. Mobile's
`EventModels.kt` already carries the compaction fields but nothing in chat
consumes them.

- Consume the compaction frame in `applyChatEvent` (`core/chat/ChatEvents.kt`)
  and render a notice row in the transcript.
- **Verification**: force a compaction (long session) and confirm the notice
  appears in the mobile transcript.

## 5. Server-Computed Diffs (cleanup)

Mobile computes edit diffs client-side with its own LCS
(`core/util/DiffUtils.kt`); desktop renders server-provided diffs. Same look
today, but the two implementations can drift.

- Requires small backend work: expose the diff the desktop already receives
  (or the file-change payload behind it) on the wire for the mobile client,
  then delete `DiffUtils.kt` usage in `DiffView.kt` / `ToolHelpers.kt`.
- **Verification**: existing tool-call UI unchanged; remove the LCS code path
  and confirm diffs still render for edit/write/batchWrite.

## 6. Pending-Interaction Posture (decision needed)

Mobile's `InteractionPanel` *replaces* the composer while a permission or
question is pending; desktop renders the card *above* the composer. Both work,
but they are different UX. Pick one as canonical before touching either —
recommendation: match desktop (card above composer) so the draft survives.

---

## Suggested Order

1. Context usage display (§1) — backend done, pure Android UI work.
2. Clickable links (§2) — small, high value.
3. Queued-prompt card (§3) and compaction notice (§4) — small parity items.
4. Pending-interaction posture (§6) — needs the decision first.
5. Server diffs (§5) — needs backend work, do last.
