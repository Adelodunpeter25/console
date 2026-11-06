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
- **Preservation**: the migration is a port, not a redesign. Every existing functionality moves
  1:1 — see "The preservation rule" below.

## The preservation rule

The NSH app must preserve **every single functionality** of the current desktop app, and its
UX as users know it today. What it must *not* inherit are the known bugs and bad UX: those are
fixed deliberately as part of the migration (each one noted when it's ported), not carried over
and not silently "improved". A port that quietly changes how a feature behaves is wrong; a port
that knowingly ships a bug it could have fixed is also wrong.

The bar is set highest at the workspace model, which is the spine of the app:

- A **workspace** belongs to a project and holds its chats, browser tabs, terminals and
  file/diff tabs together — same project, one workspace.
- **Separate worktrees of the same project are separate workspaces**, each with its own tabs
  and state.
- Pane trees, tab stashing per project (`project_workspace_roots`), workspace close paths, and
  the persisted layout (`persistence/`) all carry over exactly.

Every phase's "Done when" includes this rule; phase 2 (where the workspace model moves) is
where it bites hardest.

## Current state

- Research done and source-verified: [assessment.md](assessment.md).
- NSH 0.4.0 has been exercised end to end (init, TSX screens, keymap, headless tests, web
  backend) outside the repo; nothing in `apps/` has been touched.

## Priorities

The priority is **migrating the desktop app over** (phases 0–5). The web build is **not a
priority** — it is planned after the cutover (phase 6) and only started once the desktop app
is the real Console app. Phases 2–4 can interleave once phase 1 lands.

## Phases

| Phase | File | What it delivers |
| --- | --- | --- |
| 0 | [phase-0-spike.md](phase-0-spike.md) | One real Console screen in TSX; go/no-go |
| 1 | [phase-1-foundation.md](phase-1-foundation.md) | NSH app crate wired to `console-core` |
| 2 | [phase-2-workspace-screens.md](phase-2-workspace-screens.md) | Chat, workspace, settings screens |
| 3 | [phase-3-browser-and-devices.md](phase-3-browser-and-devices.md) | Built-in browser + device streaming |
| 4 | [phase-4-editor-and-visualise.md](phase-4-editor-and-visualise.md) | Editor, diffs, `/visualise`, terminal |
| 5 | [phase-5-cutover.md](phase-5-cutover.md) | Parity checklist, packaging, remove GPUI app |
| 6 | [phase-6-web.md](phase-6-web.md) | Web build on Vercel — later, not a priority |

Each phase ends with something runnable. The GPUI app is deleted only in phase 5.
