# Android Native Kotlin Migration — Phased Plan

> **Scope:** Rebuild `apps/mobile` (Expo 57 / React Native 0.86) as **Android-only Native Kotlin + Jetpack Compose**. No KMP, no iOS.
> **Constraint:** Absolutely not a one-shot rewrite. Every phase ships a runnable APK and can coexist with the Expo app until cutover.

---

## 1. Executive Summary

The current mobile app is Expo + React Native. For Android-only, Native Kotlin is strictly superior: faster, smaller, no Metro/EAS, and — unlike KMP — it *does* have a world-class code-editor stack. The app is largely a **viewer** (Shiki/Prism → `ThemedToken[][]` → `VirtualizedCodeView`), not a full editor, and `sora-editor` covers that directly as a read-only viewer (`isEditable = false`) with full highlighting/line numbers intact. The hardest KMP gap is a non-issue here.

**Recommendation:** New Gradle project `apps/android` (or `apps/mobile-native`) living beside `apps/mobile`. Reuse `apps/mobile/android/` native sources where possible, especially `modules/console-terminal` (already Kotlin + `libghostty.so`). Migrate screen-by-screen behind a navigation flag.

---

## 2. Goals / Non-Goals

**Goals**
- Native Kotlin + Compose Material 3 app, Android-only, feature-parity with Expo app.
- Reuse backend (`apps/server` Hono), types (`packages/types`), and native terminal.
- Each phase ends with a shippable APK. No long-lived "big bang" branch.

**Non-Goals**
- iOS (if needed later, re-evaluate KMP then).
- Changing `apps/server` API contract (keep Hono/Zod as source of truth).
- Expo OTA (`expo-updates`) — replaced by Play Store delivery.

---

## 3. Current Mobile Inventory (what we're replacing)

```
apps/mobile/
  index.tsx, app.json, eas.json, query-client.ts
  components/  chat/*, changes/*, files/*, terminal/*, common/*, layout/*, icons/*
  screens/     home, chat, files, changes, terminal, subagents, settings/*, projects/*
  hooks/       useChatStream, useChatDecisions, useHomeSessions, useFileSearch,
               useServerConnection, useAuth, useLocalNotifications, etc. (20+ hooks)
  stores/      useAppStore, useChatStore, useSessionStore, useTerminalStore,
               useFsStore, useProjectStore, useProviderStore, useEnvironmentsStore,
               Legend State observables (@legendapp/state)
  services/    highlighter.ts (Shiki native+JS engine, vitesse-dark, 19 langs)
  utils/       chat-events, diff, reconstruct-runs, native-stream, nitro-fetch, storage, etc.
  modules/     console-terminal (Kotlin + JNI + libghostty), local-auth-server, native-stream
```

**Key behaviors to preserve:** tab shell (`MainContent` 8 tabs), Legend State narrow subscriptions via `useValue(app$.field)`, TanStack Query (`staleTime 15s`, `refetchOnWindowFocus`), onboarding `backendUrl` flow, streaming chat, file tree + highlighted preview, diff/changes, Ghostty terminal.

---

## 4. Package Mapping — everything you need exists

### 4.1 Direct 1:1

| Current (npm / Expo) | Native Kotlin | Notes |
|---|---|---|
| `react-native`, `expo`, `expo-constants`, `expo-status-bar`, `expo-font` | **Jetpack Compose BOM `2024.10+` + Material 3 + `FontFamily(res/font/jetbrains_mono)`** | Compose 1.7+ |
| Navigation (custom `AppShell` + `MainContent`) | **Navigation Compose 2.8+** + `navigation-compose` + bottom bar | Single-Activity, type-safe routes (`kotlinx.serialization`) |
| `@tanstack/react-query` + `nitro-fetch` + `native-stream` (SSE) | **Ktor Client 3.x (`ktor-client-okhttp` + `ktor-client-content-negotiation` + `kotlinx.serialization-json`) + `kotlinx.coroutines` + `Flow`** | SSE via `ktor-client-sse` or raw `Flow` over OkHttp. Same `common` feel as RN. |
| `@console/api` (Axios + `adapter: fetch`) | **Ktor** + `kotlinx.serialization` + `kotlinx-datetime`** | Generate from `packages/types` or hand-port. Interceptor for `Authorization: Bearer` (same as `client.ts`). |
| `@legendapp/state` + `LegendList` | **`StateFlow`/`MutableStateFlow` + `ViewModel` + `LazyColumn` (keys)** | MVI-ish. No Legend equivalent needed; Compose recomposes narrowly. |
| `react-native-mmkv` + `utils/storage.ts` | **`androidx.datastore:datastore-preferences` + `EncryptedSharedPreferences` (for tokens)** | Matches MMKV perf; use `Multiplatform Settings` is KMP-only, not needed here. |
| `react-native-gesture-handler` + `reanimated 4.5` + `expo-blur` | **Compose `pointerInput` + `animate*AsState`/`AnimatedVisibility` + `RenderEffect` blur** | Simpler. |
| `react-native-safe-area-context` + `react-native-screens` | **`WindowInsets` + `enableEdgeToEdge()`** | Native. |
| `react-native-svg`, `lucide-react-native`, `@hugeicons/*` | **`ImageVector` + `com.google.android.filament` not needed; use `svg -> Compose` via `com.github.dev-icpac` or just `Painter`** | Keep `file-type-registry` mapping; render with `Icon` |
| `expo-clipboard`, `expo-image-picker` | **`ClipboardManager` + `ActivityResultContracts.PickVisualMedia`** | |
| `expo-notifications` | **`WorkManager` + `NotificationManager` + `POST_NOTIFICATIONS` permission** | Same as `useLocalNotifications` |
| `expo-linking` | **`intent-filter` scheme `console://` (already in `app.json`) -> `NavDeepLink`** | Preserve deep links |
| `diff` | **`java-diff-utils 4.12`** | Pure JVM |
| `tailwindcss`/`uniwind` + `styles/theme.ts` | **`MaterialTheme` + `CompositionLocal` + `ColorScheme` (light/dark)** | Port `vitesse-dark` tokens to `Color.kt` |
| DI if needed | **Hilt 2.51 (or Koin 4.x)** | Hilt preferred for Android-only |
| Local DB if needed | **Room 2.6+** | Not currently used; add only if caching grows |

