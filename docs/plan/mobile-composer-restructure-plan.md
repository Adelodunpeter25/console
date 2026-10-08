# Mobile Composer Restructure Plan (Project + Branch Strip, Thinking Chip, Context Ring)

Goal: restructure the Android composer into the three-band layout — a **top
strip** (project / branch bubbles), a **two-row composer bubble** (multiline
text + action row), and a **reduced bottom strip** (approval + context ring).
Adds a branch/worktree selector and a fused model + thinking-level chip.

Target: **Android Kotlin/Compose only.** UI scope — the endpoints this needs
already exist on the Go server; see §7 for the two client-side gaps that do
require wiring before the UI can render them.

Supersedes nothing, but overlaps `docs/plan/mobile-chat-parity-plan.md` §1
(context usage display) and its §0 non-goal that deferred the thinking-level
stepper. Both are now in scope.

---

## 0 Non-Goals

- No desktop changes. Desktop is the reference implementation.
- No Go server changes. Every endpoint referenced here already exists.
- No mic / voice input (not currently on Android).
- No app-level back button in the top strip — Android system back plus the
  existing app bar cover navigation.
- **No composer shape morph.** The current pill→rect morph is dropped (see §3.1).

## 1 Current State

- `Composer.kt` renders a single `Row` bubble: attach (37dp) +
  `BasicTextField` (weight 1f, `heightIn(max = 120.dp)`) + send/stop (35dp).
  Shape is `CircleShape` when one line, `RoundedCornerShape(20.dp)` when
  wrapped (`bubbleShape`, line 264), consumed by `.clip()` and `.border()`.
- `ComposerBottomStrip.kt` is a `LazyRow` of three `PickerChip`s — project,
  model, approval — each opening a `ModalBottomSheet` from `PickerSheets.kt`.
  Rows 0 and 1 own the session mutation (`updateSession(UpdateSessionDto(...))`).
- `PickerChip` is 26dp tall, 11sp text, 13dp glyph, 8dp corner radius. Renders
  a provider logo in place of the passed icon when `provider` names a known one.
- `PickerSheets.kt` provides `PickerSheetTitle`, `PickerRow(title, subtitle,
  selected, monoSubtitle, trailing, onClick)`, `PickerPlaceholder`, and
  `ProjectPickerSheet` / `ModelPickerSheet` / `ApprovalModePickerSheet`.
- `ThinkingLevels` (`data/model/ProviderModels.kt:11`) declares the full
  vocabulary (`none|minimal|low|medium|high|xhigh|max`) but has **zero call
  sites** — `grep -rn ThinkingLevels apps/android` hits only the declaration
  and one generated Wire file.
- `supportedThinkingLevels` exists on the generated Wire `Model` type. **No
  Kotlin consumer.**
- `UpdateSessionDto` (`data/model/ApiModels.kt:24`) carries `title`, `cwd`,
  `projectId`, `modelId`, `provider`, `approvalMode` — **no `thinkingLevel`**.
- Branch data on Android is read-only and chat-unwired: `GitRepository` builds
  a `Map<projectId, branch>` and `HomeScreen.kt` renders branch text on session
  rows (line 406). `ConsoleApi.checkoutBranch` + `GitRepository.checkoutBranch`
  are implemented and **unreferenced by any chat UI**.

## 2 UX Proposal

Three bands, top to bottom:

```
┌─────────────────────────────────────────┐
│  [ 🗎 Local ]   [ ⑂ main ]              │  ← NEW top strip
├─────────────────────────────────────────┤
│  ┌───────────────────────────────────┐  │
│  │ Ask for follow-up changes         │  │
│  │                            [ ⚡ ]  │  │
│  │  +                       ( ➤ )    │  │
│  └───────────────────────────────────┘  │
├─────────────────────────────────────────┤
│  [🛡 Full Access]             ( ◔ 42% ) │  ← reduced strip
└─────────────────────────────────────────┘
```

| Control | Was | Becomes |
|---|---|---|
| Project | bottom strip chip | top strip bubble |
| Branch | — (read-only in Home) | top strip bubble (new) |
| Model | bottom strip chip | composer action row, fused with thinking |
| Approval | bottom strip chip | bottom strip (unchanged) |
| Context | — | bottom strip ring (new) |

The bottom strip drops from three chips to two items, which is what removes the
overflow — approval was the highest-stakes control in that row and a fourth
chip would have pushed it off-screen on a narrow phone.

### 2.1 Fused model + thinking chip

One chip reading `⚡ {model} {thinkingLabel}`, always visible while the composer
is on screen. Tap opens `ModelPickerSheet` with a thinking section appended.

- Levels render from the **highlighted** model's `supportedThinkingLevels`, so
  they re-filter live as the user moves through the model list.
- The chip **hides entirely** when the selected model reports no levels. A
  permanently-disabled `⚡ None` is worse than absence.
- Raw enum strings go on the wire; human labels go in the UI. `PickerRow`'s
  `subtitle` slot carries the description ("Fastest", "Balanced",
  "Thorough", "Maximum depth") so `XHIGH`/`MAX` never surface to users.

### 2.2 Top strip — project → branch

Branch is scoped to project, so these are not independent pickers:

- Changing project **resets** the branch bubble to that project's default.
- A project with no git repo **collapses** the branch bubble rather than
  showing a dead control.
- "New worktree" is a **distinct row at the top** of the branch sheet,
  separated by a divider — creating one is an action, not a selection.
- Bubbles truncate to one line with ellipsis; the strip scrolls horizontally
  when it overflows, since long project paths blow out the width fast.

