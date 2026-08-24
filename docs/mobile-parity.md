# Mobile → Desktop Parity — GPUI Migration Plan

> Goal: desktop (`apps/desktop` — now GPUI Rust, `src/main.rs:1`) reaches **feature parity with mobile** (`apps/mobile`) this week so it is dog-foodable daily. Mobile shipped ~80 commits Aug 20-23 and now leads. This doc is the checklist to close the gap.

Source period: `git log --since=2026-08-15` minus last desktop inline commit `04bb4e5 refactor(desktop): replace Electron with GPUI Rust app and inline assets`. All paths relative to repo root.

---

## 1. Why mobile is ahead

Last week mobile landed, in order:

* **Env / Connectivity** — `d5b23b2 feat(mobile): multi-environment backend switching with home-screen switcher`, `a221990 feat: environment editor as full screen`, `fbc5576 feat: clean-install disconnect`, `922122a fix: switch without confirmation`, `88bf0b6 feat: open editor inline in bottom sheet`, `ece702e feat: add files tab`, `763d016 feat: Files screen` — docs `docs/environments-feature.md:1`.
* **Auth** — `8537685 feat: implement codebuff device-code login flow`, `hooks/useLocalOAuthLogin.ts:1` (Android localhost server `modules/local-auth-server/` Kotlin `LocalAuthServerModule.kt`), `stores/useAuthStore.ts:84` polling.
* **Chat / Composer** — `85d2d4a feat: add composer autocomplete`, `7b9e558 feat: wire autocomplete`, `0a899ae fix: file type icons`, `4f075e5 refactor: split composer into input + attachment`.
* **Files / FS** — `3d981aa feat(server): recursive fs entries`, `763d016 feat: Files screen`, `c58fad4 feat: FileTreeBrowser`, `21235b6 feat(api): assist service` for slash/file search.
* **Terminal** — `918ff8b feat: port t3-terminal as console-terminal`, `4a72921 feat: implement full mobile terminal`, `90fb385 refactor: split terminal screen into hook + components`, `f2a743a fix: scope terminal to explicit project` — docs `docs/terminal-mobile-plan.md:1`.
* **Polish** — theme generation `1b05d47`, icon registries `a31db83`, syntax highlighting `75d5683`, drafts `4c336f1`, etc.

Desktop was Electron until `04bb4e5` (Aug 24) and now re-hosts `gpui-ui/console-rs` with a single `assets/` (`apps/desktop/src/assets.rs:1`) and no mobile-equivalent features ported.

---

## 2. Mobile feature inventory (what desktop must match)

Grouped; each bullet has canonical file(s).

### 2.1 Connectivity — multi-environment backend switching

* **Environments store** `apps/mobile/stores/useEnvironmentsStore.ts:15` — `environments[] {id: env_XXXX, name, url}`, `activeId`, `probes: Record<id,{ok,checkedAt}>`, MMKV keys `@console_environments` + legacy `@console_backend_url` (migration), `add/update/remove/activate/deactivate/probeEnvironment` (6s `fetch /api/projects`), `resetServerState()` on switch.
* **Switcher sheet** `apps/mobile/components/environments/environment-switcher.tsx:22` — home-header `Server` button → `SharedBottomSheet` 50%/80% with status dots, `probe` on open, inline create `EnvironmentEditor` inside sheet.
* **Editor** `apps/mobile/components/environments/environment-editor.tsx` — single component `mode=create|edit`, states `idle/testing/test-ok/test-fail/saving`, URL dedupe, `normalizeBackendUrl` (`utils/url.ts`), requires passing test before save.
* **Connection settings** `apps/mobile/screens/settings/environments-settings.tsx` — list + `+ Add`, disconnect (wipe all envs → clean install).
* **Onboarding** `apps/mobile/index.tsx` + `hooks/useServerConnection.ts:1` — shown when `backendUrl==null`, `saveConnection`/`testConnection` (6s timeout against `/health`→ now `/api/projects`), syncs `configureConsoleApi({baseUrl})` + legacy key + `useAppStore.backendUrl`.
* **Persistence helper** `utils/server-state.ts:1` `resetServerState()` = `clearChatCache()` + `queryClient.clear()`.

### 2.2 Auth — account settings, OAuth, device-code, local server

