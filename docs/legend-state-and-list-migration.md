# Legend State & Legend List Migration — ✅ Complete

Status record for replacing Zustand with `@legendapp/state` and FlashList with
`@legendapp/list` in `apps/mobile`. The original phased plan is preserved at the
bottom; all phases shipped between commits `9846eb7` (first Legend List swap)
and `e79448c` (zustand removed).

## Final state

| Concern | Before | After |
| --- | --- | --- |
| Lists | `@shopify/flash-list` 2.0.2 | `@legendapp/list` 3.3.8 (`@legendapp/list/react-native`) |
| State | `zustand` 5.x (10 stores) | `@legendapp/state` 3.0.0-beta.48 (pinned) |
| Chat persistence | zustand `persist` middleware + MMKV | manual MMKV persistence via `chat$.sessions.onChange`, same on-disk format |

## Conventions going forward

- Reads in components: `useValue(store$.field)` from `@legendapp/state/react`
- Derived reads with dynamic keys: `useValue(() => store$.record[key].get())`
- Imperative reads outside render (commands, async work): `.peek()`
- Writes via exported action functions (`store$.x.set(...)`, array methods);
  multi-field updates wrapped in `batch(() => …)`
- Never mutate data returned by `.get()` / `.peek()`

Store modules keep their old file names (`useAppStore.ts`, etc.) but export
observable nodes (`app$`, `provider$`, …) plus plain action functions.

## What shipped

