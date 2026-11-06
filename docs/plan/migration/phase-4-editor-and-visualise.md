# Phase 4 — editor, diffs, /visualise

**Goal**: the heavy custom UI, which today is the deepest GPUI coupling (the `gpui-editor` fork)
and the weakest GPUI result (`/visualise` is not interactive).

## Editor and diffs

- `nsh/base` ships an **Editor** (syntax highlighting, the bat grammar set) and a **DiffView**
  (split/unified, word-level highlights, collapsed unchanged lines) — Rust components, usable
  from TSX. This replaces the pinned `gpui-editor` fork and its `syntax`/`editor-ui` crates.
- The file-and-diff workspace tab renders through `DiffView`; server diff payloads feed it
  directly, no client-side diffing needed.

## /visualise

- Rebuilt as TSX screens: layout and interaction with Tailwind, custom drawing with `Canvas`
  (2D) or `Shader` (WGSL) where a chart needs it, gestures via NSH's pan/touch bindings.
- Interactivity is the acceptance bar: hover, select, zoom — not a static picture.

## Terminal tab

- The plan is to adopt the **upstream NSH terminal component**: the NSH creator has demoed a
  terminal built on NSH (see his Twitter), and it is expected to land in the framework. We use
  that when it ships — no custom terminal emulator of our own.
- Until it lands upstream, the terminal tab stays on the GPUI app, or the NSH app shows the
  terminal as read-out text behind a flag; track the upstream component and wire it in phase 3/4
  order as availability allows. The server-side terminal machinery is unchanged either way.

## Done when

- File-and-diff tab and `/visualise` render from the NSH app with tests.
- The `gpui-editor` dependency is used by nothing but the legacy GPUI app.
- The terminal tab runs on the upstream NSH terminal component once released (or the tab is the
  only remaining GPUI-side surface, tracked explicitly here).
