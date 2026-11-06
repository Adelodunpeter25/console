# Phase 6 — cutover

**Goal**: the NSH app becomes *the* Console app; the GPUI app is removed.

## Parity checklist (verify per platform: macOS, Windows, Linux)

- Menus, window chrome, title bar (`nsh/desktop`), context menus.
- Global shortcuts, single instance, deep links, file associations — confirm against
  `single_instance.rs`, `app_menu.rs`, `menubar.rs` before assuming NSH covers them; anything
  missing is a native component or an upstream contribution.
- Auto-update: NSH ships an `nsh/updater` module — wire it into the release flow.
- Packaging and signing: `nsh package --target macos|windows|linux` replaces the bespoke bundle
  scripts; notarization/code signing per the NSH docs.
- Android/iOS: the mobile apps move to the same TSX screens (Kotlin app becomes a host for
  `NshView`, or is replaced by the packaged NSH app — decide with real devices).

## Order

1. Ship the NSH desktop app as a beta download alongside the stable GPUI build.
2. Default new downloads to the NSH build once parity is checked.
3. Delete `apps/desktop`'s GPUI sources, the `gpui-editor` fork pins, and the zed/gpui
   dependencies; keep `console-core`, `console-proto`, `console-shared` (now used by the NSH
   crates).

## Done when

- The parity checklist is green on all three desktop platforms, release builds are signed and
  updating via `nsh/updater`, and no target in `apps/` depends on GPUI.