### 2.3 Context ring

A compact ring in the bottom strip, mirroring desktop's footer ring. Fed by the
same `ContextSnapshot` shape desktop uses (`usedTokens`, `contextWindow`,
`percentUsed`, `thresholdRatio`), and updated live from the `contextUpdate`
SSE frame — no polling.

### 2.4 Disabled during a run

Changing thinking level or branch mid-stream silently applies to the *next*
turn, not the one in flight. Both controls are disabled while a run is active,
with a subtitle saying so, rather than letting the user believe they changed
the current turn.

## 3 Desktop Parity Notes

### 3.1 Dropping the morph

`bubbleShape` is computed once (`Composer.kt:264`) and consumed in exactly two
places on the same Row — `.clip()` at 266 and `.border()` at 268. Deleting the
conditional and using `RoundedCornerShape(20.dp)` unconditionally removes the
morph outright; it is not entangled with anything else.

`visualLines` is threaded from line 86 through 155, 204 and 250. Once the shape
no longer reads it, confirm nothing else does and unwind the parameter in the
same pass rather than leaving a dead value flowing through the signature.

`CircleShape` stays imported — lines 274, 374 and 382 still use it for the
attach and send buttons.

## 4 Implementation Steps

- **1 — Unmorph.** Delete the conditional shape; unwind `visualLines` if it
  has no other consumer. No behavior change beyond the shape.
- **2 — Composer restructure.** `Column` bubble: text row (attach + field) over
  action row (fused chip + send). Keep `BasicTextField` and the mention-overlay
  machinery untouched — the chip must not go inside the field, since mention
  icons ride as overlays on reserved slots and would collide. Shrink
  `heightIn(max = 120.dp)` to ~72–80dp so text + action row doesn't produce a
  dominant bubble.
- **3 — Fused chip.** `PickerChip` with composed label; hidden when the model
  reports no levels. Always visible otherwise.
- **4 — Thinking section in `ModelPickerSheet`.** Divider + title + level rows
  from the highlighted model; local `remember` for the highlighted model,
  separate from the committed value.
- **5 — Top strip composable.** Project bubble reuses `ProjectPickerSheet`
  as-is; branch bubble opens a new sheet with the worktree row pinned at top.
- **6 — Wire `thinkingLevel` into `UpdateSessionDto`.** Add the field and
  thread it from the chip selection.
- **7 — Context ring.** Bottom-strip ring component + the `contextUpdate`
  handler (§7.2).

## 5 Verification

- Composer renders three bands; bubble is a constant 20dp rect at one line and
  when wrapped.
- Long project name truncates with ellipsis; strip scrolls rather than
  shrinking bubbles.
- Switching project resets the branch bubble; no-git project shows no branch
  bubble.
- Thinking section lists only the highlighted model's levels; chip is absent
  for a model reporting none.
- Level labels read as words ("Balanced"), never `XHIGH`.
- Both new controls are disabled during an active run.
- Context ring moves after each turn without polling, and matches desktop's
  footer ring for the same session.

## 6 Out of Scope

- Mic / voice input.
- App-level back affordance in the top strip.
- Restoring the composer shape morph.
- Desktop equivalents of any of the above.

## 7 Client-Side Gaps (must land before the UI can render)

These are Kotlin wiring, not server work — the Go endpoints already exist.

### 7.1 Thinking level has no write path

`GET /api/sessions/:id/context`-style read paths are fine, but the thinking
level must round-trip. `UpdateSessionDto` has no `thinkingLevel` field, and no
Kotlin code consumes `supportedThinkingLevels`. Until step 6 lands, the chip
renders but selection cannot persist.

Confirm the server persists an incoming `thinkingLevel` on session update —
`TurnRequest.ThinkingLevel` exists in the agent loop
(`agent/loop/loop.go:50`), so the plumbing exists at the loop layer; this is
only about whether the session-update path surfaces it. **Verify against the
server source before building the picker.**

### 7.2 Context snapshot is unreferenced by the Kotlin client

- `GET /api/sessions/:id/context` → `{success, data}` where `data` is
  proto-JSON of `console.v1.ContextSnapshot` (`event.proto:33`: `usedTokens`,
  `contextWindow`, `percentUsed`, `thresholdRatio`, `modelId`, `provider`,
  `source`). Note the field names are **snake_case in proto, camelCase in
  proto-JSON** — decode as camelCase.
- SSE `contextUpdate` frame carries the same payload under `context`
  (`routes/run.go:674`), but `AgentSessionEvent` has **no `context` field** and
  `ChatEvents.kt` has **no `contextUpdate` branch** — the frame is dropped
  today. Add `val context: JsonElement? = null` to `AgentSessionEvent` and a
  reducer branch, decoding with Moshi like the other nested-proto payloads.

Without 7.2 there is no live update path, so the ring would only ever show a
value fetched once at session open.

## 8 Open Questions

- Context ring size next to an 11sp approval chip — a bare percentage is more
  legible at that size, but the ring was chosen for parity. Worth a look on a
  5.4" device.
- Vertical budget. Roughly 165–235dp of chrome before any message renders
  (top strip ~40dp + composer ~90–160dp + bottom strip ~34dp). If it bites,
  hide the top strip when the transcript is scrolled, or collapse non-essential
  action-row glyphs on focus.
- Whether the worktree row should also offer "remove worktree" / orphan cleanup
  (`WorktreeService.RemoveOrphan`, `WorktreePrune` exist server-side).
- Whether `thinkingLevel` should reset when the model changes — currently the
  chip would keep a level the new model may not support.