* **Account screen** `apps/mobile/screens/settings/account-settings.tsx:25` — provider list `catalog.providers.filter(authMethod!=="none")`, row status `Check/Circle` + email, actions `Login / Re-login / Pair` (device-code → `auth.loginCodebuff()`), `LoginWithLocalServer` branch, Gemini `projectId` input + Save.
* **Auth store** `apps/mobile/stores/useAuthStore.ts:1` — `status: AuthStatusResponse`, `projectIds {gemini, antigravity}`, `loadStatus` → `authService.getAuthStatus()`, `loginWithBrowser` (`getLoginUrl` → `openURL`), `loginCodebuff` (poll every 2s until `completed` or `expiresAt`), `saveProjectId` POST `/api/auth/project-id`.
* **Local-auth-server** `apps/mobile/hooks/useLocalOAuthLogin.ts:1` + `modules/local-auth-server/` — Android-only localhost HTTP server catching `http://127.0.0.1:PORT/callback?code&state` (`LocalAuthServerModule.kt`), 2-min timeout, listeners `onAuthCallback/onAuthComplete`, then `handleCallback` + `loadStatus`; availability check `isLocalAuthServerAvailable()`.
* **Deep-link OAuth** `hooks/useAuth.ts` `useOAuthDeepLink()` — `Linking.addEventListener("url")` for `hostname==="auth"` → `handleCallback`.
* **Provider catalog** `hooks/useProviderCatalog.ts` / `stores/useProviderStore.ts`.

### 2.3 Chat / Composer — streaming, decisions, composer UX

* **Chat screen** `apps/mobile/screens/chat/chat-screen.tsx:1` — `useChatStream` + `useAbort`, header `SquareTerminal`/`Folder` shortcuts (cwd→project), `FlashList` `ChatMessageList`, `InteractionPanel` vs `Composer`.
* **Composer** `components/chat/composer.tsx` + `composer-input.tsx` + `composer-bottom-strip.tsx` — `KeyboardStickyView`, `ComposerAutocomplete`, `AttachmentStrip`, `Project/Model/Approval` pickers (project locked if messages exist), `ImagePicker.launchImageLibraryAsync` (multiple, base64 0.85, cap `MAX_DRAFT_IMAGES=2`).
* **Autocomplete** `components/chat/composer-autocomplete.tsx:35` — triggers `/` at line-start `^[\w:-]*` and `@` `[\w./-]*`, slash via `assistService.listSlashCommands(sessionId)` once, file via `assistService.searchFiles(sessionId, query)` (20 cap, seq-guarded), splice `${before}/name  ${after}`.
* **Message rendering** `chat-message-list.tsx`, `message-bubbles.tsx`, `run-activity.tsx`, `tool-call-block.tsx`, `markdown-renderer.tsx`, `syntax-highlighter.tsx` (Prism, 18 aliases), `diff-view.tsx` (`diff@7`, `computeLineDiff`).
* **Interactions** `interaction-panel.tsx` / `question-panel.tsx` / `approval-panel.tsx` — permission `Allow/Deny` (upgrade flag), multi-type question wizard.
* **Chat store** `stores/useChatStore.ts` — persisted `console-chat-cache` (MMKV) per-session `ChatSessionState {messages,input,attachments,draftUpdatedAt,streamingText,activeToolCalls,pendingQuestions/Permissions,runs}`, `sendMessage(sessionId,prompt)` validates `supportsImages`, `startNativeChatStream` POST `${baseUrl}/api/sessions/${id}/run`, coalesced `requestAnimationFrame`, `answerQuestion/approvePermission`, `abort`, `clearChatCache`.
* **Native stream** `modules/native-stream/` — `startNativeChatStream` + `startNativeNotificationStream` (WS/SSE) with XHR fallback; `utils/chat-events.ts`, `utils/reconstruct-runs.ts`.

### 2.4 Sessions / Projects / Home