### Part A — Legend List
- **A1** `components/chat/chat-message-list.tsx` + `screens/chat/chat-screen.tsx` — component/ref swap. `LegendListRef` is non-generic (vs `FlashListRef<T>`). `className` → `style={{ flex: 1 }}` (no NativeWind interop registered). Confirmed smoother on device at equal message count.
- **A2** `components/files/FileTreeBrowser.tsx` — same swap.
- **A3** `@shopify/flash-list` removed; zero straggler references.
- Deliberately skipped: `recycleItems` (FlashList v2 didn't recycle either), `initialScrollAtEnd`, `KeyboardAwareLegendList` — parked as future tuning (see below).

### Part B — Legend State (one commit per store)
| Store | Commit |
| --- | --- |
| `useSessionStatusStore` → `sessionStatuses$` (pilot) | `b4cccf4` |
| `useAppStore` → `app$` (22 consumer files) | `236e2ef` |
| `useProviderStore` → `provider$` | `348d165` |
| `useAuthStore` → `auth$` | `59fa374` |
| `useSessionStore` → `sessionsView$` | `a82ef66` |
| `useProjectStore` → `project$` | `8189b69` |
| `useFsStore` → `fs$` | `4ace18d` |
| `useEnvironmentsStore` → `environments$` | `879357a` |
| `useTerminalStore` → `terminals$` / `terminalBuffers$` | `dc965b6` |
| `useChatStore` → `chat$` + confirm-dialog + drop zustand | `e79448c` |

Notable decisions & deviations from the original plan:

- **No adapters needed.** Every consumer fit cleanly into imperative-action or
  reactive-read patterns, so stores were migrated straight to their final shape
  (all consumers updated in the same commit).
- **Environments persistence kept hand-rolled.** Its dual-key format (env list +
  legacy single-URL key for downgrade safety) doesn't map to
  `ObservablePersistMMKV`; the explicit `persist()` calls were preserved instead
  of switching to `syncObservable`.
- **Chat persistence format is unchanged.** Same key (`console-chat-cache`),
  same `{ state: { sessions }, version: 1 }` wrapper — upgrades *and* downgrades
  are lossless, no version bump required. Writes throttle at 300 ms; the
  streaming suppress/flush mechanism (`setSuppressPersist`) was kept rather than
  deleted, since it prevents MMKV writes during high-frequency stream deltas.
- **Terminal `opening` promises moved off reactive state** into a module-level
  `Map` — in-flight promises are not UI data.
- **`confirm-dialog.tsx`** also migrated (`confirmDialog$`) so `zustand` could
  be fully removed.
- **B0 config file never created.** No store uses `syncObservable` plugins, so
  there was nothing to configure globally.

## Verification performed

- `bunx tsc --noEmit` clean after every phase
- `bunx expo export --platform android` green after each list/store milestone
- Legend List verified on device (smoother scrolling reported at identical
  message counts)

## On-device regression checklist (recommended after upgrade)

1. Chat: send → stream renders; stop button aborts; tool approvals/answers clear panels
2. Persistence: send message → kill app → relaunch restores messages and drafts
3. Terminal: open, type, resize (rotation/keyboard), kill → respawn
4. Environments: switch/add/remove environments; disconnect-all clears server-scoped caches
5. Home drafts: typed-but-unsent chats survive restart and show as DRAFT

## Future tuning (parked)

- `recycleItems` on the chat list (requires converting per-bubble `useState`
  — `expanded`/`copied`/`previewUri` in `message-bubbles.tsx` — to
  `useRecyclingState`)
- `initialScrollAtEnd` / `maintainScrollAtEnd` to replace manual follow-scroll
  in `chat-message-list.tsx`
- `KeyboardAwareLegendList` requires bumping
  `react-native-keyboard-controller` to ≥ 1.21.7 (currently 1.18.5) plus a new
  dev build, and reworking composer insets
- Legend State beta bumps: pin is exact (`3.0.0-beta.48`); bump deliberately

---

# Original plan (for reference)

## Why

| Library | Replaces | Win |
| --- | --- | --- |
| `@legendapp/list` | `@shopify/flash-list` | Faster with dynamically sized items (chat messages), built-in chat affordances (`initialScrollAtEnd`, keyboard-aware variants), 100% JS — no native rebuild required |
| `@legendapp/state` | `zustand` | Fine-grained observables — components re-render only on exact nodes they read; faster streaming updates via `batch()` |

Both are by Legend App (same author). Legend List is stable-ish (v3); Legend
State v3 is **beta** — pin the version and expect occasional API drift.

## Original audit (pre-migration)

**Lists (FlashList):**

- `apps/mobile/components/chat/chat-message-list.tsx` — `<FlashList>` + `FlashListRef<AgentMessage>`, manual follow-scroll (`onContentSizeChange` → `scrollToEnd`), top-pagination on scroll, `ListHeaderComponent`/`ListFooterComponent`
- `apps/mobile/components/files/FileTreeBrowser.tsx` — plain `<FlashList>` with `RefreshControl`
- `apps/mobile/screens/chat/chat-screen.tsx` — `FlashListRef<AgentMessage>` type only
- `apps/mobile/components/chat/message-bubbles.tsx` — doc-comment mention only

**Stores (Zustand, ~1,700 lines across 10 files):**

- `apps/mobile/stores/useAppStore.ts` (36) — no persistence
- `apps/mobile/stores/useSessionStatusStore.ts` (32) — no persistence
- `apps/mobile/stores/useProviderStore.ts` (88) — no persistence
- `apps/mobile/stores/useSessionStore.ts` (125) — no persistence
- `apps/mobile/stores/useAuthStore.ts` (161) — no persistence
- `apps/mobile/stores/useProjectStore.ts` (154) — no persistence
- `apps/mobile/stores/useFsStore.ts` (176) — no persistence
- `apps/mobile/stores/useEnvironmentsStore.ts` (199) — hand-rolled persistence helper
- `apps/mobile/stores/useTerminalStore.ts` (359) — no persistence
- `apps/mobile/stores/useChatStore.ts` (360) — zustand `persist` middleware via
  `apps/mobile/stores/chat/chat-persist.ts`, MMKV-backed, streaming coalescing
  (`_streamBuf` + rAF), cross-store imports
- Barrel: `apps/mobile/stores/index.ts`

Server state lives in TanStack Query (`packages/api`) — intentionally **not**
migrated; Legend State only replaced client/UI state.

## Phases (all complete)

- [x] A0: Install `@legendapp/list`
- [x] A1: Chat message list swap (`9846eb7`)
- [x] A2: File tree browser swap (`9846eb7`)
- [x] A3: Remove FlashList (`0f97ac7`)
- [x] B0: Install `@legendapp/state@beta` (with B1, `b4cccf4`)
- [x] B1: Pilot — session status store (`b4cccf4`)
- [x] B2: app/provider/auth/session/project/fs stores (`236e2ef`, `348d165`, `59fa374`, `a82ef66`, `8189b69`, `4ace18d`)
- [x] B3a: Terminal store (`dc965b6`)
- [x] B3b: Chat store + persistence (`e79448c`)
- [x] B4: Remove zustand (`e79448c`)

The detailed step-by-step instructions for each phase were executed as written;
deviations are documented in "What shipped" above.

## References

- Legend State v3: https://www.legendapp.com/open-source/state/v3/intro/getting-started/
- Persist & sync: https://www.legendapp.com/open-source/state/v3/sync/persist-sync/
- Legend List v3 (RN): https://www.legendapp.com/open-source/list/v3/react-native/getting-started/
- Legend List API: https://www.legendapp.com/open-source/list/v3/api/
