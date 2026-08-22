# Console Terminal Native Module (Android)

Local Expo module owning the native terminal surface for the mobile app, backed by
Ghostty's `libghostty-vt` (prebuilt `.so` for all 4 Android ABIs in `jniLibs/`).

Ported from T3 Tools' MIT-licensed `t3-terminal` module — see
`THIRD_PARTY_NOTICES.md` for attribution and license obligations.

The JavaScript contract is intentionally small:

- input from the native surface is emitted as `{ data: string }`
- resize from the native surface is emitted as `{ cols: number, rows: number }`
- remote PTY output arrives as the `initialBuffer` prop (raw string incl. ANSI escapes);
  the view diffs it against what it already fed and only pushes the suffix into
  `libghostty-vt`, which also gives free replay on remount/font-size change

## Layout

- `android/src/main/java/expo/modules/consoleterminal/` — Kotlin side
  (`ConsoleTerminalModule`, `ConsoleTerminalView`, `TerminalCanvasView`,
  `TerminalFrame`, `GhosttyBridge`)
- `android/src/main/cpp/console_terminal_jni.cpp` — JNI bridge to `libghostty-vt`
  (symbol prefix must match the Kotlin package: rebuild via CMake on app build)
- `android/src/main/cpp/include/` — vendored libghostty-vt C headers (revision pinned in `GHOSTTY_REVISION`; must match the `libghostty-vt.so` builds)
- `android/src/main/jniLibs/` — prebuilt `libghostty-vt.so` per ABI
- `android/src/main/assets/fonts/` — MesloLGS NF fonts

## Upgrading libghostty-vt

`scripts/build-libghostty-android.sh` (+ patches under
`scripts/libghostty-android-patches/`) rebuilds `libghostty-vt.so` from source.
Requires ANDROID_NDK_HOME and Zig 0.15.2.

## Registration

Referenced from `apps/mobile/package.json` as a `file:` dependency; JS accesses the
view via `requireNativeView("ConsoleTerminalSurface")`.