* **Home** `screens/home/home-screen.tsx` + `hooks/useHomeSessions.ts` — `SessionList` grouped by project/`cwd→folderName` sorted by `latestAt`, drafts pinned (`selectDraftSummaries`, `createEphemeralDraftHeader`), search `SearchBar`, `SessionActionSheet` (rename/delete/restore), pull-refresh, prefetch top 5.
* **Projects** `stores/useProjectStore.ts`, `hooks/queries.ts:1` `useProjects/useSessions/useInfiniteSession` (TanStack 15s stale, focus refetch), `screens/projects/add-project-screen.tsx` (fs browse, breadcrumb, `Add Folder`).
* **Session view** `stores/useSessionStore.ts` — `sessionModelId/sessionProvider/sessionCwd/approvalMode`, `changeModel`/`changeProject` (guard: no change if messages exist), `SessionStatusStore` (`idle/working/needs_attention`).
* **Drafts** `stores/chat/draft.ts` — `draftPreview`, `trimDraftAttachments`, persisted via `chat-persist.ts`.

### 2.5 Files — explorer, tree, search, git

* **Files screen** `screens/files/files-screen.tsx:27` — project-scoped `useFsEntries(projectRoot,6)` + `useReadFile(selectedFilePath)`, empty-state when no project, selectable mono preview, search + `FileTreeBrowser`.
* **Tree** `components/files/FileTreeBrowser.tsx` — `utils/fileTree.ts` `buildFileTree/defaultExpandedTreePaths/flattenFileTree`, `FlashList` with `Chevron`+`Folder/FileIcon`, `gitStatus` badge, optimistic 1s, ancestors auto-expand, search filter, pull-refresh.
* **Fs / Git hooks** `useProjectFsWatcher.ts` (5s poll `browseDirectory`), `useGit.ts` (`gitService.getStatus`), `useProjectBranches.ts` (60s cache).
* **Server/API** `packages/api/src/services/fs.service.ts` (`getFsBrowse/getFsTree/getFsEntries/readFile/...`), `git.service.ts` (`/api/git/status`), `assist.service.ts` (`/api/assist/:id/search`).

### 2.6 Terminal — native PTY (Android-only)

* **Native module** `modules/console-terminal/` — `ConsoleTerminalView.kt` (Canvas + `libghostty-vt.so` 4 ABIs), `GhosttyBridge` JNI `console_terminal_jni.cpp` (13 symbols), `jniLibs/{arm64-v8a,armeabi-v7a,x86,x86_64}/libghostty-vt.so`, Meslo fonts; scripts `build-libghostty-android.sh`.
* **JS bridge** `modules/console-terminal/src/terminal-surface.tsx` (`ConsoleTerminalSurface` props `terminalKey/buffer/fontSize/isRunning/themeConfig`), `terminal-theme.ts` (`CONSOLE_TERMINAL_THEME`), `terminal-surface` fallback (`ScrollView+TextInput`).
* **Store** `stores/useTerminalStore.ts` — `terminals: Record<id,TerminalRecord>` (`spawning/running/exited/error`), `buffers: Record<id,string>` (memory-only, 80ms coalesce), `openTerminal({projectId,cwd,cols,rows})` via `terminalService.connectTerminal({baseUrl,params,onEvent,onClose})` WS `/api/terminals` (`spawned/output/exit/error`), `write/resize/kill/findLiveTerminal/subscribe`.
* **Screen** `screens/terminal/terminal-screen.tsx:19` + `hooks/useTerminalScreen.ts` — project picker before spawn, `findLiveTerminal` replay, `BackHandler` → previousTab, 100ms resize debounce, `ExtraKeysBar` (`Esc \u001B`, `Tab \t`, `Ctrl-C \u0003`, arrows `\u001B[A-D]`), `KeyboardAvoidingView` padding, `RestartShellBar`.

### 2.7 UI / Theming / Navigation / Images / Polish

* **Navigation** `components/layout/main-content.tsx` — tab router `useAppStore.activeTab` (`home|chat|settings|terminal|files`), `app-shell.tsx`, `screen-header.tsx`.
* **Theming** `styles/theme.ts` auto-generated from `global.css` tokens, `uniwind/tailwindcss 4`, `lucide-react-native + hugeicons`, `react-native-svg`, `expo-blur`, `reanimated`, `gesture-handler`.
* **Images** `expo-image-picker` base64 flow, `attachment-strip.tsx` thumbnail grid, `image-preview-modal.tsx` pinch/zoom, `supportsImages` guard, copy via `expo-clipboard`.
* **Markdown / Diff** `syntax-highlighter.tsx` (Prism static imports 18 langs), `markdown-renderer.tsx` (`react-native-markdown-display` selectable, `Linking.openURL`), `diff-view.tsx` + `utils/diff.ts` (`diff@7`, collapsed 60 lines).
* **Notifications** `hooks/useNotificationStream.ts` → `startNativeNotificationStream(${backendUrl}/api/notifications/stream)`.
* **Storage** `utils/storage.ts` (MMKV), `utils/app-focus-manager.ts`, `query-client.ts` TanStack.