### 4.2 Good alternatives

| Current | Native Kotlin |
|---|---|
| `react-native-markdown-display` | **`org.jetbrains.compose.markdown` or `com.halilibo.compose-markdown` or Markwon wrapped in `AndroidView`** — Compose M3 renderer. Mature. |
| `@gorhom/bottom-sheet` | **`ModalBottomSheet` (Material 3)** — loses Gorhom snap points, gains native behavior |
| `react-native-keyboard-controller` | **`WindowInsets.ime` + `bringIntoViewRequester()`** — actually simpler |
| `@shikijs/*` + `prismjs` + `react-native-shiki-engine` | **See §4.3 — not a blocker** |

### 4.3 Code viewer / editor (the "no package" concern)

Your app is a **viewer**, not an editor**:

- `services/highlighter.ts` → `ThemedToken[][]` (vitesse-dark, 19 langs)
- `components/files/VirtualizedCodeView.tsx` → `LegendList` + `Text selectable` + gutter + copy pill
- `components/common/syntax-highlighter.tsx` → `ScrollView` + Prism tokens

**For viewer (what you need now): `sora-editor` in read-only mode IS the viewer.** No custom `LazyColumn` + tokenizer needed as primary path.

- **`sora-editor` (`io.github.Rosemoe:sora-editor:0.21.1` + `sora-editor:language-textmate:0.21.1`) — read-only viewer:** Set `isEditable = false` (`codeEditor.setEditable(false)`) after creating `CodeEditor`. This disables input/selection handles/editing while keeping highlighting, line numbers, folding, and TextMate grammars active — exactly `VirtualizedCodeView` behavior. Used by AndroidIDE/CodeAssist. In Compose: `AndroidView(factory = { CodeEditor(it).apply { isEditable = false; setEditorLanguage(...) } })`. 6k stars, actively maintained.
  - Viewer tuning: hide caret (`cursorBlinkPeriod = 0` / disable caret drawing), don't wire long-press editing menus, keep `EditorLanguage` set so highlighting stays rich. Still supports `vitesse-dark` via TextMate theme.

**Fallback tokenizer-only path (if you want a pure Compose `LazyColumn` without sora):**

- **`Prism4j` (`io.noties:prism4j:2.0.0`)** — Java port of Prism.js you already use. Easiest: `Prism4j.getGrammar("kotlin")`.
- **`TextMate4J / TM4E`** — VS Code TextMate grammars (same Shiki uses) → best fidelity, reuse `vitesse-dark`.

> On JVM, Shiki's hack (`react-native-shiki-engine` JSI + Oniguruma C++) is unnecessary — tokenization is native and ~10× faster. `sora-editor` handles this internally via `language-textmate`.

### 4.4 Native modules you already own

| Module | Current | Native Kotlin |
|---|---|---|
| `modules/console-terminal` (ghostty) | Kotlin + JNI + `libghostty.so` + `TerminalSurface` view | **Reuse verbatim.** Copy `android/src/` + `build-libghostty-android.sh` + `.so`. Wrap as Compose `AndroidView`. Zero rewrite. |
| `modules/local-auth-server` | Tiny loopback HTTP for OAuth redirect (`12 KB` TS) | **`Ktor embeddedServer(Netty/CIO)` ~30 lines** or `NanoHTTPD` |
| `modules/native-stream` | SSE bridge | **Deleted — replaced by Ktor SSE `Flow`** |

---

## 5. Architecture & Tech Stack (Android-only)

