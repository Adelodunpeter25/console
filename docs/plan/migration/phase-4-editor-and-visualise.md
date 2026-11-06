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

- Keep the server-side terminal as-is; the tab surface starts as read-out text in TSX. A full
  terminal emulator in NSH is its own project — decide after phases 2–3 whether to keep using
  the GPUI app's terminal, embed a webview-based one, or scope it down.

## Done when

- File-and-diff tab and `/visualise` render from the NSH app with tests.
- The `gpui-editor` dependency is used by nothing but the legacy GPUI app.