---

## 3. Desktop today (GPUI `apps/desktop`)

What exists in `apps/desktop` after `04bb4e5`:

* **Window / Layout** `src/main.rs:20` `gpui_platform::application().with_assets(Assets)` + `src/assets.rs:1`, `src/view.rs` `ConsoleDesktopApp:Render` with `TitleBar`, `SidebarView`, `WorkspacePane` (node tree, splits, drag `WorkspaceDrag`, `EmptyChatState`), `src/state/layout.rs` sidebar resize, `persistence/window.rs` frame persist.
* **Sessions / Projects / Models** `src/state/app.rs:30` single `ConsoleDesktopApp` entity, `sessions: Rc<Vec<SessionHeader>>`, `projects`, `providers`, `models_by_provider` (lazy `load_models_for_provider`), `favorites HashSet`, `branches GitBranchInfo`, `selected_model/approval_mode`.
* **Chat streaming** `src/state/run.rs` SSE via `reqwest` + `eventsource-stream`, `transcript_view` + `composer_input` per pane (`workspace_pane_states`), `attachments: HashMap<paneId,Rc<Vec<ImageAttachment>>>`, `pending_permissions/questions`, `todo_items`, `agent_notices`, `running_sessions: HashMap<sessionId,startedAt>`.
* **Composer** `crates/console-ui/src/common/composer_view.rs` — `ComposerView` `ComposerInput` + `Attachment` paste, `autocomplete` (`crates/console-ui/src/common/autocomplete.rs`) with same `/` + `@` triggers wired through pane state, `model_picker`/`approval_selector`/`workspace_footer` (project/branch pickers, branch loaded/pending).
* **File helpers (not a Files tab)** — `crates/console-ui/src/common/file_search.rs`, `crates/console-ui/src/layout/sidebar_view.rs` session list with date groups `session_groups.rs`, but **no dedicated File Tree / Files screen**, no `useFsEntries` tree browser, no `readFile` preview.
* **No environment switching** — `console_core/src/client.rs` `ConsoleClient` holds single `baseUrl`; `src/state/mod.rs` has no `EnvironmentsStore`, no MMKV, no `probe`, no onboarding sheet.
* **No Account/Auth UI** — no `account-settings`, no device-code, no `local-auth-server`; `console_core` has `ProviderCatalogEntry` but no login flow.
* **No Terminal** — no PTY, no `console-terminal`, no `@console/mobile-terminal-native`, no `useTerminalStore`.
* **No dedicated notifications stream** — chat SSE only.

Roughly: desktop has solid **chat + workspace shell** parity, but **missing the last 3 weeks of mobile product surface** (env/account/terminal/files).

---

## 4. Parity matrix

