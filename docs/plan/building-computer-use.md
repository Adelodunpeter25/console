# Building Computer Use Ourselves

Status: **planning** (2026-10-06). Companion to `computer-use.md`, which covers the overall feature (permissions, remote viewing, invocation). This document covers **how we get the capability itself**: start with an existing package, then build our own.

## 1. Decision

1. **Start with trycua's Cua Driver.** It is the industry-standard open-source computer-use package, installs as an MCP server, and the Go server can register it today with no code (see `computer-use.md`, Phase 0). This gets computer use working quickly and shows us what the experience should be.
2. **Study the existing implementations before writing ours.** Clone and read both reference projects (section 3).
3. **Then build our own native computer use** and replace the external package. Reason: it will be built into Console (own tool, own images, own permission model, no third-party runtime to install on the server), and we control the behaviour and the platform story (macOS now, Linux server later).

Starting with Cua is explicitly the first step, not the end state.

## 2. Why build it ourselves

- Computer use becomes a normal built-in tool, like the browser tool, instead of an external process the server has to install and keep working.
- Screenshots flow natively to the model (today the MCP adapter drops MCP images, see `computer-use.md` 3.3).
- We decide how permissions, approvals and stop behave, and can fit the plan-mode / bypass-only end state.
- The same tool interface can later have a Linux backend for the Ubuntu server.

## 3. Reference projects to study

### 3.1 trycua/cua (clone and read, full history)

To do: `git clone https://github.com/trycua/cua` into `~/Developer/Projects` (full clone, not shallow) and read it. It is a large monorepo; focus on **Cua Driver**, not the VM tooling (Lume, Spaces, SDK, Bench).

What to find out:
- What the MCP tools are called and what they take and return (and which have `readOnlyHint`).
- How it delivers input in the background, handles focus, and handles minimized windows.
- How it observes apps (accessibility tree versus screenshots and pixels).
- How it supports Windows and Linux (the README lists them), and whether Linux works headless.
- Transports (stdio only, or HTTP too), install story, licence of the driver itself (the README says most components are MIT and Cua Spaces is FSL-1.1-MIT; confirm for the driver).
- Which behaviours it gets right that we want, and where it is weak.

### 3.2 shhivv/arc-cua (already cloned at `~/Developer/Projects/arc-cua`, full history)

Findings so far (read from the code and the 91 commits):
- Two tools in one MIT package: **arc-driver** (a policy-free macOS driver, MCP over stdio, 16 tools) and **arc-cua** (a decision-model loop we do not need).
- The driver core is about 4.7k lines of Python (5.5 to 6k with caches and errors). Most of it was written in two days after about two weeks of groundwork. Much of the history was AI-assisted, and the benchmarks are the author's own, on one Mac (macOS 26.6).
- It is accessibility-first: it reads each window's accessibility tree, acts through accessibility actions where it can, and sends no event at all in those cases.
- Its own benchmark against Cua Driver (their file `BENCHMARKS.md`) reports it faster and more reliable on their tasks, but it is the author's benchmark on tasks tuned for it; treat as a hint, not proof.

## 4. What a native macOS implementation needs

### 4.1 APIs (from the arc-cua code)

- **Public APIs cover almost everything:** the accessibility API (read, walk, actions, observers), `CGWindowList*` for window info, `NSWorkspace` / `NSRunningApplication`, Vision for OCR, and the permission checks (`AXIsProcessTrusted`, `CGPreflightScreenCaptureAccess`).
- **Screenshots:** arc-cua uses `CGWindowListCreateImage`, which is unavailable in the macOS 15 SDK. We use **ScreenCaptureKit**.
- **Private APIs appear in only two arc-cua files:**
  1. Background input without moving the pointer or stealing focus: SkyLight functions (`SLEventPostToPid`, `SLPSPostEventRecordTo`, ...), undocumented event field numbers, a hand-built 0xF8-byte focus record, and a Chromium auth message that reads struct offsets which vary by macOS release. This is the most fragile part.
  2. Making minimized or hidden windows actionable on an invisible display (`CGVirtualDisplay*`).
- arc-cua has **no foreground input path**: when a private symbol is missing, input fails instead of falling back.

### 4.2 Architecture (proposal)

```
 Go server
 +---------------------------------+   JSON over stdio   +----------------------------+
 | built-in `computer` tool        | <-----------------> | Swift helper (macOS)       |
 | (internal/agent/tools/)         |                     |  - Accessibility (AX)      |
 |  observe / act / screenshot     |                     |  - ScreenCaptureKit        |
 |  status / settle / release      |                     |  - CGEvent input           |
 +---------------------------------+                     |  - change journal, settle  |
        |  platform-neutral interface                    +----------------------------+
        +--> later: Linux helper (AT-SPI / X11), same tool interface
```

- **A small Swift helper** (public APIs only at first), started by the Go server and spoken to over JSON on stdio. Go cannot call Accessibility cleanly; Swift gives direct access to AX, ScreenCaptureKit and CGEvent.
- **A built-in `computer` tool** in the server's tool registry, like the browser tool. Screenshots return as native image results, so no MCP adapter fix is needed.
- **A platform-neutral tool interface** (observe, act, screenshot, status) so a Linux helper can plug in for the Ubuntu server.
- The tool can also be exposed over MCP later if useful, but that is not the goal.

