# Plan: Native Terminal on Mobile (Android-first) via Ghostty

Port T3 Code's MIT-licensed `t3-terminal` Expo module (Android side) into Console's mobile
app, wired to our existing `/api/terminals` WebSocket stack. **Android-only**: strip iOS
(Swift views, podspec, GhosttyKit.xcframework). No xterm.js, no WebView.

Source reference: `t3code/apps/mobile/modules/t3-terminal/` (MIT, © T3 Tools Inc.)
and `t3code/apps/mobile/src/features/terminal/*`.

---

## Why this design

- The hard problems (VT/xterm-256color parsing, scrollback, reflow, selection, IME,
  hardware keys) are already solved in the vendored module using Ghostty's `libghostty-vt`
  (`libghostty-vt.so` prebuilt for all 4 Android ABIs, checked in — no Zig/NDK builds needed).
- Contract maps 1:1 onto our stack: native view emits `onInput {data}` / `onResize {cols,rows}`;
  our `connectTerminal()` (`packages/api/src/services/terminal.service.ts`) exposes
  `input()` / `resize()`. Our `useTerminalStore` (`apps/mobile/stores/useTerminalStore.ts`)
  already owns WS lifecycle, spawn/kill/status.
- Output stays authoritative in JS (a raw string buffer incl. ANSI escapes); the native view
  diffs the buffer prop (`initialBuffer`) and feeds only the suffix into libghostty — this
  gives free replay on remount/font-size change.

Data flow:

```
server PTY ──ws frames──▶ useTerminalStore (append raw buffer)
                                │ buffer prop
                                ▼
              T3TerminalView (libghostty-vt parse ──▶ Canvas snapshot)
                                │ onInput / onResize events
                                ▼
                useTerminalStore.write() / resize() ──ws──▶ server PTY
```

---

## Phase 0 — Copy plan into repo

Copy this file to `docs/terminal-mobile-plan.md` (user wants it in-repo).

## Phase 1 — Port the native module (Android-only)

Create `apps/mobile/modules/console-terminal/` by copying
`t3code/apps/mobile/modules/t3-terminal/` and deleting:

- `ios/` (T3TerminalView.swift, T3TerminalModule.swift)
- `Vendor/` (GhosttyKit.xcframework — iOS only)
- `scripts/` (iOS/Android rebuild scripts; optionally keep
  `build-libghostty-android.sh` under `scripts/` for future upgrades — needs
  ANDROID_NDK_HOME + Zig 0.15.2)
- `T3TerminalNative.podspec`

**Keep unchanged (minimal-diff rule):**

- Kotlin package `expo.modules.t3terminal` and native view name `Name("T3TerminalSurface")`.
  Renaming would require touching JNI symbol names in `t3_terminal_jni.cpp`; not worth it.
  JS references the same string, kept consistent below.
- `expo-module.config.json`: remove the `ios.modules` block, keep android
  `expo.modules.t3terminal.T3TerminalModule`.
- `android/` entirely: `build.gradle` (externalNativeBuild cmake 3.22.1, C++17),
  `src/main/cpp/{CMakeLists.txt,t3_terminal_jni.cpp}`,
  `src/main/java/expo/modules/t3terminal/*.kt` (module + view + canvas + frame + bridge),
  `src/main/jniLibs/{arm64-v8a,armeabi-v7a,x86,x86_64}/libghostty-vt.so`,
  `src/main/assets/fonts/MesloLGS-NF-{Regular,Bold}.ttf`.
- Update module `package.json` name to `@console/mobile-terminal-native` (private, 0.0.0).
- Copy `THIRD_PARTY_NOTICES.md` (preserve Ghostty + T3 attribution; MIT).

Register in `apps/mobile/package.json` dependencies:
`"@console/mobile-terminal-native": "file:./modules/console-terminal"`.
No JS export needed — JS uses `requireNativeView("T3TerminalSurface")` from `expo`.

Build requirements: NDK + CMake 3.22.1 via Android Studio SDK Manager (standard Expo setup).

## Phase 2 — JS bridge + theme (new files in apps/mobile)

New `apps/mobile/features/terminal/` directory:

1. `native-terminal-module.ts` — adapted from
   `t3code/.../src/features/terminal/nativeTerminalModule.ts`:
   - keep `resolveNativeTerminalSurfaceView()` + `hasNativeTerminalSurface()`
     (`requireNativeView("T3TerminalSurface")` behind a `getViewConfig` guard);
   - drop t3-specific bits (`NativeViewResolutionError`, hardware-key revision debug);
     console.error on resolution failure is enough.