| # | Feature | Mobile | Desktop GPUI | Gap |
|---|:---:|:---:|:---:|---|
| 1 | **Multi-environment backend switcher** (list, status dots, probes, onboarding, clean disconnect) | ✅ sheets + MMKV + `probeEnvironment` | ❌ single URL | **P0** |
| 2 | **Environment Editor** (create/edit, test connection, dedupe, inline sheet) | ✅ `environment-editor.tsx` | ❌ | **P0** |
| 3 | **Connection probe & reset semantics** (`resetServerState` = clear chat cache + queryClient + reload auth) | ✅ | ❌ (desktop clears none on switch) | **P0** |
| 4 | **Account — device-code (codebuff) login** (open browser → poll 2s → `loadStatus`) | ✅ | ❌ | **P0** |
| 5 | **Account — OAuth with localhost server** (Android `LocalAuthServer`, port parse, 2-min timeout) | ✅ Android-only | ❌ (desktop can use loopback `127.0.0.1` more reliably than mobile) | **P0** |
| 6 | **Account — Gemini projectId + per-provider projectIds save** | ✅ `POST /api/auth/project-id` | ❌ | **P1** |
| 7 | **Provider catalog UI** (displayName, authMethod filter, login/pair buttons) | ✅ | ◐ (desktop has model dropdown only) | **P0** |
| 8 | **Terminal — native PTY parity** (ghostty or alt, live replay, extra keys) | ✅ Android ghostty | ❌ | **P0** |
| 9 | **Files — FileTreeBrowser** (build/flatten/expand, search, git badge, 6-depth entries) | ✅ `files-screen.tsx` | ❌ (only inline file search in composer) | **P1** |
| 10 | **Files — readFile preview + file watcher** (5s poll mobile; desktop could `watch_directory`) | ✅ | ❌ | **P1** |
| 11 | **Composer autocomplete** `slash` + `file @` (assistService, 20 cap, seq guard) | ✅ | ◐ desktop has `autocomplete.rs` but verify `assistService` + staging/pick behavior parity | **P1** |
| 12 | **Image attachments** (picker, strip, `MAX_DRAFT_IMAGES=2`, supportsImages guard) | ✅ `expo-image-picker` | ✅ desktop has `attachments.rs` + base64, check 2-image cap + preview modal parity | **P1** |
| 13 | **Drafts** (persisted per-session, `Drafts` pinned section, `draftPreview`) | ✅ MMKV 2-cap | ◐ desktop has composer history but no `Drafts` pinned synthetic headers | **P1** |
| 14 | **Session management** (create/update/delete/restore/permanentDelete, refresh header, search, group by project) | ✅ `useHomeSessions` | ✅ desktop has `sessions.rs/projects.rs` + sidebar groups, verify soft-delete/restore + search parity | **P1** |
| 15 | **Syntax highlighting / diff** (Prism 18 langs, diff collapsed 60) | ✅ | ✅ desktop has `markdown/highlight.rs`, `chat/diff_view.rs` — audit token colors | **P2** |
| 16 | **Notifications / realtime** (`/api/notifications/stream` SSE) | ✅ | ❌ | **P2** |
| 17 | **Theming / icons** (global.css → theme.ts, provider/file-type icons, hugeicons/lucide) | ✅ | ✅ desktop has `theme/mod.rs`, `primitives/icons.rs` (191 icons) — audit parity | **P2** |
| 18 | **Onboarding flow** (name+URL+Test Connection when backendUrl==null) | ✅ | ❌ | **P0** |

---

## 5. Phased plan — ship desktop this week

### P0 — Must have to dogfood (Mon–Wed)

**P0-1 Env switcher**
* Store `apps/desktop/src/state/environments.rs` (port `useEnvironmentsStore.ts:15`) — `environments`, `activeId`, `probes`, `activeUrlOf`, `newId`, persist via `persistence/store.rs` (follow `persistence/layout.rs` pattern) instead of MMKV; keys `@console_environments` JSON + legacy `@console_backend_url`. Functions `add/update/remove/activate/deactivate/probe` (probe = `reqwest::get(format!("{url}/api/projects")).timeout(6s)`). On URL change: call shared `resetServerState` (see P0-3).
* UI `crates/console-ui/src/common/environment_switcher.rs` + `environment_editor.rs` — port `environment-switcher.tsx:22` bottom-sheet → desktop `popover`/`menu` (use `crates/console-ui/src/primitives/menu.rs` + `tooltip.rs`); status dot `ok==None gray / true emerald / false red`; inline create state `creating bool`. Editor modes `create|edit` with `Test connection` button (spinner), dedupe same normalized URL (reuse `console_core/src/utils/diff.rs` url normalize or new `utils/url.rs`). Save requires `probe ok`.
* Wire into `src/view.rs` header: replace static title/Server icon — `EnvironmentSwitcher` where mobile had `Server` button in `home-screen.tsx` header. `TitleBar` already has traffic lights; add right-action area.
* Health endpoint: align with mobile `GET /api/projects` (mobile also had `/health`→`/api/projects` change) — keep 6s Abort.

**P0-2 Onboarding**
* `crates/console-ui/src/common/onboarding.rs` — when `environments.is_empty() || activeId.is_none()` show full-screen centric form (name + URL) + `Test connection` / `Connect`, same as `apps/mobile/index.tsx` `OnboardingScreen`. Desktop can show as modal over empty pane.

**P0-3 Reset semantics**
* Extract `resetServerState` in `src/state/server_state.rs` (mobile `utils/server-state.ts:1`): clear `sessions` cache substitute on desktop = clear `sessions` header? + clear any TanStack equivalent; on desktop clear `pending_*`, `running_sessions`, `transcript_scroll_positions` + notify; reuse on env switch / deactivate.

