# NSH migration — overview

Migrate Console's desktop app from GPUI to [NSH](https://nsh.dev): the Rust core stays Rust,
**all UI becomes TSX**, no webview except where a feature needs one (built-in browser, device
player). The same codebase targets macOS, Windows, Linux, iOS, Android and the web — the web
build deploys as a static site and talks to the Go server.

## Decisions already made

- **Framework**: NSH, not MyGo (Go UI is a non-starter), not Electron/Tauri (webview), not raw
  GPUI (styling ergonomics, compile times). See [assessment.md](assessment.md) for the research
  and source-verified findings.
- **Split**: Rust crate = core only (logic, services, state), exported to TSX through
  `#[native_module]`; every screen is TSX with Tailwind classes.
- **Pace**: no rush; the GPUI app keeps working and shipping until the NSH app reaches parity.

## Current state

- Research done and source-verified: [assessment.md](assessment.md).
- Demo: a TSX notes app (`~/apps/notes` in the orb) runs on the web backend and headless tests;
  `mod+n` keymap binding verified in-browser. NSH 0.4.0.
- Nothing in `apps/` has been touched.

## Phases

| Phase | File | What it delivers |
| --- | --- | --- |
| 0 | [phase-0-spike.md](phase-0-spike.md) | One real Console screen in TSX; go/no-go |
| 1 | [phase-1-foundation.md](phase-1-foundation.md) | NSH app crate wired to `console-core` |
| 2 | [phase-2-workspace-screens.md](phase-2-workspace-screens.md) | Chat, workspace, settings screens |
| 3 | [phase-3-browser-and-devices.md](phase-3-browser-and-devices.md) | Built-in browser + device streaming |
| 4 | [phase-4-editor-and-visualise.md](phase-4-editor-and-visualise.md) | Editor, diffs, `/visualise` |
| 5 | [phase-5-web.md](phase-5-web.md) | Web build on Vercel, connected to the server |
| 6 | [phase-6-cutover.md](phase-6-cutover.md) | Parity checklist, packaging, remove GPUI app |

Each phase ends with something runnable; phases 2–4 can interleave once phase 1 lands. The GPUI
app is deleted only in phase 6.