**Language:** Kotlin 2.0+, Gradle Kotlin DSL, version catalog (`libs.versions.toml`)
**UI:** Compose BOM, Material 3, Navigation Compose, Accompanist is dead — use built-in `WindowInsets`
**Async:** `kotlinx.coroutines 1.8+`, `StateFlow`, `SharedFlow`, lifecycle-aware `collectAsStateWithLifecycle`
**Network:** `Ktor 3.x`, `kotlinx.serialization`, `OkHttp` engine, `kotlinx-datetime`
**Storage:** `DataStore Preferences`, `EncryptedSharedPreferences` (tokens), `Room` if needed
**DI:** `Hilt`
**Images:** `Coil 3` (if needed for chat/file previews)
**Markdown:** `Markwon` or Compose markdown
**Code viewer:** `sora-editor` read-only (`isEditable = false`) + `language-textmate` (primary); `Prism4j`/`TextMate4J` + `AnnotatedString` only as fallback if avoiding `AndroidView` per screen
**Build:** AGP 8.6+, `minSdk 24`, `targetSdk 35`, Kotlin DSL, R8

**Proposed structure** (new root, side-by-side):

```
apps/android/                      # or apps/mobile-native — pick one, document it
  app/
    src/main/java/com/console/mobile/
      ConsoleApp.kt / MainActivity.kt
      ui/
        theme/Theme.kt  Color.kt  Type.kt   # port of styles/theme.ts + vitesse-dark
        navigation/NavGraph.kt  Routes.kt
        components/  # design system: AppShell, ScreenHeader, GlassSurface, Skeletons, etc.
      feature/
        home/        # HomeScreen + SessionList + ViewModel
        chat/        # ChatScreen + MarkdownRenderer + ChatViewModel + stream runner
        files/       # FileTreeBrowser + CodeViewer + FilesViewModel
        changes/     # ChangesScreen
        terminal/    # ConsoleTerminalSurface (AndroidView wrapper)
        subagents/
        settings/    # account, environments, projects, usage
        onboarding/  # backendUrl input + testConnection
      data/
        api/ConsoleApi.kt  ConsoleApiClient.kt  interceptors
        repo/  SessionRepository, FileRepository, etc.
        local/ PreferencesDataStore, TokenStorage
      core/
        viewer/ SoraCodeViewer.kt  ViewerTheme.kt  # sora-editor read-only (isEditable=false) + language-textmate
        markdown/
        diff/
  terminal/  # reused module: ghostty .so + Kotlin JNI (from apps/mobile/modules/console-terminal)
  build-logic/ or version catalog
  gradle/libs.versions.toml
```

Keep `packages/types/src/*` as source of truth — either generate Kotlin models via `openapi` or hand-port once and keep in sync manually (small surface; server is Hono/Zod).

### 5.5 Reference Implementation — Remodex Android (`Stivy-01/remodex/android`)

Remodex is the closest public analogue to Console's target: a remote-agent client (Codex bridge) rebuilt as **Android-only Native Kotlin + Compose** alongside an Expo/RN history. Its `android/` folder (355 Kotlin files, production-shipped) validates the stack this plan proposes and gives us concrete patterns to steal.

**What to copy directly**