### 4.3 Behaviours to copy (the edge-case checklist)

The arc-cua history shows what took the time. Turn each into a test:
- **Stale or changed screens:** keep a journal of structural accessibility notifications (windows, sheets, menus, focus changes). Refuse to act when the app changed since the snapshot, and return a fresh snapshot. Compare an element's name, value and state before acting, and include row selection state, or sidebar items look changed.
- **Exact window targeting:** act on one window id, resolved once, never "the focused window" again. A focused sheet resolves to its parent window.
- **Minimized windows and hidden apps:** the accessibility tree is still readable and `AXPress`/`AXValue` still work in place; only key and pointer events need the window on screen.
- **Electron and Chromium apps:** set `AXManualAccessibility`; the tree may need up to about 0.6 s to appear after a page loads; some apps need relaunching with `--force-renderer-accessibility`.
- **Text entry:** writing `AXValue` does not fire a web page's handlers and does not mark a native document edited. Type the text with key events on web fields and documents.
- **Sliders, steppers, dates:** use increment and decrement actions, step back if they overshoot. Identify date parts by how they step, not by English labels.
- **Scrolling:** scroll events to a background web view are ignored; set the scroll bar's value through accessibility instead.
- **Menus:** AppKit commits a picked item about 0.3 s after the click, so wait for the commit. Enabled state can be stale, so press the item anyway.
- **Settling:** count accessibility notifications; done when quiet for about 0.15 s, 0.6 s if nothing reacted, 2 s at most. No Screen Recording permission needed for this.
- **Large lists:** read only visible rows (a 2,000-file list went from about 30 s to about 50 ms).
- **Safety:** refuse risky controls (delete, send, purchase, close) unless allowed, redact secrets, and use stable error codes. See `computer-use.md` for how this fits our permission end state.

## 5. Phases

**Phase A: start with Cua Driver** (needs no code). Do `computer-use.md` Phase 0 and Phase 1: install Cua Driver on the Mac mini, register it in the server's MCP config, run harmless tasks, and pass MCP images through to the model. This is the baseline to compare against and the working feature in the meantime.

**Phase B: study.** Clone trycua/cua in full and read the driver (section 3.1). Re-read arc-cua where needed. Write down the conclusions in this document (what to adopt, what to avoid, which tools to expose).

**Phase C: native core, public APIs only.** Swift helper plus the Go `computer` tool: permission status, apps and windows, an accessibility snapshot with stable element ids and actions, performing actions (accessibility actions first), ScreenCaptureKit screenshots, the change journal with stale-action refusal, and settle. Build the checklist in 4.3 as tests against a small fixture app plus Calculator, System Settings and Finder.

**Phase D: input fallbacks.** Foreground input through public `CGEvent` APIs for what accessibility actions cannot do (this can move the pointer). On a dedicated Mac mini this is likely acceptable.

**Phase E: replace Cua.** Compare against Cua Driver on the same tasks (a small benchmark harness; arc-cua's `benchmarks/` is a model). Switch the default to the native tool once it is at least as reliable for our tasks, and keep Cua as an optional MCP server.

**Phase F (optional): private background input.** Only if foreground input proves a problem. Load private symbols at runtime, fail closed, report capability through the `status` tool, and treat it as best effort across macOS versions.

**Phase G: Linux.** A Linux helper behind the same tool interface (accessibility tree via AT-SPI, a virtual display, input through X11), for the Ubuntu server. Needs its own study; Cua's README lists Linux support, which is worth reading in Phase B.

## 6. Risks and open questions

- Effort: my rough estimate for Phase C is a few thousand lines of Swift (about 3 to 5k, an estimate, not a measurement) plus the Go tool and a test fixture. arc-cua suggests it is tractable, but its author needed many fixes after the first version.
- Private APIs break between macOS releases. Keeping them optional (Phase F) limits the damage.
- Which macOS versions must the Mac mini support? ScreenCaptureKit screenshots need a recent macOS.
- Permissions are granted per binary: the Swift helper (or whatever launches it) needs Accessibility and Screen Recording.
- Licences: arc-cua is MIT (per its `pyproject.toml`). Check the licence of any Cua Driver code before copying anything; read-and-reimplement is the safer route.
- Do we expose the native tool over MCP as well, so other clients can use it?
- How do we test accessibility-driven behaviour reliably in CI (probably only on a Mac runner)?

## 7. Verification

| Check | How |
|---|---|
| Phase A works | agent opens an app and reads its screen through Cua Driver; screenshot reaches Claude |
| Native observe matches expectations | snapshot of Calculator and System Settings has stable ids and the right actions |
| Stale refusal | open a sheet after a snapshot, then act; the tool refuses and returns a fresh snapshot |
| Background behaviour | acting on a window does not change the front app or move the pointer (for accessibility-action paths) |
| Parity with Cua | the same task list runs on both; compare success and time before switching the default |
