# Phase 1 — foundation: the NSH app crate

**Goal**: a real NSH app in the repo that links Console's existing Rust core and builds for
desktop + web, while the GPUI app keeps shipping.

## Structure

- New `apps/console-nsh/` (mixed template): `nsh.json`, TSX screens in `src/`, one or more Rust
  crates under `native/` exported through `#[native_module("console/…")]`.
- Reuse, don't fork: `apps/desktop/crates/console-core` (types, services, `HttpTransport`) and
  `console-proto`/`console-shared` are UI-free Rust — the NSH crate depends on them by path, the
  same way the GPUI app does. Nothing moves yet.
- Theme: port Console's palette into a theme file (shadcn token names map directly) so every
  later screen is styled from day one.
- `.nsh/types` and `nsh check` in CI alongside the existing workspace checks.

## Validate

- `nsh run` opens a shell screen (nav + placeholder panes) on the dev machine; `nsh test` passes
  headless; `nsh run --target web` serves the shell.
- A `console-core` service called from TSX round-trips (e.g. `HttpTransport` fetch of
  `/api/…` against a local server).

## Done when

- CI builds the app for desktop and web on every push.
- A "hello" screen reads real data from the Go server through console-core in TSX.