| Concern | Remodex pattern (`android/app/build.gradle.kts`, `MainActivity.kt`, `AppContainer.kt`) | Recommendation for Console |
|---|---|---|
| **Gradle / SDK** | `compileSdk 36`, `targetSdk 36`, `minSdk 26`, `Java 17`, `org.jetbrains.kotlin.plugin.compose` + `kotlin.plugin.serialization`, version catalog via `libs.versions.toml`-style BOM pin, `namespace com.remodex.mobile`, `usesCleartextTraffic=true` + `networkSecurityConfig` | **Adopt `minSdk 26` (was 24).** Remodex ships 26 and covers 99%+ of Console's audience while avoiding back-compat shims ghostty doesn't need. Keep `usesCleartextTraffic` + `network_security_config.xml` for `http://192.168…` (mirrors `app.json`). |
| **Compose BOM** | `androidx.compose:compose-bom:2026.02.01` — note comment: `mike penz markdown 0.39.x` needs Kotlin 2.2 / newer runtime (`Updater.init-impl` fix) | Pin BOM + Kotlin in lockstep; don't mix old BOM with new markdown. Document the constraint. |
| **Markdown** | `com.mikepenz:multiplatform-markdown-renderer:0.39.2` + `-m3` + `-code` — renders chat timeline markdown (thinking, tool calls, file links) with M3 theming | **Use exactly this** for chat (`TurnMarkdownBody` equivalent). Validated for the same content types Console needs (`markdown-renderer.tsx` replacement). |
| **DI** | No Hilt/Koin — `object AppContainer` singleton initialized in `RemodexApplication.onCreate()`, holds `SecureStore`, `SessionPersistence`, `CodexMessagePersistence`, `OkHttpClient`×2, `CodexRepository`/`CodexService`, etc. Injected via `CompositionLocal` (`LocalCodexRepository`) | **Prefer `AppContainer` over Hilt for v1.** Remodex proves 355 files ship cleanly without Hilt; manual container + `CompositionLocal` is simpler to migrate screen-by-screen and avoids Hilt's build-time cost. Revisit Hilt only if graph grows. |
| **Networking** | Two `OkHttpClient`s: `httpCallClient` (10/20/30s timeouts, `retryOnConnectionFailure false`) for RPC + `httpClient` (ping 30s, `readTimeout 0`, retry true) for WebSocket/bridge + `kotlinx-serialization-json 1.7.3`, `kotlinx-coroutines-android 1.9.0` | **Copy the two-client split.** Console's `nitro-fetch`/`native-stream` SSE maps to `httpClient` (infinite read) + `Flow`; REST calls use `httpCallClient`. Replace Axios/Ktor plan with OkHttp+serialization (Ktor client is fine but Remodex proves OkHttp alone is enough; pick one and stick to it). |
| **Security / Storage** | `androidx.security:security-crypto:1.1.0` + `org.bouncycastle:bcprov-jdk18on:1.78.1` + `Security.insertProviderAt(BouncyCastleProvider(), 1)` in `Application`, `SecureStore`/`PhoneIdentityStore`, `EncryptedSharedPreferences`-style `SessionPersistence` | Reuse for Console `backendUrl` + tokens (replace `react-native-mmkv`). Same threat model (pairing secrets). |
| **Terminal** | `com.termux.termux-app:terminal-view:0.118.0` + `terminal-emulator:0.118.0` + `com.hierynomus:sshj:0.39.0` | Console keeps **Ghostty** (`libghostty.so` + JNI) — no need for Termux view. But Remodex validates `terminal-view` as fallback and confirms `sshj` if Console ever adds SSH profiles. |
| **Navigation / App shell** | `androidx.navigation:navigation-compose:2.8.4`, `activity-compose:1.9.3`, `lifecycle-runtime-compose:2.8.7`, `lifecycle-viewmodel-compose:2.8.7`, single `MainActivity` (`singleTop`, `adjustResize`, `enableEdgeToEdge`), `RootScreen` + `CompositionLocalProvider` for prefs | Mirror: single-activity, `enableEdgeToEdge`, Navigation Compose typed routes, `RootScreen` equivalent to `MainContent` 8-tab shell. |
| **Prefs / Theme** | `ThemePreferences`/`LanguagePreferences`/`UserBubblePreferences` backed by `SharedPreferences` with `OnSharedPreferenceChangeListener` + `RemodexTheme(darkTheme)` | Same for Console (`OnboardingPreferences` → `Preferences DataStore` or `SharedPreferences`; theme = `vitesse-dark` port). Lifecycle-aware `collectAsStateWithLifecycle` matches `sdk 26+`. |
| **Notifications** | `RemodexLocalNotificationPresenter.ensureChannelCreated`, `AppForegroundTracker.register()`, `POST_NOTIFICATIONS` permission, `consumeLaunchToken` deep-link pattern | Copy channel + foreground tracker for `useLocalNotifications` replacement. |
| **QR / Camera** | `androidx.camera:camera-core/lifecycle/view 1.4.1` + `com.google.mlkit:barcode-scanning:17.3.0` | Reuse if Console needs QR pairing (mirrors `local-auth-server` OAuth QR). |
| **Images / Polish** | `coil-compose:3.0.0` + `coil-network-okhttp:3.0.0`, `com.valentinilk.shimmer:compose-shimmer:1.4.0`, `icons-lucide-android:2.2.1` | Matches Console's `lucide-react-native`/`@hugeicons` → lucide-android; shimmer for skeletons. |

**What NOT to copy**

- Beta/supabase/subscription/pet-companion/beta-engagement modules — Console has no equivalent; keeps the template lean.
- Multi-Mac scoping (`MacScopedSessionStore`, `MacScoped*`) — Remodex's 148-commit `ios-parent` parity effort for multi-device isolation is overkill for Console's single-server model; note it as prior art if Console ever needs it.
- `android/supabase/` folder and `beta/` engagement service — out of scope.

**How to use Remodex during migration**

1. Scaffold `apps/android` from Remodex `android/` shape (`settings.gradle.kts` with `FAIL_ON_PROJECT_REPOS` + `jitpack`, `gradle.properties` `nonTransitiveRClass`, `build.gradle.kts` `plugins { id("com.android.application") ... }`). Don't fork — cherry-pick `gradle/wrapper`, `AndroidManifest.xml` (`usesCleartextTraffic`, `launchMode singleTop`), `RemodexApplication` → `ConsoleApplication` structure.
2. Copy `AppContainer.kt` pattern verbatim (object + `initialize(context)`) and adapt fields to `ConsoleApi`, `SessionPersistence`, `MessagePersistence`.
3. Reference `ui/turn/*` (40+ files: `TurnComposerBar`, `TurnTimeline*`, `TurnMarkdownBody`, `TurnThinkingTimelineRow`) when implementing Phase 3 chat — same decomposition Console needs.
4. Keep Remodex linked as “prior art” in the plan so reviewers can cross-check decisions.