2. `terminal-theme.ts` — small standalone rewrite (do NOT port t3's theme system):
   - `interface TerminalTheme { background; foreground; mutedForeground; border;
     cursorForeground; cursorBackground; palette }`
   - one hardcoded dark theme matching Console tokens from `apps/mobile/global.css`
     (`--color-screen #0a0a0b` background; foreground/muted/border from same block;
     cursor `#009fff`; 16-color palette borrowed from t3's Pierre dark palette);
   - `buildGhosttyThemeConfig(theme)` copied verbatim (ghostty config format:
     `background = ...`, `palette = N=#hex`, newline-joined).
3. `terminal-surface.tsx` — adapted from t3's `NativeTerminalSurface.tsx`:
   - `ConsoleTerminalSurface` renders the native view with props: `terminalKey`,
     `initialBuffer={buffer}`, `fontSize`, `themeConfig`,
     `backgroundColor/foreground/mutedForeground`, `appearanceScheme="dark"`,
     `autoFocus`, `focusRequest`; handlers map `onInput(e)` → `props.onInput(data)`,
     `onResize(e)` → `props.onResize({cols, rows})`;
   - keep the grid estimator (`estimateGridSize`) for the fallback;
   - keep `FallbackTerminalSurface` (ScrollView + transparent TextInput + Ctrl-C button)
     but restyle with uniwind classes + our theme object (drop t3's
     `useAppearancePreferences`/`AppText`/typography imports — dark-only app);
   - memoize exactly like t3 does.

## Phase 3 — Store: raw output buffer

Modify `apps/mobile/stores/useTerminalStore.ts` (additive):

- New state `buffers: Record<string, string>` (NOT persisted; memory only).
- In `openTerminal`'s `onEvent`, on `message.type === "output"` append `data` to
  `buffers[terminalId]` with a cap (~400k chars, trim head) before `emit`.
- On `end`/`kill`, delete `buffers[id]`.
- Screen reads it via `useTerminalStore(s => s.buffers[id] ?? "")`.

Rationale: the native view diffs `initialBuffer` against what it already fed
(`feedPendingBuffer` in `T3TerminalView.kt` feeds only the suffix; recreates on non-prefix
change), so appending raw ANSI output straight through is correct and gives replay.

## Phase 4 — Screen + entry point

1. `apps/mobile/screens/terminal/terminal-screen.tsx` following the chat-screen pattern
   (`ScreenHeader`, `BackHandler`):
   - Pick cwd: selected/default project path from `useProjectStore` (fallback sensible);
     pass through `openTerminal({ projectId, cwd })`.
   - On mount / when no terminal for the current project: `openTerminal(...)`.
   - Subscribe: `const unsub = useTerminalStore.subscribe(id, msg => {...})` in `useEffect`;
     status from `terminals[id].status` (spawning/running/exited/error banners).
   - Render `<ConsoleTerminalSurface>` filling flex-1 above an extra-keys row
     (Esc `\u001B`, Tab `\t`, Ctrl-C `\u0003`, arrows `\u001B[A|B|C|D`) sending bytes
     via `write(id, ...)`.
   - `onResize` → `resize(id, cols, rows)` (debounced ~100ms).
   - Leaving the screen does NOT kill the PTY (store keeps sink + buffer alive; remount
     replays buffer). Header trash icon → `kill(id)`.
   - Keyboard insets via `react-native-keyboard-controller` (already a dep); native view
     manages its own hidden IME input.
2. Register: `MobileTab` += `"terminal"` in `apps/mobile/stores/useAppStore.ts`;
   render conditionally in `apps/mobile/components/layout/main-content.tsx` like chat;
   export from `apps/mobile/screens/index.ts`; add a nav entry wherever tabs are switched
   (home header action or tab bar) — follow the existing pattern during implementation.

## Phase 5 — Verification

1. `npm run typecheck` (root).
2. Native build: `cd apps/mobile && npx expo run:android` on device/emulator — confirms
   autolinking of the `file:` module + cmake/jniLibs compile.
3. Bundle sanity per AGENTS.md: `cd apps/mobile && npx expo export --platform android`.
4. Manual: open Terminal tab → `ls`, `htop` (alternate screen), `vim`, Ctrl-C a long
   command, rotate device (resize), background/foreground app, leave + return to tab
   (buffer replay), kill from header.
5. Fallback path: temporarily force fallback component, confirm basic echo works.
6. Server tests untouched but confirm protocol intact:
   `cd apps/server && npx tsx tests/terminal.test.ts`.

Commit per phase (single-line messages, staged files only) per AGENTS.md.

---

## Risks / notes

- **SDK skew**: t3 module targets Expo ~56/RN 0.83; ours is Expo 54/RN 0.81. Kotlin uses
  stable expo-modules-core APIs (`ExpoView`, `Prop`, `EventDispatcher`, `OnViewDestroys`);
  expect at most minor compile fixes. The C++/JNI layer is RN-version agnostic.
- **New Architecture**: Expo 54 defaults to new arch; `requireNativeView` + ExpoView works
  under it (t3 runs new arch too).
- **Bundle size**: 4 × libghostty-vt.so + 2 fonts. Acceptable; can drop `armeabi-v7a`/`x86`
  slices later if APK size matters (keep arm64-v8a + x86_64 for emulator).
- **License**: MIT throughout — keep T3 Tools + Ghostty notices in the module dir.
- **Out of scope for MVP**: multi-terminal UI (store supports it; screen starts with one),
  font-size setting persistence, iOS support (contract is platform-neutral; revisit later),
  renaming the native package/module name.
