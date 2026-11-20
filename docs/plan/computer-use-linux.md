# Computer use on Linux, and agent-enabled activation

## Problem
1. Linux is not a supported platform for computer use. `cua.EnsureHostEnv()` returns early on
   anything but darwin, and nothing in the server looks at DISPLAY or starts a display. On a
   headless box the driver cannot reach a screen. The agent had to write `DISPLAY=:99` into
   `~/.console/env`, restart the daemon and hand-start Xvfb.
2. Only the user can switch the capability on. `computer-use` has `AgentMayEnable: false`, so
   `readSkill` answers "only the user can switch it on: ask them to type /computer-use" and the
   `computer` tool never appears. Typing a bare `/computer-use` is accepted by
   `cua.SplitInvocation` (the task may be empty), so the model's claim that a task is required is wrong.
   The failure the user saw is a harness path, not the parser.

## Goals
- Linux is first class, with the same flow as macOS: no manual env edits, no hand-started processes.
- The agent can switch computer use on itself, like `console`, with no approval prompt.

## Part A: Linux support

### A1. Display resolution (new `services/cua/hostenv_linux.go`)
Split `hostenv.go` by platform. `EnsureHostEnv` keeps its macOS behaviour and gains a Linux
counterpart, `ResolveDisplay()`, which runs in order:
1. `DISPLAY` or `WAYLAND_DISPLAY` already set and reachable: use it. Probe by connecting to
   `/tmp/.X11-unix/X<n>`, with no shell-out to xdpyinfo.
2. A desktop session is discoverable (a live `/tmp/.X11-unix/X*` socket owned by the current user):
   attach to it. The agent then drives the real desktop.
3. Otherwise start a private headless Xvfb (A2).
The result is applied to the driver process environment only, never written to `~/.console/env`.
Behaviour lives in one place, and tests can stub the probe.

### A2. Managed Xvfb (new `services/cua/xvfb_linux.go`)
- Pick a free display number with `-displayfd`, so there is no hard-coded `:99` and no race.
- Start `Xvfb` with `-screen 0 1280x800x24 -nolisten tcp`, owned by `cua.Manager`.
- Start it lazily on the first `Driver()` call and stop it in the manager's shutdown path.
- If the process dies, restart it on the next call. Xvfb holds no state we need.
- Xvfb missing from PATH returns a typed error that names the package to install
  (`apt install xvfb`, `dnf install xorg-x11-server-Xvfb`).

### A3. Early, readable failure
`Manager.Driver()` currently caches whatever the open returns. Add a preflight that maps failures to
actionable errors: no display and no Xvfb, driver library missing (`CUA_DRIVER_LIB_PATH`), unsupported
Wayland-only session. The text goes to the model and to the Status call. A failed preflight is
cached only briefly, so installing Xvfb does not need a daemon restart.

### A4. CLI parity (`cli/commands/computer_use.go`)
- `computer-use status` reports display source (existing / attached / managed Xvfb), display
  name and driver library path on Linux.
- `computer-use permissions` stays darwin-only, with a clear message elsewhere. Linux has no TCC.
- `computer-use probe` works on Linux through the same `ResolveDisplay`.
- Skip the bundle and reset commands on Linux, with a clear message.

### A5. Driver library
`driver.go` already has `#cgo linux LDFLAGS: -ldl`, so loading is in place. Confirm that a Linux
`libcua_driver.so` is built and located next to the server binary, and that CI/release packaging ships it.
Open question: does upstream cua-driver ship a Linux X11 backend? Verify before building on it.

### A6. SKILL.md
Add a short Linux section: the display is managed automatically, say what to do when the status
reports no display, and tell the agent not to edit `~/.console/env` or restart the daemon.
Keep the macOS-only TCC text under a macOS heading.

## Part B: Agent-enabled activation

Computer use works like `console`: the agent calls `readSkill` and it switches on, with no
approval prompt. `AgentMayEnable: true` is the only change in `run/capabilities.go`.
Typing `/computer-use <task>` stays as a shortcut.

- Plan mode denies the exec tier, so the capability is neither listed nor enabled there. That is the
  existing `Tier` behaviour and needs no new code.
- An earlier iteration added an Allow/Deny prompt through the ask flow. It was removed on purpose:
  switching it on should not need permission.

## Part C: Tests (per AGENTS.md, specific test files, no inline test modules)
- `tests/services/cua_hostenv_linux_test.go` (build tag linux): resolve order with a stubbed
  socket probe, the existing display wins, an attached desktop beats Xvfb, and the missing-Xvfb error.
- `tests/services/cua_xvfb_linux_test.go`: display number parsed from the displayfd stub, restart
  after exit, and shutdown stops the process. Skip when Xvfb is absent.
- `run/capabilities_test.go`: agent activation goes through approval, deny leaves it off, plan mode hides it.
- `tests/services/cua_invoke_test.go`: add a case that a bare `/computer-use` activates.
Run only the relevant file, e.g. `cd apps/server-go && go test ./tests/services/ -run TestSplitInvocation -v`.

## Order of work
1. A1 + A3 (display resolution and clear errors). Fixes the immediate pain, no new process.
2. A2 (managed Xvfb).
3. B (agent activation, no approval prompt).
4. A4 + A6 (CLI and skill text), then A5 packaging.
Each step is one commit with a single-line message.

## Open questions
- Does the cua driver have a working Linux X11 backend, or only macOS and Windows? This decides the size of Part A.
- Wayland-only desktops: out of scope for v1 (report unsupported), or use XWayland?
- When attaching to a real desktop on Linux, should the agent ask for confirmation, as macOS does through TCC?