**P0-4 Account — auth parity**
* `crates/console-ui/src/common/account_settings.rs` — port `account-settings.tsx:25` GlassSurface list: provider rows `authMethod !== "none"`, status `Check/Circle`, email, `Login/Pair/Re-login` buttons, Gemini projectId input + Save (`POST /api/auth/project-id` via `console_core::services/provider.rs` / new `auth.rs`).
* Store `src/state/auth.rs` — port `useAuthStore.ts:1` (`status`, `projectIds`, `loadStatus`, `loginWithBrowser` (poll), `loginCodebuff` (GET `startCodebuffLogin` → open browser → poll 2s until `completed`), `saveProjectId`). Use `open::that` or `webbrowser` crate on desktop instead of `expo-linking`.
* **Loopback OAuth** — desktop is easier than mobile: reuse server's `GET /api/auth/login/url` `redirectUri` port, spawn `tiny_http`/`axum` one-shot localhost server on that port (`startAuthServer` equivalent in Rust), listen `onAuthCallback` → `handleCallback` → `loadStatus`. No Android JNI needed. Port mobile timeout 2 min (`OAUTH_TIMEOUT_MS`). Add native dep `tiny_http` or `tokio::net::TcpListener`.

**P0-5 Terminal — minimal shippable**
* Mobile uses ghostty + WS `/api/terminals`. For desktop, **do not re-port ghostty JNI this week** — instead:
  * **V1 (week)**: reuse `apps/server` PTY WS `terminal.service.ts:1` endpoint with a Rust `xterm-ghostty` alternative: embed `alacritty_terminal` or `vt100` parse + `gpui` canvas, or **fast path**: shell out to system terminal view using `gpui::canvas` + `portable-pty` + `vt100` and `crates/console-ui/src/terminal/` new module. Simpler: reuse `console_core::services/terminal` client already for mobile WS and render in a `TerminalView` `Entity`.
  * Data flow: same as mobile `terminal-mobile-plan.md:27` ws frames → store buffer → view; start with single PTY per project (`findLiveTerminal` dedup), `buffers: HashMap<terminalId,String>` 80ms coalesce, `write/resize/kill`.
  * UI `crates/console-ui/src/terminal/terminal_view.rs` + `extra-keys-bar.rs` (desktop has real keyboard, but keep `Esc/Ctrl-C` bar for discoverability). Screen tab `terminal` next to `home|chat`.

### P1 — High value, days 3-5

**P1-1 Files tab**
* `crates/console-ui/src/files/file_tree_browser.rs` — port `FileTreeBrowser.tsx` `FlashList` → `gpui::List` (`ListState` already used for sidebar), `buildFileTree` logic port from `apps/mobile/utils/fileTree.ts` into `crates/console-ui/src/utils/file_tree.rs`, icons via `primitives/icons.rs:219` `FileTypeIcon`, git badge via existing `console_core::services/git.rs` `getStatus`. Data: `fs.service.getFsEntries(projectRoot, 6)` + `readFile`.
* `src/state/fs.rs` — `fsStore` equivalent.

**P1-2 Composer parity check**
* Audit `autocomplete.rs` vs `composer-autocomplete.tsx:35` seq guard + 20 cap; ensure `assistService` wired via `console_core::services/assist.rs` (already has `types/assist.rs` `SlashCommandInfo/FileSearchResult`).

**P1-3 Drafts / search / soft-delete**
* Add synthetic `Drafts` section + search `filter` like `useHomeSessions.ts` — desktop sidebar `sidebar_view.rs` already has `collapsed_groups` + `SessionDateGroup`, extend with `draftSummaries` pinned.

### P2 — Polish after dogfooding

* Notifications SSE, markdown copy/select parity (`markdown/selection.rs` already), pinch zoom for `image_viewer.rs`, `generate-theme.mjs` single-source check, MMKV→`persistence/store.rs` audit.

---

## 6. Detailed checklists

### Env switcher — definition of done