Source: [`Stivy-01/remodex/android`](https://github.com/Stivy-01/remodex/tree/main/android) — `app/build.gradle.kts`, `app/src/main/kotlin/com/remodex/mobile/{RemodexApplication,MainActivity,AppContainer}.kt`, `app/src/main/AndroidManifest.xml`.

---

## 6. Phased Delivery — overview

| Phase | Name | Goal | Ships | Depends on |
|---|---|---|---|---|
| **0** | Scaffolding & Foundations | Runnable empty app + CI | APK with nav + theme + placeholder tabs | — |
| **1** | Core Infrastructure | Networking + Storage + Auth + Navigation | API calls working, onboarding gated | 0 |
| **2** | App Shell + Home + Settings | First real screens, read-only | Users can connect, browse sessions | 1 |
| **3** | Chat (streaming) | Hardest product surface | Full chat with markdown + tool calls | 2 |
| **4** | Files + Code Viewer + Changes | File browser + highlight + diff | Code viewing parity | 2 (parallel with 3) |
| **5** | Terminal | Reuse ghostty module | Terminal parity | 1 |
| **6** | Polish + Notifications + OAuth + Release | Deep links, image picker, push, Play Store | Production cutover | 3,4,5 |

> Run Phases 3 and 4 in parallel after Phase 2. Phase 5 can start right after Phase 1 (no UI dependency beyond shell).

---

## 7. Phase 0 — Scaffolding & Foundations

**Goal:** `apps/android` builds, runs, has theme + navigation, and does not break `apps/mobile`.

### Tasks

- [ ] **0.1** Create project `apps/android` with Android Studio (Kotlin DSL, version catalog, Compose BOM, Hilt, Navigation Compose). `minSdk 24`, `targetSdk 35`, `compileSdk 35`.
- [ ] **0.2** Port `styles/theme.ts` → `ui/theme/Theme.kt` + `Color.kt`. Map `vitesse-dark` + tailwind tokens to `ColorScheme`. Keep `theme.ts` as spec.
- [ ] **0.3** App shell: `MainActivity` + `NavGraph` with 8 destinations (`home`, `chat`, `terminal`, `files`, `changes`, `subagents`, `subagent-details`, `settings`, `onboarding`). Bottom bar or `AppShell` equivalent.
- [ ] **0.4** Placeholder screens (empty `Scaffold` + `Text("Home")` etc.) so nav is testable.
- [ ] **0.5** CI: `Makefile` target `build:android:debug`, GitHub Action `./gradlew assembleDebug`. Do **not** remove `eas.json` / Expo CI until cutover.
- [ ] **0.6** Establish naming: confirm `apps/android` vs `apps/mobile-native` and document in README. Add `AGENTS.md` note.

**Verification:** `./gradlew :app:assembleDebug` succeeds; app launches on emulator; tabs switch; dark theme matches `theme.ts` screenshot; `apps/mobile` still builds (`bunx tsc --noEmit`).

---

## 8. Phase 1 — Core Infrastructure

**Goal:** The app can configure `backendUrl`, authenticate, and call `apps/server`.

### Tasks

- [ ] **1.1** **Networking:** Ktor client (`OkHttp` engine, `ContentNegotiation` JSON, auth interceptor). Port `packages/api/src/client.ts` (`configureConsoleApi`/`getConsoleApiClient`) → `data/api/ConsoleApiClient.kt`. Base URL from onboarding; `Authorization: Bearer <token>` if present.
- [ ] **1.2** **Serialization:** Port `packages/types/src/*` (`session.ts`, `events.ts`, `fs.ts`, `terminal.ts`, `usage.ts`, etc.) to Kotlin `@Serializable` data classes. Keep a `SYNC.md` mapping or generate.
- [ ] **1.3** **Storage:** `PreferencesDataStore` for `backendUrl`, `environments`, `model favorites`; `EncryptedSharedPreferences` for tokens. Replace `react-native-mmkv` + `utils/storage.ts`.
- [ ] **1.4** **Repositories:** `SessionRepository`, `ProjectRepository`, `FileRepository`, `ProviderRepository`, `UsageRepository` — thin wrappers over `ConsoleApi` returning `Flow`/`Result`.
- [ ] **1.5** **Onboarding:** Port `OnboardingScreen` (from `index.tsx`): `Name` + `backendUrl` inputs, `Test Connection`, `Connect`. Persist to DataStore, gate `NavGraph` on `backendUrl == null ? onboarding : home`.
- [ ] **1.6** **Query/cache layer:** Decide `Ktor + Flow` vs `TanStack`-like cache. Recommendation: `Flow` + `ViewModel` + `StateFlow` with `whileSubscribed(5_000)` — matches `staleTime 15s` behavior from `query-client.ts`. Add `AppFocusManager` equivalent via `Lifecycle.resume`.
- [ ] **1.7** **Error handling:** Global `ErrorBoundary` equivalent — `CoroutineExceptionHandler` + Snackbar + crash reporting (Firebase Crashlytics or Sentry Kotlin).

**Verification:** Enter `http://192.168.1.X:3000`, Test Connection succeeds, token stored; list sessions after wiring Phase 2 reads from real server; unit tests for `ConsoleApiClient` interceptors.

---

## 9. Phase 2 — App Shell + Home + Settings (first user value)

**Goal:** Users can connect and browse sessions. Read-only, low risk.

### Tasks

- [ ] **2.1** **App state:** Port `stores/useAppStore.ts` + `useSessionStore` + `useProjectStore` → `AppViewModel` / `SessionViewModel` with `StateFlow<MobileTab>` etc. Preserve `setActiveTab`, `openChatSession`, `clearAppSelections` semantics.
- [ ] **2.2** **Home:** Port `screens/home/home-screen.tsx` + `components/home/session-list.tsx` + `hooks/useHomeSessions.ts` → `feature/home/HomeScreen.kt` with `LazyColumn` (keys). Keep mount-once optimization from `MainContent` where needed.
- [ ] **2.3** **Project picker:** Port `components/terminal/project-picker.tsx` + `screens/projects/add-project-screen.tsx`.
- [ ] **2.4** **Settings:** Port `screens/settings/*` (account, environments, projects, deleted-chats, usage) + `components/environments/*`. Backed by `EnvironmentsStore` → `EnvironmentsViewModel`.
- [ ] **2.5** **Common components:** `confirm-dialog`, `search-bar`, `skeleton`, `empty-state`, `error-boundary`, `shared-bottom-sheet` → Compose `AlertDialog`, `SearchBar`, `ModalBottomSheet`, `Skeleton` placeholders.
- [ ] **2.6** **Icons:** Port `utils/icons/file-type-registry.ts` + `file-type-mapping.ts` → `core/icons/FileTypeRegistry.kt` + Compose `Icon`.

**Verification:** Home lists real sessions from server; pull-to-refresh works; Settings CRUD (environments/projects) persists; theme matches Expo app.

---

## 10. Phase 3 — Chat (most complex — allocate most time)

**Goal:** Streaming chat parity — messages, markdown, tool calls, slash commands, chat decisions.

### Tasks

- [ ] **3.1** **Stream plumbing:** Port `hooks/useChatStream.ts` + `stores/chat/*` (`chat-stream-runner`, `run-stream-controller`, `chat-persist`, `draft`) + `utils/chat-events.ts` + `utils/reconstruct-runs.ts` → `ChatStreamRunner` + `RunStreamController` using Ktor SSE `Flow<ChatEvent>`. Handle `part.thinking`, `reasoning-delta`, cancellation.
- [ ] **3.2** **State:** Port `stores/useChatStore.ts` (14 KB, largest store) → `ChatViewModel` (`StateFlow<ChatState>`). Preserve `Legend State` narrow subscription semantics via `collectAsStateWithLifecycle` per field.
- [ ] **3.3** **Messages UI:** Port `components/chat/**/*` (`messages/`, `tools/`, `banners/`, `composer/`, `selectors/`, `interactions/`) + `screens/chat/chat-screen.tsx`.
- [ ] **3.4** **Markdown:** Integrate Compose markdown renderer (`Markwon` or `compose-markdown`). Port `components/common/markdown-renderer.tsx` + clickable file paths plan (if merged). Preserve `linkify_bare_urls` behavior.
- [ ] **3.5** **Composer:** Port `components/chat/composer/*` — text input, slash commands (`hooks/useSlashCommands.ts`), image picker (`expo-image-picker` → `PickVisualMedia`), clipboard, keyboard insets (`WindowInsets.ime`).
- [ ] **3.6** **Decisions & steering:** Port `hooks/useChatDecisions.ts` + steering controls (if prompt-queueing spec active).
- [ ] **3.7** **Persistence:** Persist drafts + runs like `chat-persist.ts`.

**Verification:** Send message → see streaming tokens; thinking/reasoning blocks render; tool calls render; abort works; rotation doesn't lose stream; file-path links open Files tab.

---

## 11. Phase 4 — Files + Code Viewer + Changes (parallel with Phase 3)

**Goal:** File browsing, code viewing with highlighting, diffs.

### Tasks

- [ ] **4.1** **File browser:** Port `components/files/FileTreeBrowser.tsx` + `FileTreeRows.tsx` + `hooks/useFileSearch.ts` + `useProjectFsWatcher.ts` → `FileTreeBrowser` with `LazyColumn`. Use `FileRepository` (backed by `@console/api` `fs.service`).
- [ ] **4.2** **File reading:** Port `screens/files/files-screen.tsx` (preview via `useReadFile`) → `FilesViewModel` + `FilePreview`.
- [ ] **4.3** **Code viewer (primary — sora-editor read-only):** Port `components/files/VirtualizedCodeView.tsx` → `CodeViewer.kt` backed by `sora-editor`:
  - Dependency: `io.github.Rosemoe:sora-editor:0.21.1` + `language-textmate` (TextMate grammars, same family Shiki uses, so `vitesse-dark` ports directly).
  - Compose wrapper: `AndroidView(factory = { ctx -> CodeEditor(ctx).apply { isEditable = false } })`. `isEditable = false` disables input/selection handles/editing while keeping highlighting, line numbers, folding, and `EditorLanguage` active — exactly `VirtualizedCodeView`'s read-only contract.
  - Viewer tuning: hide caret (`cursorBlinkPeriod = 0` / disable caret drawing), don't wire long-press edit menus, keep `setEditorLanguage(...)` mapped via `LANGUAGE_ALIASES` (28 langs from `syntax-highlighter.tsx` + `getFileTypeLanguage` mapping). Theme: apply `vitesse-dark` TextMate theme.
  - Preserve existing UX on top: re-implement the floating pill (Copy/Clear) and `selectedLines` affordance. sora handles gutter/line numbers natively, so `gutterWidth` digit logic is no longer manual. Horizontal scroll is native.
  - Font: `JetBrainsMono` via `Typeface` / sora text style.
  - Keep `services/highlighter.ts` `vitesse-dark` + `TOKEN_STYLES` colors as reference for theme, but tokenization itself moves inside sora/TextMate — no `ThemedToken[][]` plumbing in Compose.
- [ ] **4.4** **Fallback / no-AndroidView path (only if you need pure Compose):** If a screen must avoid `AndroidView`, implement `core/highlighter/Tokenizer.kt` with `Prism4j` or `TextMate4J` → `tokenizeLines(code, lang): List<List<HighlightSegment>>` + `resolveLanguage(path)` and render via `LazyColumn` + `AnnotatedString`. This is a fallback, not the primary. The current app has no requirement for it.
- [ ] **4.5** **Markdown file preview:** Port `components/files/markdown-file-preview.tsx`.
- [ ] **4.6** **Changes:** Port `screens/changes/changes-screen.tsx` + `components/changes/ChangesRows.tsx` + `hooks/useChanges.ts` + `utils/diff.ts` + `utils/changes.ts` → `ChangesScreen` with diff view (`java-diff-utils`). For code blocks inside diffs, reuse the same sora read-only viewer (or inline `AnnotatedString` if lighter).
- [ ] **4.7** **Editing later (flip one flag):** To allow editing, just set `isEditable = true` and wire the editing handlers — no library swap.

**Verification:** Browse large repo; open 2k-line file in sora viewer — smooth scroll, `vitesse-dark` via TextMate theme matches Expo, line numbers built-in, read-only verified (no keyboard/input), long-press edit menu suppressed, floating Copy pill still works; diff code blocks highlight; file search works.

---

## 12. Phase 5 — Terminal (reuse — small but native)

**Goal:** Ghostty terminal parity, reusing the existing Kotlin module.

### Tasks

- [ ] **5.1** Copy `apps/mobile/modules/console-terminal/android/` + `scripts/build-libghostty-android.sh` + `.so` into `apps/android/terminal/`. Keep `THIRD_PARTY_NOTICES.md`.
- [ ] **5.2** Wrap native view: `ConsoleTerminalSurface` (`terminal-surface.tsx` 234 lines) → `TerminalScreen.kt` + `TerminalSurface` composable (`AndroidView(factory = { NativeTerminalSurfaceView(...) })`). Preserve props: `terminalKey`, `buffer`, `fontSize`, `isRunning`, `onInput`, `onResize` (`estimateGridSize` logic).
- [ ] **5.3** Port `hooks/useTerminal.ts` + `useTerminalScreen.ts` + `stores/useTerminalStore.ts` → `TerminalViewModel` (cols/rows, extra-keys bar, restart-shell bar).
- [ ] **5.4** Fallback surface: Keep `FallbackTerminalSurface` logic as Compose fallback when `.so` not loaded (e.g., emulator without NDK).
- [ ] **5.5** Extra keys / restart bar: Port `components/terminal/extra-keys-bar.tsx` + `restart-shell-bar.tsx`.

**Verification:** Terminal opens, resizes correctly on rotation, input works, `Ctrl-C` sends `\u0003`, `libghostty` renders; fallback still works if native view missing.

---

## 13. Phase 6 — Polish, System Integrations & Release

**Goal:** Production-ready APK, Play Store, feature parity on platform integrations.

### Tasks

- [ ] **6.1** **OAuth / deep links:** Port `modules/local-auth-server` → `Ktor embeddedServer` on loopback; handle `console://` `intent-filter` (`app.json` scheme) via `NavDeepLink`. Port `hooks/useAuth.ts` + `useLocalOAuthLogin.ts` + `useServerConnection.ts`.
- [ ] **6.2** **Local notifications:** Port `hooks/useLocalNotifications.ts` + `useNotificationStream.ts` → `WorkManager` + `NotificationManager` channels.
- [ ] **6.3** **Image picker / clipboard:** Wire `PickVisualMedia` + `ClipboardManager` in chat composer + code viewer.
- [ ] **6.4** **Subagents:** Port `screens/subagents/*` + `hooks/useSessionSubagents.ts` + `useSessionTodos.ts`.
- [ ] **6.5** **Usage:** Port `screens/settings/usage-settings.tsx` + `hooks/useUsage.ts` + `utils/usage-helpers.ts`.
- [ ] **6.6** **Git:** Port `hooks/useGit.ts` + `useProjectBranches.ts`.
- [ ] **6.7** **Cleartext traffic:** Preserve `plugins/withCleartextNetworkSecurity.js` + `usesCleartextTraffic` for `http://192.168...` (local server) via `network_security_config.xml`.
- [ ] **6.8** **Onboarding polish:** `QueryClient` `staleTime 15s` + `AppFocusManager` equivalent (`ProcessLifecycleOwner`).
- [ ] **6.9** **Release:** `signingConfig`, `bundleRelease`, Play Internal Testing, versioning (`versionCode` from `app.json` `1.0.0`). Drop EAS only after Play track is live. Keep `scripts/generate-theme.mjs` / `generate-svg-icons.mjs` or port to Gradle tasks.
- [ ] **6.10** **Coexistence removal:** After dogfooding, archive `apps/mobile` or keep as reference; update root `package.json` scripts (`dev:android` → `gradlew`).

**Verification:** OAuth login via `console://` works; notifications fire in background; image attach works; release APK installs; Play pre-launch report clean; Expo app still available but not required.

---

## 14. Cross-Cutting

### Testing (per phase)
- Unit: `JUnit 5` + `MockK` + `Turbine` (for `Flow`).
- UI: Compose `createComposeRule`, screenshot tests for `vitesse-dark` tokens.
- No `run-all-tests` — test the module touched by the phase.

### CI
- Keep Expo CI until cutover. Add `apps/android: assembleDebug` + `ktlint` + `detekt` jobs. Reuse `scripts/patch-fff-binary.mjs` if needed on server side.

### Coexistence
- Both apps point at same `apps/server`. Share `backendUrl` via same DataStore key or manual re-entry. No shared code between `apps/mobile` and `apps/android` initially — deliberate duplication keeps phases independent.

---

## 15. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| **Chat streaming is hardest** | Do Phase 2 first (read-only confidence), then Phase 3 with Ktor SSE spike before porting UI. Keep RN app as reference. |
| **Shiki/Prism parity → sora viewer** | No custom tokenizer spike needed as primary. Verify `sora-editor` `language-textmate` + `vitesse-dark` TextMate theme covers the 28 langs + `LANGUAGE_ALIASES` in a 1-day viewer spike in Phase 0. `Prism4j`/`TextMate4J` kept only as fallback if a screen must avoid `AndroidView`. |
| **Ghostty `.so` NDK drift** | Pin NDK version from `apps/mobile/android/gradle.properties`; reuse `build-libghostty-android.sh`. Test on arm64 emulator + physical device. |
| **Large session lists jank** | `LazyColumn` keys + `recycle` equivalent + `staleTime` caching. Profile with `Baseline Profiles`. |
| **Auth token migration** | Dual-write or re-login on first native launch; tokens are short-lived. |
| **One-shot temptation** | Enforce phase gates: each phase must produce an installable APK reviewed before next starts. |

---

## 16. Verification Checklist (cutover)

- [ ] Onboarding → Test Connection → Connect → Home lists sessions (real server).
- [ ] Chat: stream, thinking, tool calls, slash commands, image attach, abort, rotation.
- [ ] Files: tree, search, 2k-line file highlight (vitesse-dark), gutter, line copy, markdown preview.
- [ ] Changes: diff renders, file links jump to Files.
- [ ] Terminal: Ghostty renders, resize, input, Ctrl-C.
- [ ] Settings: environments/projects CRUD, account, usage.
- [ ] Subagents + todos.
- [ ] Deep link `console://` + OAuth loopback.
- [ ] Notifications in background.
- [ ] `cargo check` not relevant; `./gradlew :app:assembleRelease` + R8 + Play Internal Testing.

---

## 17. Effort Estimate (solo dev, part-time)

| Phase | Estimate | Notes |
|---|---|---|
| 0 Scaffolding | 1–2 days | Mostly project setup |
| 1 Core infra | 3–5 days | Networking + storage is mechanical |
| 2 Home + Settings | 1 week | Read-only, good momentum win |
| 3 Chat | 2–3 weeks | Largest; streaming + markdown |
| 4 Files/Code/Diff | 1–1.5 weeks | Can parallel with 3 |
| 5 Terminal | 2–3 days | Reuse — small |
| 6 Polish & Release | 1–2 weeks | OAuth, notifications, signing |

Total: ~7–10 weeks part-time, 4–5 weeks full-time. Parallelizing 3+4 saves ~1 week.

---

## 18. Decisions to Make Before Phase 0

1. **Project location:** `apps/android` (recommended) vs `apps/mobile-native` — pick and freeze.
2. **Viewer spike (Phase 0, 1 day):** `sora-editor` read-only (`isEditable = false`, `language-textmate`, `vitesse-dark`) — confirm 28 langs + `LANGUAGE_ALIASES` highlight correctly. No standalone tokenizer spike needed; `Prism4j`/`TextMate4J` only as pure-Compose fallback.
3. **Cache strategy:** `Flow` + `ViewModel` vs adding `Room` cache — start without Room, add if Home feels slow.
4. **DI:** `Hilt` (recommended) vs `Koin` — Hilt wins for Android-only.
5. **Min SDK:** 24 (recommended) vs 26 — 24 covers 99%+ and keeps ghostty compat.

---

*Next step: approve this plan, then start Phase 0 (scaffolding). Each phase will be its own branch/PR gated on the verification checks above.*
