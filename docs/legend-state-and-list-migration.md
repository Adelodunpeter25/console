# Migrating to Legend State & Legend List

Implementation guide for replacing Zustand with `@legendapp/state` and FlashList
with `@legendapp/list` in `apps/mobile`. Phased so each step ships independently
and can be rolled back without touching later work.

## Why

| Library | Replaces | Win |
| --- | --- | --- |
| `@legendapp/list` | `@shopify/flash-list` | Faster with dynamically sized items (chat messages), built-in chat affordances (`initialScrollAtEnd`, keyboard-aware variants), 100% JS — no native rebuild required |
| `@legendapp/state` | `zustand` | Fine-grained observables — components re-render only on exact nodes they read; faster streaming updates via `batch()` |

Both are by Legend App (same author). Legend List is stable-ish (v3); Legend
State v3 is **beta** — pin the version and expect occasional API drift.

## Current state audit

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
  `apps/mobile/stores/chat/chat-persist.ts`, MMKV-backed
  (`apps/mobile/utils/storage.ts` → `mmkvZustandStorage`), streaming coalescing
  (`_streamBuf` + rAF), cross-store imports (`useAppStore`, `useSessionStore`,
  `useProviderStore`)
- Barrel: `apps/mobile/stores/index.ts`

Server state already lives in TanStack Query (`packages/api`) — do **not**
migrate that; Legend State only replaces client/UI state.

---

# Part A — Legend List

## Phase A0: Install

```sh
cd apps/mobile && bun add @legendapp/list
```

No native code, no config plugin, no EAS rebuild. Import path is
`@legendapp/list/react-native` (v3).

## Phase A1: Chat message list (highest traffic)

Files:

1. `apps/mobile/components/chat/chat-message-list.tsx`
   - Replace `import { FlashList, type FlashListRef } from "@shopify/flash-list"`
     with `import { LegendList, type LegendListRef } from "@legendapp/list/react-native"`
     (verify the exported ref type name against the installed `.d.ts`; v3 may
     expose it from the package root).
   - Swap `<FlashList …>` → `<LegendList …>`.
   - Add `recycleItems` to match FlashList's recycling behavior.
     ⚠️ Recycled cells reuse views — `MessageBubble` must not hold per-item
     state in refs/render-scoped closures. If visual glitches appear, drop
     `recycleItems` first and re-add later; Legend List is fast without it.
   - Keep all existing props (`keyExtractor`, `onScroll`, `onContentSizeChange`,
     header/footer components). They are FlatList-compatible.
2. `apps/mobile/screens/chat/chat-screen.tsx`
   - Change `useRef<FlashListRef<AgentMessage>>(null)` to the Legend ref type.

Optional improvements once basic swap works (keep for a follow-up PR):

- Replace the manual bottom-follow logic (`isAtEndRef`/`followRef` +
  `onContentSizeChange` → `scrollToEnd`) with `initialScrollAtEnd` /
  anchored-end-space props from the v3 API — see
  https://www.legendapp.com/open-source/list/v3/api/
- Consider `KeyboardAwareLegendList` instead of manual
  `keyboardShouldPersistTaps` / dismiss handling.

Verify: `bunx tsc --noEmit`, then `cd apps/mobile && bunx expo export --platform android`.
Manual test: open a long conversation, stream a response, scroll up (pagination),
flip away and back (follow behavior).

## Phase A2: File tree browser

File: `apps/mobile/components/files/FileTreeBrowser.tsx`

- Same import/component swap on the `<FlashList>` at ~line 215.
- No recycling needed yet (`rows` are shallow, uniform-ish); add `recycleItems`
  only if profiling shows benefit.
- `refreshControl`, `ListEmptyComponent` etc. carry over unchanged.

Verify: same commands as A1. Manual test: browse a deep directory, pull-to-refresh, search.

## Phase A3: Cleanup

- Remove `@shopify/flash-list` from `apps/mobile/package.json`.
- Update the doc comment in `apps/mobile/components/chat/message-bubbles.tsx`
  ("chat FlashList" → "chat list").
- Grep for stragglers: `grep -rn "flash-list\|FlashList" apps/mobile --include='*.ts*'`.

---

# Part B — Legend State

Strategy: incremental, one store per PR. Zustand and Legend State coexist fine;
components migrate alongside their store. Order is leaf-stores-first,
`useChatStore` last because it has persistence + cross-store coupling +
streaming coalescing.

## Phase B0: Groundwork

1. `cd apps/mobile && bun add @legendapp/state@beta` (pin exact version in
   `package.json`; bump deliberately).
2. Create `apps/mobile/stores/legend-config.ts`: global sync config registering
   `ObservablePersistMMKV` (from `@legendapp/state/persist-plugins/mmkv`) as the
   default persist plugin, using the existing MMKV instance pattern from
   `apps/mobile/utils/storage.ts`. Call it once from app bootstrap
   (`apps/mobile/app/_layout.tsx`).
3. Document conventions at the top of `apps/mobile/stores/index.ts`:
   - Reads in components via `useValue(store$.path.to.value)`
   - Writes via observable actions (`store$.x.set(...)`, array/map methods)
   - Wrap multi-update bursts in `batch(() => …)` from `@legendapp/state`
   - Never mutate data returned by `.get()` / `.peek()`

## Phase B1: Pilot — leaf store

Migrate `apps/mobile/stores/useSessionStatusStore.ts` (32 lines, no persistence,
few consumers) end-to-end:

1. Rewrite as an observable module:
   ```ts
   // useSessionStatusStore.ts
   import { observable } from "@legendapp/state";
   export const sessionStatus$ = observable({ /* same shape */ });
   ```
   Keep the old zustand hook temporarily as a thin adapter delegating to the
   observable so untouched consumers keep working during transition.
2. Migrate its consumers one by one (`grep -rn "useSessionStatusStore" apps/mobile`).
3. Delete the zustand version once zero consumers remain.

This validates: TypeScript inference, re-render behavior, and the
adapter-then-delete workflow we'll repeat for every store.

Verify: `bunx tsc --noEmit` + expo export; manual smoke of whatever UI reads it.

## Phase B2: Simple stores (repeat pattern per store, one PR each)

Order (least → more consumers):

1. `apps/mobile/stores/useAppStore.ts`
2. `apps/mobile/stores/useProviderStore.ts`
3. `apps/mobile/stores/useAuthStore.ts`
4. `apps/mobile/stores/useSessionStore.ts`
5. `apps/mobile/stores/useProjectStore.ts`
6. `apps/mobile/stores/useFsStore.ts`

Per-store checklist:

- [ ] Observable created; actions exported or attached
- [ ] Cross-store reads: prefer direct imports of the other `$` node over
      copying values (observables compose; zustand needed hooks)
- [ ] All consumers migrated (`grep -rn "<storeName>" apps/mobile`)
- [ ] Adapter removed
- [ ] Typecheck + expo export green

Note: `useEnvironmentsStore.ts` hand-rolls persistence (its local `persist()`
helper writing to storage). When migrating it, replace that helper with
`syncObservable(environments$, { persist: { name: "console-environments",
plugin: ObservablePersistMMKV } })` — see Phase B3 patterns.

## Phase B3: Hard parts

### B3a — Terminal store

File: `apps/mobile/stores/useTerminalStore.ts` (359 lines, high-frequency PTY
output updates — this is where Legend's fine-grained reactivity pays off most).

- Model per-session terminal buffers as observable nodes so only the active
  surface re-renders.
- Batch output chunks: `batch(() => buffer$.push(chunk))` replaces any manual
  rAF throttling.
- Verify against `apps/mobile/modules/console-terminal` contract (initialBuffer
  diffing) — observable identity changes must not break the "only push suffix"
  diff in the native view.

### B3b — Chat store + persistence

Files: `apps/mobile/stores/useChatStore.ts`,
`apps/mobile/stores/chat/chat-persist.ts`, `apps/mobile/stores/chat/draft.ts`,
`apps/mobile/stores/chat/chat-stream-runner.ts`,
`apps/mobile/stores/chat/chat-decisions.ts`

This is the riskiest migration. Do it last, alone in a PR.

1. **Shape**: `sessions: Record<string, ChatSessionState>` becomes
   `sessions$[sessionId].messages` etc. Per-session observable nodes mean
   opening chat A no longer re-renders anything tracking chat B.
2. **Persistence**: replace zustand `persist` middleware entirely with
   `syncObservable(chat$, { persist: { name: "console-chat-cache", plugin:
   ObservablePersistMMKV } })`.
   - Port the partialize/sanitize logic from `chat-persist.ts`
     (`sanitizeSessionPartial`, `MAX_PERSISTED_SESSIONS = 25`,
     `MAX_PERSISTED_MESSAGES = 50`) into the sync config's transform options so
     persisted size stays bounded.
   - The streaming suppress/flush mechanism (`setSuppressPersist` +
     `debouncedStorage`) can likely be deleted — configure the sync engine to
     throttle saves instead, but confirm write frequency during streaming
     before removing.
   - **Bump `PERSIST_VERSION` semantics**: key name stays `console-chat-cache`;
     write a small one-time migration (read old JSON shape → seed observable →
     delete old key) or accept cache loss on upgrade since this is a cache, not
     source of truth. Decide explicitly in the PR.
3. **Streaming coalescing**: `_streamBuf` + `_streamRaf` in `useChatStore.ts`
   map naturally to `batch()` around event application
   (`applyChatEvent`). Keep the coalescing intent: many SSE deltas per frame →
   one notification.
4. **Cross-store calls**: `sendMessage` touches `useAppStore`,
   `useSessionStore`, `useProviderStore` — after Phase B2 these are all
   observables; call their actions directly.
5. **Consumers**: `apps/mobile/hooks/useChatStream.ts` and the chat screens.
   `useValue` on narrow nodes (`streamingText`, `running`, `messages.length`)
   should cut re-renders vs today's whole-snapshot subscriptions.

Verify: full chat flow — send, stream, tool approvals (`ask`), abort,
background/foreground (rehydrate), kill & relaunch (persistence), pagination.

## Phase B4: Cleanup

- Remove `zustand` from `apps/mobile/package.json`.
- Delete leftover adapters and `apps/mobile/stores/chat/chat-persist.ts` if
  fully superseded.
- `grep -rn "zustand" apps/mobile --include='*.ts*'` → zero hits.
- Full verification: `bunx tsc --noEmit && cd apps/mobile && bunx expo export --platform android`.

---

## Rollback strategy

- Every phase is a separate commit/PR; revert restores previous behavior.
- During Part B, zustand ↔ observable adapters make mixed states safe.
- Chat persistence (B3b) is the only place with on-disk format change — keep
  the old-key migration isolated there.

## References

- Legend State v3: https://www.legendapp.com/open-source/state/v3/intro/getting-started/
- Persist & sync: https://www.legendapp.com/open-source/state/v3/sync/persist-sync/
- Legend List v3 (RN): https://www.legendapp.com/open-source/list/v3/react-native/getting-started/
- Legend List API: https://www.legendapp.com/open-source/list/v3/api/