* [ ] `src/state/environments.rs` persisted to `persistence/store.rs` key `@console_environments` JSON + legacy sync.
* [ ] `probeEnvironment` is correct (`GET {url}/api/projects` 6s, ok→green, fail→red, never→gray), called on switcher open and editor save.
* [ ] `activateEnvironment` clears desktop caches (`pending_*`, `running_sessions`, `transcript_scroll_positions`) before `configureConsoleApi` swap and `loadStatus`.
* [ ] UI matches mobile: header `Server` icon → bottom-sheet/menu; list rows `name + urlHost + Check`; inline `+ Add environment` without leaving sheet.
* [ ] Onboarding modal when zero envs (mirrors mobile when `backendUrl==null`).
* [ ] `cargo check` and `cargo run` create → edit → switch → reconnect.

### Account — definition of done

* [ ] `account_settings.rs` lists only `authMethod!=="none"` providers, shows `loggedIn/email` + `Login/Pair/Re-login`.
* [ ] `loginCodebuff` opens system browser (`open::that`) then polls `pollCodebuffStatus` 2s until `completed` or `expiresAt` timeout.
* [ ] Loopback OAuth: parsed `redirectUri` port/callbackPath → one-shot `TcpListener` → `onAuthCallback` → `POST /api/auth/login/callback` → `loadStatus`.
* [ ] Gemini projectId persists via `POST /api/auth/project-id` and pre-fills input.

### Terminal — definition of done (V1)

* [ ] Spawns via `POST /api/terminals` or WS `connectTerminal` with `cwd` from selected project.
* [ ] Output buffer replay on tab return (coalesced; no per-token persist).
* [ ] `write(\r)` + arrows + `Ctrl-C \u0003` etc. work; `resize` on window resize (100ms debounce).
* [ ] `kill` → `exited` → `RestartShellBar`.

---

## 7. API contracts desktop must honor (mobile already does)

* `GET /api/projects` — probe health (6s).
* `GET /api/sessions` + `GET /api/sessions/:id?limit&before` + CRUD + `DELETE` soft-delete / `POST restore` (mobile uses `session.service.ts:1`).
* `GET /api/fs/entries?path&depth=6` (`packages/api/src/services/fs.service.ts`), `GET /api/fs/read?path`, `GET /api/git/status?path`.
* `GET /api/assist/:sessionId/commands`, `GET /api/assist/:sessionId/search?q=` (`assist.service.ts`).
* `WS /api/terminals?cwd&cols&rows&label` frames `spawned/output/exit/error` (`terminal.service.ts`).
* `GET /api/auth/status`, `GET /api/auth/login/url?provider`, `POST /api/auth/login/callback`, `POST /api/auth/project-id`, `POST /api/auth/codebuff/start` + `poll`.
* `GET /api/notifications/stream` SSE (P2).

All exist on `apps/server` (verified via `a429fdc feat(api): add fs entries client` up to `21235b6 feat(api): add assist`).

---

## 8. What not to port

* Mobile 5-second poll `useProjectFsWatcher.ts:1` — desktop should use native `notify::watch_directory` / `gpui` fs watcher instead.
* Android-only ghostty JNI, `expo-image-picker` base64 flow (desktop has real file picker + clipboard images) — keep desktop's `attachments.rs` paste logic.
* Mobile `BottomSheetTextInput` quirks (`7b493a3 fix: use BottomSheetTextInput`) — desktop uses regular `TextField` (`primitives/text_field.rs`).

---

## 9. Suggested ticket order (this week)

1. **EN V-1** `src/state/environments.rs` + `persistence` + `resetServerState`. (1 day)
2. **ENV-2** `environment_switcher.rs` + `environment_editor.rs` + titlebar wiring. (0.5 day)
3. **AUTH-1** `src/state/auth.rs` + `account_settings.rs` + codebuff poll. (0.5 day)
4. **AUTH-2** loopback server for OAuth (1 day — reuse existing redirect port).
5. **TERM-1** minimal terminal V1 (2 days — alternatives if ghostty port is heavy).
6. **FILES-1** `FileTreeBrowser` + `files-screen` tab (1 day).
7. **POLISH** composer/drafts/search audit + docs update (0.5 day).

> Tip: keep mobile's `docs/environments-feature.md` and `docs/terminal-mobile-plan.md` as the spec; desktop ports reuse the same state machines (`plan-mode-state-machine.md`) and `docs/diff.md` diff rendering.

---

*Generated `2026-08-24` from `git log --since 2026-08-15` and source audit. Next step: `cargo check` locally, then open P0 branches against `apps/desktop`.*
