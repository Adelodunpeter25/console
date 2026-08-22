# Mobile Review — `apps/mobile`

> **Date:** 2026-08-22  
> **Scope:** Full scan of `apps/mobile` — config, deps, Metro, state, hooks, stores, components, screens, native modules, persistence, streaming, theming, build.  
> **Stack:** Expo **54.0.0** / React Native **0.81.5** / React **19.1.0** / TypeScript **5.9** (strict) / Zustand **5.0** / TanStack Query **5.101** / MMKV **4.3** / Uniwind + Tailwind **4.3**  
> **Entry:** `apps/mobile/index.tsx` → `AppRoot` (Onboarding vs `MainContent`)  
> **Build:** EAS (`eas.json`), prebuilt `android/` (Gradle), custom native modules under `modules/`

---

## Table of Contents

1. [Overview](#overview)
2. [Project Structure](#project-structure)
3. [Dependencies & Config](#dependencies--config)
4. [Architecture & Data Flow](#architecture--data-flow)
5. [State & Persistence](#state--persistence)
6. [Streaming & Realtime](#streaming--realtime)
7. [Navigation & UI Shell](#navigation--ui-shell)
8. [Styling & Theming](#styling--theming)
9. [Build, Assets & Deploy](#build-assets--deploy)
10. [What's Good](#whats-good)
11. [Issues — Ranked (P0 / P1 / P2)](#issues--ranked-p0--p1--p2)
12. [Recommendations & Quick Wins](#recommendations--quick-wins)
13. [Checklist](#checklist)
14. [Appendix — File References](#appendix--file-references)

---

## Overview

`apps/mobile` is the Expo-managed companion to the Console desktop/server — chat-centric, with session/project browsing, markdown + diff rendering, bottom-sheet pickers, and SSE streaming. It mirrors the desktop chat runtime (events, runs, tool calls, drafts) via shared packages `@console/api` / `@console/types` and a Zustand + MMKV stack tuned for Android (native OkHttp SSE, localhost OAuth server). No tests, no lint config, no `ErrorBoundary`. Strong foundation; release blockers are networking/OAuth/stream lifecycle.

---

## Project Structure

```
apps/mobile/
├── index.tsx                # AppRoot — fonts, QueryClientProvider, Gesture/SafeArea/Keyboard/BottomSheet, Onboarding | MainContent
├── query-client.ts          # Shared QueryClient (15s staleTime, focusManager)
├── app.json / eas.json / metro.config.js / babel.config.js / tsconfig.json / global.css
├── components/
│   ├── chat/                # 13 files: composer, message-list, bubbles, run-activity, tool-call-block, diff-view, interaction/question/approval panels, selectors
│   ├── common/              # confirm-dialog, markdown-renderer, syntax-highlighter, search-bar, session-sub-list, skeleton, image-preview
│   ├── context-menu/        # base-context-menu, file-context-menu, session-action-sheet
│   ├── home/session-list.tsx
│   └── layout/              # app-shell, main-content, screen-header, glass-surface
├── hooks/                   # 17 files (see Architecture)
├── stores/                  # useAppStore, useChatStore + chat/{persist,stream-runner,decisions,draft}, useSessionStore, useProjectStore, useAuthStore, +3
├── screens/                 # home/home-screen, chat/chat-screen, settings/*, projects/add-project-screen
├── modules/
│   ├── native-stream/       # Android OkHttp SSE (NativeStreamModule) + XHR fallback
│   └── local-auth-server/   # Android localhost OAuth redirect server (LocalAuthServerModule)
├── utils/                   # storage(MMKV), chat-events, reconstruct-runs, native-stream, tool-helpers, sse/diff/format/time
├── styles/theme.ts          # JS theme tokens
├── assets/                  # icon, splash-icon, android adaptive icons, favicon
└── android/                 # Prebuilt (gradle, mipmap-*, MainActivity.kt) — committed
```

Monorepo aliases (`tsconfig.json:7`): `@console/api → packages/api/src`, `@console/types → packages/types/src`, resolved in `metro.config.js:24-25` and `extraNodeModules`.

---

## Dependencies & Config

**`package.json` highlights**

- Expo SDK 54, RN 0.81.5, React 19.1, reanimated 4.1, gesture-handler 2.28, safe-area 5.6, screens 4.16, mmkv 4.3, flash-list 2.0.2, bottom-sheet 5.2, keyboard-controller 1.18, enriched-markdown + markdown-display, prismjs, diff, zustand, react-query.
- Dev: `babel-preset-expo`, `typescript`, `@types/*`.

**Notable config**

- `app.json`: `name Console`, `slug console`, `orientation portrait`, `icon ./assets/icon.png`, `userInterfaceStyle light`, `ios.bundleIdentifier com.console.mobile`, `android.package com.console.mobile`, `android.usesCleartextTraffic true`, adaptive icons (foreground/background/monochrome), `predictiveBackGestureEnabled false`, plugins `expo-font` + `react-native-enriched-markdown` (math + codeHighlight), `extra.eas.projectId`, `owner adelodunpeter`, `sdkVersion 54.0.0` (deprecated — SDK now inferred from `expo`).
- `eas.json`: `cli >=14`, `appVersionSource remote`, builds `development` (devClient apk), `preview` (apk), `release` (autoIncrement, apk) — all `apk` (should be `aab` for Play Store), `submit.production {}`.
- `metro.config.js`: watches `workspaceRoot`, `nodeModulesPaths` local then workspace, `extraNodeModules` pins `react`, `react-native`, `react-query` to mobile's node_modules + maps `@console/api|types`, wraps `withUniwindConfig(cssEntry ./global.css, dts ./uniwind-types.d.ts)`.
- `babel.config.js`: `babel-preset-expo` only (no explicit `react-native-reanimated/plugin` — works via Expo but be explicit).
- `tsconfig.json`: `extends expo/tsconfig.base`, `strict true`, `baseUrl .`, `paths` for `@console/*`, `include **/*`, `exclude node_modules`.

**.gitignore (apps/mobile)** — Correctly ignores `node_modules`, `.expo`, `dist`, native build dirs, `*.apk`, `.DS_Store`, `*.tsbuildinfo`. `git ls-files` clean; local `build-1787328543751.apk` (127 MB) untracked but present on disk.

**Installed size** — `node_modules` present; `android/` prebuilt committed (mipmaps, gradle wrapper). `package-lock.json` 352 KB suggests npm install (monorepo uses pnpm elsewhere — mixed lockfiles).

---

## Architecture & Data Flow

**Boot (`index.tsx:60 AppRoot`)**

1. `useServerConnection()` loads `backendUrl` from MMKV (`@console_backend_url`) → `configureConsoleApi({baseUrl})`.
2. `useFonts` (JetBrainsMono 4 weights).
3. `loading || !fontsLoaded` → spinner.
4. `QueryClientProvider` → `GestureHandlerRootView` → `SafeAreaProvider` → `View #0a0a0b` → `StatusBar light` → `backendUrl ? (KeyboardProvider → BottomSheetModalProvider → MainContent) : OnboardingScreen` + `ConfirmDialog`.

**Queries (`hooks/queries.ts`)**

- `useSessions(params?)` / `useProjects` / `useInfiniteSession(id, limit=100)` / `useSession` — all `staleTime 15s`, `refetchOnWindowFocus/Reconnect`, `refetchInterval 15s` (background false where set). Keys from `@console/api` `sessionKeys`/`fsKeys`. Mutations: `useCreate/Update/Delete/Restore/PermanentlyDeleteSession`, `useAdd/DeleteProject`, `useFsBrowse/Tree/ReadFile`.
- `prefetchSession(queryClient, id)` prefetches `sessionKeys.detail(id)` with `staleTime 60s`.
- `hooks/useHomeSessions.ts` orchestrates home: `useProjects` + `useSessions` + `useProjectBranches`, filters by `searchQuery`, builds `Drafts` section from `useChatStore.sessions` (0 msgs + `isDraftSession`), groups by project (by `cwd` or `projectId`), sorts sections by `latestAt`, prefetches top 5 sessions after 300ms, exposes `openSession/composeSession/deleteSession/prefetchSession/navigateToSettings`.

**Chat (`hooks/useChatStream.ts` + `stores/useChatStore.ts`)**

- `useChatStream` selects `selectedSessionId` from `useAppStore`, drives `useInfiniteSession(id,100)`, derives `snapshot = getSnapshot(id)` + `input`, exposes `sendMessage/abort/setInput/loadMessages`, memoizes `allMessages = reverse(pages).flatMap(messages)`, syncs `loadMessages` + `useSessionStore` header (`modelId/provider/cwd/approvalMode`) on `allMessages/latestHeader`.
- `useChatStore` (persisted via `chat-persist.ts`): `sessions: Record<id, ChatSessionState>`, `loadMessages` (skips if `running`), `setInput/addAttachments/removeAttachment/clearAttachments`, `sendMessage` (validates image support vs `useProviderStore.modelsByProvider`, appends optimistic user message + new `run`, calls `startNativeChatStream(url=/api/sessions/:id/run, body={prompt, attachments, modelId, provider, approvalMode})`), `abort/answerQuestion/approvePermission/handleEvent/getSession/getSnapshot`. Events via `utils/chat-events.ts:46 applyChatEvent` (handles `modelStreamPart/End`, `toolExecutionStart/Result/End`, `askQuestion/permissionRequest/todoUpdate/sessionEnd/error`) and `stores/chat/chat-stream-runner.ts` (`finalizeSessionRun`, `abortSessionStream`, `syncSessionStatus`).

**Other hooks**

- `useAuth` / `useLocalOAuthLogin` / `useServerConnection` / `useNotificationStream` / `useProjectBranches/Sessions/FsWatcher/Terminal/Git/SlashCommands/FileSearch/Abort/ChatDecisions/Providers` — see Issues.

---

## State & Persistence

- **`useAppStore`** (`stores/useAppStore.ts:16`): `activeTab: home|chat|settings`, `selectedProjectId`, `selectedSessionId`, `backendUrl` — no persistence (backendUrl persisted separately via MMKV key by `useServerConnection`).
- **`useChatStore`** (`stores/useChatStore.ts:46`): `persist(chatPersistConfig)` with `PERSIST_NAME console-chat-cache`, `MAX_PERSISTED_SESSIONS 25`, `MAX_PERSISTED_MESSAGES 50`, `partialize` keeps sessions with `messages>0 || hasPersistableDraft`, sorted by `draftUpdatedAt` then `messages.length`, sliced, caps attachments via `trimDraftAttachments`. `merge` spreads persisted partials onto `createChatSessionState()` — **unvalidated spread** (risk).
- **`useSessionStore`** (`stores/useSessionStore.ts:50`): `sessions: Record<id, SessionViewState {sessionModelId, sessionProvider, sessionCwd, approvalMode}>`, `loadSession/changeModel/changeProject/setApprovalMode` (project lock if `hasMessagesChecker`), syncs `useSessionStatusStore` + `useProjectStore.refreshSessionHeader`.
- **`useProjectStore`** (`stores/useProjectStore.ts:29`): `projects`, `sessions`, `deletedSessions` + loaders/mutators, invalidates `sessionKeys.all`.
- **`useAuthStore`** (`stores/useAuthStore.ts:34`): `status/loading/loggingIn/error/projectIds/savingProjectId`, `loadStatus/loginWithBrowser/loginCodebuff/saveProjectId/reset`. `loginWithBrowser` fetches login URL then `openAuthUrl` + `loadStatus` — no deep-link exchange (handled elsewhere).
- **Storage** (`utils/storage.ts:4`): `appStorage = createMMKV({id: console-mobile-storage})`, `mmkvZustandStorage` adapter (sync JSI).
- **Other stores**: `useProviderStore`, `useSessionStatusStore`, `useFsStore`, `useTerminalStore` + `stores/chat/draft.ts` (`MAX_DRAFT_IMAGES 2`, `isDraftSession/hasPersistableDraft/trimDraftAttachments/draftPreview`).

---

## Streaming & Realtime

- **`modules/native-stream/index.ts:32`** `startNativeChatStream(streamId, url, body, callbacks, headers)`: if `isNativeStreamAvailable()` (Android + `NativeStreamModule` + `emitter`), subscribes to `onStreamEvent/onStreamError/onStreamEnd` (filter by `streamId`, `finished` guard), calls `NativeStreamModule.startChatStream(streamId, url, JSON.stringify(body), headers)`, returns cleanup that `abortStream(streamId)`. **Fallback** (`108`): `XMLHttpRequest` POST with `onprogress` SSE parsing (`data: ` lines), `onload/onerror` → `onEnd`.
- **`modules/native-stream/index.ts:161`** `startNativeNotificationStream(streamId, url, callbacks)`: native `onNotificationEvent/onNotificationError` vs fetch reader-loop fallback (`214`).
- **`utils/native-stream.ts`** re-exports native-stream; **`utils/sse.ts`**, **`utils/app-focus-manager.ts:11`** (`setupAppFocusManager` bridges `AppState → focusManager`), **`utils/chat-events.ts`** / **`utils/reconstruct-runs.ts`** complete the pipeline.

---

## Navigation & UI Shell

- **No router** — tab state via `useAppStore.activeTab`. `components/layout/main-content.tsx:7` renders `AppShell` with: `Home` always mounted (`View display: home?flex:none` to preserve scroll), `Chat`/`Settings` conditionally mounted.
- **`components/layout/app-shell.tsx:10`** — thin `View #0a0a0b` wrapper; comment notes safe areas handled by `ScreenHeader` + bottom bars to avoid double insets.
- **Screens**: `screens/home/home-screen.tsx` (SearchBar sticky, SessionList, SessionActionSheet, BackHandler), `screens/chat/chat-screen.tsx` (ChatMessageList via FlashList, ChatScrollBottomButton, Composer vs InteractionPanel, BackHandler), `screens/settings/*` (account/connection/deleted-chats/projects).
- **Components**: `components/chat` (composer, chat-message-list, message-bubbles, run-activity, tool-call-block, diff-view, interaction-panel, live-tool-results, selectors), `components/common` (confirm-dialog, markdown-renderer, skeleton, etc.).

---

## Styling & Theming

- **`global.css`**: `@import tailwindcss + uniwind`, `@theme` tokens `--color-screen #0a0a0b`, `--color-card #121316`, border `rgba(255,255,255,0.12/0.06)`, foregrounds `#fff/#a1a1aa/#71717a`, `destructive #f87171`, `font-mono JetBrainsMono`.
- **`styles/theme.ts`**: `colors.background #0d0d0e`, `backgroundAlt #0a0a0b`, `surface #16171a`, `surfaceElevated #1f2024`, `text/danger/status` — **diverges from `global.css`** and hardcoded `#0a0a0b` in `app-shell.tsx:11` / `index.tsx:88`.
- **Type safety**: `uniwind-types.d.ts` generated; `tsconfig strict true`.

---

## Build, Assets & Deploy

- **Assets**: `assets/icon.png`, `splash-icon.png` (legacy), `android-icon-{foreground,background,monochrome}.png`, `favicon.png`. Mipmaps generated (`android/app/src/main/res/mipmap-*`). `LICENSE` present.
- **Android**: `android/` prebuilt (gradle wrapper, `MainActivity.kt`, `MainApplication.kt`, `AndroidManifest.xml`, `build.gradle`, `gradle.properties`). `app.json android.predictiveBackGestureEnabled false`.
- **EAS**: `eas.json` remote version, `development` (devClient apk internal), `preview` (apk internal), `release` (autoIncrement, apk internal) — all `apk` (Play Store requires `aab`). No `submit` config.
- **Scripts** (`package.json:6`): `start`, `android`, `ios`, `web`, `eas-build-pre-install` (curls `bun canary` → hermeticity risk).
- **Artifact on disk**: `apps/mobile/build-1787328543751.apk` (127.7 MB) — untracked (`.gitignore *.apk`) but wastes disk / risk if committed.

---

## What's Good

| Area | Detail | File |
|------|--------|------|
| Metro monorepo | Watches workspace, pins `react`/`react-native`/`react-query` singletons, maps `@console/*` | `metro.config.js:10-26` |
| Query + focus | `staleTime 15s`, `refetchOnWindowFocus/Reconnect`, `refetchIntervalInBackground:false`, `AppState → focusManager` | `query-client.ts:14`, `utils/app-focus-manager.ts:14`, `hooks/queries.ts` |
| Chat runtime parity | Mirrors desktop: `loadMessages → reconstructRuns`, `applyChatEvent`, `chat-stream-runner` | `stores/useChatStore.ts:51`, `utils/chat-events.ts:46`, `utils/reconstruct-runs.ts` |
| Native streaming | Android OkHttp SSE with `streamId` filtering + XHR fetch fallback for iOS/web | `modules/native-stream/index.ts:32-159` |
| Persistence | MMKV JSI (`createMMKV`) + `mmkvZustandStorage`, `partialize` caps 25×50, draft-aware | `utils/storage.ts:4`, `stores/chat/chat-persist.ts:18` |
| Draft UX | `MAX_DRAFT_IMAGES 2`, `draftPreview`, `Drafts` section survives restarts | `stores/chat/draft.ts:14`, `hooks/useHomeSessions.ts:63` |
| Layout | Home stays mounted (`display:none`) to preserve scroll; AppShell avoids double SafeArea | `components/layout/main-content.tsx:19`, `components/layout/app-shell.tsx:10` |
| Grouping | Sessions grouped by project (`cwd`/`projectId`), sections sorted by `latestAt`, prefetch top 5 | `hooks/useHomeSessions.ts:104-154` |

---

## Issues — Ranked (P0 / P1 / P2)

### P0 — Fix before release

#### 1. HTTP cleartext fallback — credentials in cleartext
- **Where:** `hooks/useServerConnection.ts:60`/`89` prepends `http://` if no scheme; `app.json:16` `android.usesCleartextTraffic true`; `index.tsx:38` placeholder `http://192.168.1.X:3000`.
- **Risk:** Backend tokens/creds sent over plain HTTP on LAN; Play Store may flag cleartext. User-entered URL silently downgraded.
- **Fix:** Default to `https://`, show warning/confirmation for `http://`, scope `usesCleartextTraffic` to debug or add `android/app/src/main/res/xml/network_security_config.xml` allowing only `192.168.*` cleartext. Update placeholder.

#### 2. OAuth wiring fragmented — deep link never fires without `scheme`
- **Where:** 3 paths: `stores/useAuthStore.ts:63` `loginWithBrowser` (open browser + `loadStatus`, no callback exchange), `hooks/useAuth.ts:78` `useOAuthDeepLink` (+ `submitCallback` → `authService.handleCallback`), `hooks/useLocalOAuthLogin.ts:84` localhost server (Android-only, 2-min timeout, lifecycle via `stopServerAndCleanup`). `hooks/useAuth.ts:14` `useAuth` + `useLocalOAuthLogin` not mounted in `index.tsx`. `stores/useAuthStore.ts:84` `loginCodebuff` is no-op (just `loadStatus`). `app.json` has **no** `expo.scheme` / `intentFilters` / `scheme`.
- **Risk:** `console://auth?code=…` deep link never delivered; localhost server dies on iOS suspend; browser flow appears to succeed but never exchanges code.
- **Fix:** Add `expo.scheme: "console"` + `android.intentFilters` / `ios.associatedDomains` in `app.json`; mount `useOAuthDeepLink()` in `AppRoot`; unify login to `useLocalOAuthLogin` on Android + deep-link fallback; implement or remove `loginCodebuff`.

#### 3. Stream lifecycle — leaks, double-end, stale pagination effect
- **Where:** `modules/native-stream/index.ts:82` `cleanup` removes listeners but `abortStream` not awaited / not idempotent; `finished` guard vs `cancelled` split between chat/notification. XHR fallback `143-148` `onload` sets `hadError` only on `status>=400` but `onerror` also calls `onEnd(true)` — caller `chat-stream-runner.ts:49 finalizeSessionRun` receives ambiguous `hadError/aborted`. `hooks/useChatStream.ts:75` `useEffect` deps include `allMessages` (new array each pagination) → re-fires `loadMessages` on every `fetchNextPage`; `stores/useChatStore.ts:54` bails if `running` masking races but leaves stale `latestHeader` sync (`useSessionStore` setState).
- **Risk:** Listener leaks, double `onEnd`, dropped tool results, pagination re-loads clobbering active run.
- **Fix:** Make `finished/cancelled` single idempotent flag, await `abortStream`, guard `onError` vs `onEnd` (emit one), memoize `allMessages` by stable key or hash, skip `loadMessages` when `running`, debounce `latestHeader` sync.

---

### P1 — High

#### 4. Build artifact & EAS shape
- **Where:** `apps/mobile/build-1787328543751.apk` (127 MB) on disk; `eas.json:7-26` all `buildType apk` (including `release`), `appVersionSource remote`, empty `submit.production`.
- **Risk:** Disk waste, accidental commit; Play Store rejects `apk` for new apps (requires `aab`), remote version needs EAS project linkage.
- **Fix:** Delete `build-*.apk` (already gitignored), set `release.buildType: app-bundle`, configure `submit` or remove `appVersionSource: remote` if not using EAS Updates.

#### 5. Duplicate deps — bundle bloat
- **Where:** `package.json:17` `@hugeicons/core-free-icons` + `31` `hugeicons-react-native` (legacy `0.0.2`), `18` `@hugeicons/react-native` (current), `36` `react-native-enriched-markdown` + `39` `react-native-markdown-display`, `32` `lucide-react-native` vs hugeicons, `33` `prismjs` (+ `51` `@types`) unused if enriched-markdown handles highlighting.
- **Risk:** Larger bundle, duplicate icon sets, conflicting markdown renderers.
- **Fix:** Keep `@hugeicons/react-native` + `@hugeicons/core-free-icons`, remove `hugeicons-react-native`; pick one markdown lib (likely `react-native-enriched-markdown` with `codeHighlight`); remove unused `prismjs`/`lucide` if not used.

#### 6. No error boundary / offline UX
- **Where:** `index.tsx:60` no `ErrorBoundary`; `hooks/useChatStream.ts` / `hooks/queries.ts` no offline banner; `hooks/useServerConnection.ts:84` `testConnection` (6s timeout) never shown in `OnboardingScreen` (`index.tsx:24`).
- **Risk:** White screen on render error; no feedback when backend unreachable.
- **Fix:** Wrap `MainContent` in `ErrorBoundary` (react-error-boundary or custom), add `NetInfo` offline banner, wire `testingStatus` to onboarding (Test button + spinner).

#### 7. Persist risks — unvalidated merge, stale queries
- **Where:** `stores/chat/chat-persist.ts:44` `merge: ...partial` spreads unvalidated JSON onto `ChatSessionState`; no `version`/`migrate`. `MAX_PERSISTED_SESSIONS 25 * 50 msgs` can exceed MMKV size. `hooks/useServerConnection.ts:14 clearChatCache` clears `sessions` + `persist.clearStorage()` but `queryClient.clear()` only on `disconnect:115` — switching URL leaves stale `sessionKeys`/`fsKeys` in cache.
- **Risk:** Corrupt persisted state crashes on upgrade; large payload; stale sessions after URL change.
- **Fix:** Add `version` + `migrate` with Zod/io-ts validation, cap total bytes, call `queryClient.clear()` in `clearChatCache` and on `saveConnection` URL change, add `onRehydrateStorage` guard.

#### 8. No types/tests/lint — `any` and hermeticity
- **Where:** `stores/chat/chat-persist.ts:15` `chatPersistConfig: any` (also `updateSession: any` via `partial`), zero `*.test.*` files, no `eslint`/`prettier` config, `package.json:11` `eas-build-pre-install` curls `bun canary` and copies to `/usr/local/bin`.
- **Risk:** Regressions, untyped persist, non-hermetic builds.
- **Fix:** Type `chatPersistConfig` (`PersistOptions<ChatStoreState>`), add `eslint` + `tsc --noEmit` in CI, basic store/utils tests, pin `bun` version or remove installer.

---

### P2 — Medium

#### 9. Theming split
- **Where:** `styles/theme.ts` (`#0d0d0e` etc.) vs `global.css:5` (`#0a0a0b` `--color-screen`) vs hardcoded `#0a0a0b` in `components/layout/app-shell.tsx:11` / `index.tsx:88` / `query-client` not themed.
- **Fix:** Single token source (e.g. `styles/theme.ts` → CSS vars or vice versa), remove hardcodes.

#### 10. Re-render polish
- **Where:** `hooks/useHomeSessions.ts:60` `useChatStore(s=>s.sessions)` subscribes to entire map → home re-renders on any draft keystroke; `hooks/useChatStream.ts:27` `snapshot` selector uses `useCallback` but `selectedSessionId` dep causes churn; `screens/chat/chat-screen.tsx:50` `hasPendingInteraction` inline.
- **Fix:** Select `isDraftSession` map or `Object.keys` length, use `useShallow` / memoized selectors, memoize derived booleans.

#### 11. Config hygiene
- **Where:** `app.json:9` `sdkVersion 54.0.0` deprecated, `babel.config.js` missing explicit `react-native-reanimated/plugin`, `utils/sse.ts` (1.2 KB) unused (native-stream handles SSE), `assets/splash-icon.png` legacy.
- **Fix:** Remove `sdkVersion`, add reanimated plugin if needed, delete or wire `utils/sse.ts`, use `expo-splash-screen` config.

#### 12. Minor code
- **Where:** `hooks/useHomeSessions.ts:198 onRefresh` invalidates `["sessions"]`/`["projects"]` (string keys) vs `sessionKeys.all` / `fsKeys.projects` — inconsistent; `stores/useProjectStore.ts:52` also invalidates `["projects"]`. `utils/tool-helpers.ts` `as any` in `resultText:186`, `stores/chat/chat-decisions.ts` swallows errors with `console.error` only.
- **Fix:** Use `sessionKeys`/`fsKeys` consistently, type `resultText`, surface decision errors to UI.

---

## Recommendations & Quick Wins

**P0 — Do first (small PRs)**

1. **Network:** `hooks/useServerConnection.ts` default `https://`, confirm dialog for `http://`; gate `usesCleartextTraffic` or add `network_security_config.xml`; update `index.tsx:38` placeholder.
2. **OAuth:** Add to `app.json`:
   ```json
   "scheme": "console",
   "android": { "intentFilters": [{ "action": "VIEW", "data": [{ "scheme": "console", "host": "auth" }], "category": ["BROWSABLE","DEFAULT"] }] }
   ```
   Mount `useOAuthDeepLink()` in `AppRoot`, unify login, remove or implement `loginCodebuff`.
3. **Stream:** Unify `finished` flag, await `abortStream`, emit `onEnd` once, guard `useChatStream` effect vs `running` + stable pagination key.

**P1 — Next**

4. Delete `build-*.apk`, set `eas.json release.buildType: "app-bundle"`.
5. Dedupe deps: remove `hugeicons-react-native`, pick one markdown lib, remove unused `prismjs`/`lucide` if unused.
6. Add `ErrorBoundary` + `NetInfo` banner, wire `testConnection` to onboarding UI.
7. Add persist `version`/`migrate` + Zod validation, `queryClient.clear()` on URL change, cap bytes.
8. Add `eslint` + `tsc --noEmit` CI, type `chatPersistConfig`, pin `bun` or remove installer.

**P2 — Polish**

9. Unify theming tokens, remove hardcodes.
10. Optimize selectors (`useShallow`), dedupe `sessionKeys` usage, add `reanimated/plugin`.

---

## Checklist

- [ ] P0: HTTPS-by-default + cleartext scoping + placeholder
- [ ] P0: `expo.scheme` + intentFilters + `useOAuthDeepLink` mounted + login unified
- [ ] P0: Stream lifecycle idempotent + single `onEnd` + pagination guard
- [ ] P1: Remove `build-*.apk`, `eas release → app-bundle`
- [ ] P1: Dedupe markdown/icon/prismjs deps
- [ ] P1: ErrorBoundary + offline banner + Test Connection UI
- [ ] P1: Persist version/migrate + validation + query clear on URL switch
- [ ] P1: ESLint + `tsc --noEmit` + typed persist
- [ ] P2: Single theme source
- [ ] P2: Selector/memo perf + reanimated plugin

---

## Appendix — File References

**Key files inspected (80+ sources, `glob apps/mobile/**/*.{ts,tsx,js,json}`)**

- `index.tsx:60` `AppRoot`, `index.tsx:24` `OnboardingScreen`
- `query-client.ts:13` `QueryClient`, `utils/app-focus-manager.ts:11` `setupAppFocusManager`
- `app.json:16` `usesCleartextTraffic`, `eas.json:6` builds, `metro.config.js:10-26`, `babel.config.js:2`, `tsconfig.json:4` strict
- `hooks/useServerConnection.ts:14` `clearChatCache`, `hooks/useServerConnection.ts:84` `testConnection`
- `hooks/useChatStream.ts:20` `useChatStream`, `hooks/useHomeSessions.ts:26` `useHomeSessions`
- `hooks/useAuth.ts:14` `useAuth`, `hooks/useAuth.ts:78` `useOAuthDeepLink`, `hooks/useLocalOAuthLogin.ts:56` `useLocalOAuthLogin`
- `stores/useChatStore.ts:46` `useChatStore`, `stores/chat/chat-persist.ts:18` `partialize`, `stores/chat/chat-stream-runner.ts:49` `finalizeSessionRun`
- `stores/useAuthStore.ts:63` `loginWithBrowser`, `stores/useSessionStore.ts:50`, `stores/useProjectStore.ts:29`
- `modules/native-stream/index.ts:32` `startNativeChatStream`, `modules/local-auth-server/index.ts:34` `isLocalAuthServerAvailable`
- `utils/storage.ts:4` MMKV, `utils/chat-events.ts:46` `applyChatEvent`, `utils/reconstruct-runs.ts:12` `reconstructRuns`
- `components/layout/main-content.tsx:19`, `components/layout/app-shell.tsx:10`, `screens/home/home-screen.tsx:16`, `screens/chat/chat-screen.tsx:17`
- `styles/theme.ts:2`, `global.css:4` `@theme`

**Git state at review**

- `git ls-files apps/mobile` — no `*.apk` tracked (`.gitignore` `*.apk`); `android/` committed; local `build-1787328543751.apk` (127 MB) present on disk — not committed.

---

*Generated from direct file reads. No assumptions — all paths above are verbatim from the repo.*
