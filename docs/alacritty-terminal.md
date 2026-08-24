# Integrating `alacritty_terminal` into a GPUI App — End-to-End Implementation Guide

**Context:** native Rust desktop app (conductor.build-style, multi-pane/agent workspace) built on Zed's **GPUI** framework, embedding `alacritty_terminal` as the terminal emulation core.

**Transport model:** PTY spawning and process lifecycle live on an existing **WebSocket backend**, not locally. The client sends/receives **JSON-framed messages** (`{type: "data" | "resize" | "exit" | ..., payload: ...}`) over the socket. This means the client never spawns a shell, never manages a PTY, and never runs `portable-pty` — it only feeds bytes it receives into `alacritty_terminal`'s VTE parser and sends input/resize back as JSON messages. Phases 2 and 4 below are rewritten for this; everything downstream (Term core, GPUI bridging, rendering, input, layout) is unchanged, since `alacritty_terminal` doesn't care where the bytes came from.

This is written to be handed to a coding agent (Claude Code, etc.) as a task list. Each phase has a goal, concrete tasks, key APIs/types, known pitfalls, and a "definition of done" so the agent can self-verify before moving on. Reference implementations to study while working: Zed's own `crates/terminal` + `crates/terminal_view` (note: Zed's *remote* terminal mode — SSH remoting — is architecturally the closest analog to your case, since it also feeds a local `Term` from bytes arriving over a transport rather than a local PTY), and the open-source `paneflow` project (github.com/ArthurDEV44/paneflow) for the local-PTY version of the same rendering/input work.

---

## Phase 0 — Prerequisites & Architecture Decisions

**Goal:** lock in the foundational decisions before writing code, since they're expensive to reverse later.

**Tasks**
1. Confirm GPUI is already wired into the app (window creation, `Application::run`, at least one `Entity<T>` rendering). If not, that's a blocker — this guide assumes GPUI is already the app's UI layer.
2. Decide the PTY backend: use `portable-pty` (recommended — abstracts ConPTY/Windows vs openpty/Unix behind one trait, zero `#[cfg]` at call sites) rather than hand-rolling PTY code or using `alacritty_terminal`'s own `tty` module directly (that module is tied to Alacritty-the-app's assumptions).
3. Decide **not** to use `alacritty_terminal::event_loop::EventLoop::spawn()`. It's convenient for a simple embedded terminal, but you lose control over OSC scanning, synchronized-output coalescing, and clean shutdown — all of which you'll want for a multi-pane workspace. Plan to hand-roll two threads per session instead (Phase 4).
4. Pin crate versions:
   - `alacritty_terminal = "0.26"` (pull from crates.io directly, not a vendored/forked copy — a fork-of-a-fork is a maintenance trap)
   - `portable-pty = "0.8"`
   - `gpui` — not reliably published to crates.io pre-1.0; pin to a specific commit/tag of `zed-industries/zed` as a git dependency and re-pin deliberately (GPUI has frequent breaking changes)
5. Decide on threading model up front: **PTY reader/writer threads are the only place raw bytes flow; the GPUI main thread is the only place `Entity<T>` is touched.** Every subsequent phase enforces this boundary — write it down somewhere the agent can re-read.

**Definition of done:** a short ARCHITECTURE.md (or equivalent) in the repo stating: PTY backend, alacritty_terminal version, "no EventLoop::spawn()", and the threading boundary rule above.

---

## Phase 1 — Dependencies & Scaffolding

**Goal:** get the crates compiling with a skeleton module layout, no behavior yet.

**Tasks**
1. Add to `Cargo.toml`:
   ```toml
   alacritty_terminal = "0.26"
   portable-pty = "0.8"
   futures = "0.3"      # for UnboundedSender/Receiver bridging to GPUI
   ```
2. Create module skeleton:
   ```
   src/terminal/
     mod.rs
     pty.rs            # PtyBackend trait + PortablePtyBackend impl
     pty_loops.rs       # reader/writer threads
     term_entity.rs     # the GPUI Entity<Terminal> wrapper
     listener.rs        # EventListener impl bridging alacritty -> GPUI
     element.rs         # GPUI Element: layout + paint (rendering)
     input.rs           # keyboard/mouse -> alacritty input translation
   ```
3. Confirm the workspace builds with the new empty modules wired into `mod.rs` / the crate root.

**Definition of done:** `cargo build` succeeds with empty modules in place; no runtime behavior yet.

---

## Phase 2 — PTY Layer

**Goal:** spawn a real shell process behind a PTY, cross-platform, with a clean trait boundary.

**Tasks**
1. Define a `PtyBackend` trait with methods roughly: `spawn(shell, cwd, env, size) -> Result<PtyHandle>`, `resize(size)`, `writer() -> Box<dyn Write>`, `reader() -> Box<dyn Read>`, `kill()`.
2. Implement `PortablePtyBackend` using `portable_pty::native_pty_system()`. This resolves to ConPTY on Windows and `openpty` on Unix automatically — do not branch on `cfg(target_os)` for spawn logic itself.
3. Wire environment variable injection (`TERM=xterm-256color`, plus any app-specific vars, e.g. `YOURAPP_TERM=true`) at spawn time.
4. Implement process shutdown as a platform seam (this is one of only two places `#[cfg(target_os)]` should appear in the whole terminal subsystem):
   - Unix: `libc::kill(pid, SIGTERM)`, grace window, then `SIGKILL`.
   - Windows: `TerminateProcess` + `WaitForSingleObject` via `windows-sys`.
5. Implement CWD detection as the other platform seam:
   - Linux: read `/proc/<pid>/cwd`.
   - macOS: `proc_pidinfo`/`libproc`.
   - Windows: no direct equivalent — fall back to OSC 7 emitted by the shell prompt (handled in Phase 4).

**Key APIs:** `portable_pty::{native_pty_system, PtySize, CommandBuilder}`.

**Pitfalls**
- Don't spawn the shell with a login-shell flag inconsistently across platforms — decide once (`-l` on Unix shells) and document it.
- Windows ConPTY resize must go through the PTY handle's resize call, not a raw ioctl — `portable-pty` already handles this, don't bypass it.

**Definition of done:** a shell process spawns, you can read raw bytes from it in a test/debug harness (e.g. print to stdout), and killing it cleans up the OS process with no zombies.

---

## Phase 3 — Alacritty `Term` Core & Event Listener

**Goal:** get `alacritty_terminal`'s grid state machine live, receiving parsed terminal state, with a way for it to notify GPUI when something changed.

**Tasks**
1. Define your event listener type, e.g. `AppListener`, implementing `alacritty_terminal::event::EventListener`. It should be a thin newtype wrapping a `futures::channel::mpsc::UnboundedSender<AlacEvent>` (your own enum wrapping/forwarding `alacritty_terminal::event::Event` plus custom variants like `Wakeup`).
2. Construct the shared terminal state as `Arc<alacritty_terminal::sync::FairMutex<alacritty_terminal::Term<AppListener>>>`. `FairMutex` (re-exported from alacritty_terminal) matters here — a plain `std::sync::Mutex` can starve the reader thread under a busy TUI; `FairMutex` prevents that.
3. Construct `Term::new(config, &grid_dimensions, listener)` with your initial size (rows/cols derived from the GPUI pane's pixel size divided by the font's cell metrics — you need font metrics wired up before this works correctly; stub with a fixed size if font layer isn't ready yet).
4. Create the `futures::channel::mpsc::unbounded()` channel: sender goes into the listener, receiver will be drained on the GPUI side in Phase 5.

**Key APIs:** `alacritty_terminal::Term`, `alacritty_terminal::sync::FairMutex`, `alacritty_terminal::event::{Event, EventListener}`, `alacritty_terminal::vte::ansi::Processor` (for Phase 4).

**Pitfalls**
- Don't hold the `FairMutex` lock across an `.await` or across a call that could block — lock, mutate, drop, exactly like the hot-path pattern in Phase 4.
- `Term` is generic over the listener type; if you later want multiple terminal "flavors" (e.g. a read-only log viewer vs an interactive shell), consider whether they share one listener type or need separate ones now, before this generic parameter spreads through your codebase.

**Definition of done:** you can construct a `Term<AppListener>` and manually feed it a byte string through a `Processor` (Phase 4 will make this a real loop) and inspect the resulting grid content in a test.

---

## Phase 4 — I/O Loops (Reader / Writer Threads)

**Goal:** the actual bytes-in-bytes-out engine: two hand-rolled detached threads per terminal session, bridging PTY ↔ `Term`.

**Tasks**
1. **Reader loop** (`pty_reader_loop`): on a dedicated thread, loop:
   - Read up to 4096 bytes from the PTY into a buffer.
   - Lock the `Term` mutex, run your `alacritty_terminal::vte::ansi::Processor::advance(&mut term, &buf[..n])`, drop the lock immediately.
   - Check `processor.sync_bytes_count()` against `n`: if fewer bytes were inside a DEC 2026 synchronized-output window than were read, send a single `Wakeup` event through the listener channel. This coalesces bursts from busy TUIs (neovim, btop, etc.) into one repaint signal instead of hundreds.
   - Run any OSC scanners you need inline here, before/after `advance` — OSC 7 for CWD, OSC 133 for shell prompt/command boundaries, OSC/DA for shell identification — since you already have the raw bytes in hand.
2. **Writer/control loop** (`pty_message_loop`): on a second thread, receive a `Msg` enum (`Write(Vec<u8>)`, `Resize(PtySize)`, `Shutdown`) over `std::sync::mpsc`, and apply it to the PTY (write to the writer half, or call the backend's resize/kill).
3. Wire a `PtyNotifier`-style handle (a cheap `Sender<Msg>` clone) that Phase 5's GPUI entity holds, so keyboard input and resize events can reach the writer thread without touching the reader thread.
4. Handle EOF/process-exit on the reader thread: when `read()` returns 0 or errors, send an `Event::PtyExit` (or your equivalent) through the listener channel so the GPUI side can show "process exited" state.

**Reference hot path** (this is the shape to aim for):
```rust
let mut term = term.lock();
processor.advance(&mut *term, &buf[..n]);
drop(term);
if processor.sync_bytes_count() < n {
    listener.send_event(AlacEvent::Wakeup);
}
```

**Pitfalls**
- This is the single most important architectural rule in the whole integration: **raw bytes and the `Term` mutex live only on these two threads.** GPUI's main thread never reads the PTY directly and never advances the VTE processor.
- Don't send a wakeup per chunk unconditionally — that's the polling-timer mistake in reverse; you'll flood the UI thread with repaints during e.g. `cat largefile.txt`. The sync-bytes check above is what prevents it.
- Make sure `Shutdown` is idempotent and closes both threads — a terminal that's closed by the user but whose reader thread is still blocked on `read()` will leak.

**Definition of done:** you can type into a raw stdin harness, see the writer thread deliver it to the shell, see the shell's response bytes flow through the reader thread and mutate the `Term` grid, confirmed by dumping grid contents to a log — all without touching GPUI yet.

---

## Phase 5 — GPUI Entity & Event Bridging

**Goal:** wrap the terminal session as a proper GPUI `Entity<Terminal>` that reacts to PTY activity via the push model, not polling.

**Tasks**
1. Define `struct Terminal { term: Arc<FairMutex<Term<AppListener>>>, notifier: PtyNotifier, ... }` and make it a GPUI entity (`cx.new(|cx| Terminal::new(...))`).
2. In the entity's constructor, `cx.spawn()` a task that drains the `UnboundedReceiver<AlacEvent>` from Phase 3/4. Batch drains over a short window (aim for ~4ms, capped at ~100 events per batch) rather than calling `cx.notify()` per event — this is the "keystroke-to-pixel" path and batching is what keeps it smooth under load.
3. After each batch, issue exactly one `cx.update()` (to apply any state changes derived from events — e.g. title change, CWD change, exit status) and one `cx.notify()` (to trigger repaint).
4. Use GPUI's `EventEmitter`/`cx.emit`/`cx.subscribe` for anything else in the app that cares about terminal activity (e.g. a sidebar showing "this pane is busy," dev-server detection, agent-lifecycle hooks) — emit custom events like `ActivityBurst` from here rather than having other parts of the app poll this entity's state on a timer.
5. Implement resize propagation: when the GPUI element's layout size changes (Phase 6/7), compute new rows/cols from pixel size ÷ cell metrics, send a `Msg::Resize` to the PTY notifier, and call `term.lock().resize(new_size)`.

**Pitfalls**
- Do not poll. If you catch yourself writing a `Timer::after` loop that repeatedly checks terminal state, that's the anti-pattern — convert it to an emitted event from the source of truth (the reader thread / PTY backend) instead.
- Keep `Entity<T>` mutation strictly on the GPUI main thread; the batched drain-and-update above is the only bridge point.

**Definition of done:** typing in a bare debug window updates a GPUI-rendered placeholder (even just a text dump of the grid) with visibly batched, non-flooding repaints, and idle terminals produce zero repaints.

---

## Phase 6 — Rendering (the GPUI `Element`)

**Goal:** actually draw the terminal grid as glyphs, matching alacritty's cell model to GPU-rendered text.

**Tasks**
1. Implement a custom GPUI `Element` (not just a `div()` tree) for the terminal surface — dense terminal rendering needs direct control over glyph layout that a generic flex-box tree doesn't give you cheaply.
2. In `layout()`, compute available size, derive rows/cols from the font's fixed cell width/height (terminal fonts must be monospace; measure the cell size once from font metrics and cache it).
3. In `paint()`, lock the `Term`, iterate its visible grid (`term.grid()` / renderable content API), and for each cell: resolve foreground/background color (16-color, 256-color, and 24-bit truecolor palettes all need mapping), resolve bold/italic/underline/strikethrough flags, and emit glyph draw calls through GPUI's text/atlas system.
4. Render the cursor (block/beam/underline per alacritty's cursor shape state) and any active selection highlight as separate overlay passes after the base grid.
5. Handle scrollback: alacritty's `Term` maintains scrollback internally; expose a scroll-offset so `paint()` can render historical lines, and wire mouse-wheel/scrollbar input (Phase 7) to adjust that offset.
6. Special-case block-drawing characters (U+2580–U+259F) explicitly rather than relying on the default font fallback path — incomplete coverage tables here cause subtle rendering gaps between adjacent block glyphs that are very hard to root-cause from shader/geometry debugging alone. Build (or find) a complete coverage table for this range up front.

**Pitfalls**
- The most common bug in this phase looks like a rendering/geometry bug (gaps between block characters, misaligned cells) but is actually a **font coverage/fallback** bug — if geometry math checks out on every probe and the artifact persists, check which codepoints are silently falling back to a different font before touching shader code again.
- Don't lock `Term` for the entire paint pass if paint is expensive — clone/copy just the renderable cell data you need, drop the lock, then draw, so you don't block the reader thread mid-frame.
- True color (24-bit) support is easy to skip accidentally; verify with a truecolor test script (`printf` escape sequences) since 16/256-color often "looks fine" while truecolor is silently wrong.

**Definition of done:** running real TUIs (`htop`, `nvim`, `lazygit`) inside the pane renders correctly at the pixel level: box-drawing characters connect with no gaps, colors match a reference terminal, cursor and selection render correctly, and scrollback scrolls.

---

## Phase 7 — Input Handling

**Goal:** get keyboard, IME, mouse, clipboard, and resize events from GPUI into the terminal correctly.

**Tasks**
1. Wire keyboard input through GPUI's `actions!`/key-dispatch system for app-level bindings (split pane, close pane, etc.), and a raw key-event handler for everything that must reach the shell verbatim (regular typing, control sequences, function keys, application cursor mode).
2. Translate GPUI key events into the byte sequences alacritty expects (this mapping already exists in `alacritty_terminal`'s input-handling helpers / mirrors Alacritty's own `input.rs` — reuse rather than reimplement where possible), then send via the PTY notifier's `Write` message.
3. Implement IME composition handling explicitly and test with an actual CJK input method — this is one of the two features (along with sub-frame key chords) that pushed the original reference implementation away from a webview-based approach, so budget real testing time here, not just a code review.
4. Implement mouse: click-drag selection (updates a selection range on `Term`, triggers repaint), scroll-wheel (adjusts scroll offset or forwards to the app if in alternate-screen/mouse-reporting mode), and right-click/paste per your app's convention.
5. Implement clipboard: copy from selection (extract text from `Term`'s selection range), paste (bracketed-paste aware — check `Term`'s bracketed-paste mode flag before wrapping pasted text).
6. Wire resize end-to-end: window/pane resize → recompute rows/cols → `Term::resize` + PTY resize message (this closes the loop opened in Phase 5).

**Pitfalls**
- Application cursor mode and application keypad mode change what bytes arrow keys / numpad keys send — don't hardcode one mapping; read the relevant mode flags off `Term` before encoding.
- Bracketed paste: pasting without checking the mode can break shells/programs that expect literal control characters not to be paste-wrapped, or vice versa.

**Definition of done:** you can run an interactive full-screen program (vim, htop) and drive it entirely via keyboard/mouse indistinguishably from a native terminal; CJK IME composition works; copy/paste round-trips correctly including bracketed paste.

---

## Phase 8 — Terminal-Adjacent Features (optional but expected for a conductor.build-style tool)

**Goal:** the features that make this an agent/dev workspace tool rather than a bare terminal widget.

**Tasks**
1. **CWD tracking:** consume OSC 7 events scanned in Phase 4 to know each pane's current working directory; fall back to `/proc/<pid>/cwd` (Linux) / `proc_pidinfo` (macOS) polling only where OSC 7 isn't available (Windows).
2. **Shell prompt/command boundaries:** consume OSC 133 to know when a command starts/ends — useful for "is this pane busy" indicators and for reliably injecting text only between commands.
3. **Dev-server / port detection (if relevant to your app):** combine (a) a fast regex pass over terminal output against known framework signatures (Vite, Next.js, uvicorn, etc.) for an immediate signal, with (b) ground-truth kernel-level socket inspection: walk the pane's process tree, then on Linux parse `/proc/net/tcp`/`/proc/net/tcp6` filtering to `TCP_LISTEN` state and matching socket inodes owned by your PID set; on macOS use `libproc` to enumerate socket FDs per PID and filter to `LISTEN`. This two-signal approach catches both the "framework announced its own URL" case and the "output was piped/lost/silent" case.
4. **Hyperlink detection:** OSC 8 explicit hyperlinks plus a URL-regex fallback over cell text, exposed as clickable regions in the Element's paint/hit-test.
5. **Session persistence:** serialize pane layout + CWDs (+ optionally scrollback) on close, restore on launch. Treat this as required-for-v1, not a nice-to-have — losing all open panes on a rebuild/restart is the fastest way to make the tool feel unsafe to use daily.
6. **Agent/automation control plane (if you want external tools or scripts to drive panes):** expose a local-only IPC surface (Unix domain socket on Unix / named pipe on Windows) speaking a small JSON-RPC-style protocol for things like "send text to pane X," "split," "report agent lifecycle event." Keep it strictly local (no network binding) and gate every connection on peer-credential/UID checks (`SO_PEERCRED` on Linux, `LOCAL_PEERCRED` on macOS) plus restrictive filesystem permissions on the socket, since this surface can inject keystrokes into any pane.

**Definition of done:** whichever subset you build is demonstrably wired to real pane state (not mocked), and the IPC surface (if built) rejects connections from other UIDs and has no network listener.

---

## Phase 9 — Process Lifecycle, Shutdown & Layout Integration

**Goal:** make panes/splits behave correctly as first-class citizens of your app's window management, and shut down cleanly.

**Tasks**
1. Integrate the `Terminal` entity into whatever pane/split abstraction your app uses. If you need N-way splits (not just binary), model the layout as an N-ary tree (`Container { children: Vec<{node, ratio}> }`) rather than nested binary splits — binary splits make "three equal columns" and consistent drag-resize surprisingly awkward.
2. On pane close: send `Msg::Shutdown` to the writer thread, join/detach both I/O threads, run the platform-specific process termination (SIGTERM→SIGKILL grace window on Unix, `TerminateProcess` on Windows), and drop the `Entity<Terminal>`.
3. On app quit: ensure every live terminal session runs its shutdown path — don't rely on process-exit-kills-children assumptions cross-platform, verify no orphaned shell processes on all target OSes.
4. Confirm resize propagates correctly through nested splits (a drag on an outer divider must correctly rebalance all affected panes' PTY sizes, not just the two touched leaves).

**Pitfalls**
- A leaked reader thread blocked on `read()` after its pane is "closed" in the UI is the easiest way to accumulate zombie processes and hung threads — test pane-close under load (an actively-printing process) specifically, not just an idle shell.

**Definition of done:** opening and closing many panes repeatedly (including panes running busy processes) leaves zero orphaned OS processes and zero leaked threads, verified with `ps`/Task Manager before and after a stress loop.

---

## Phase 10 — Testing, Performance & Cross-Platform Validation

**Goal:** verify correctness and performance before calling the integration done.

**Tasks**
1. **Correctness suite:** run a battery of real programs per pane — `vim`, `htop`/`btop`, `lazygit`, a build tool with heavy scrolling output, a truecolor test script, a CJK-heavy program — and visually diff against a reference terminal (Alacritty itself, or your OS's default).
2. **Latency:** instrument the keystroke-to-pixel path (shell write → reader thread → VTE advance → wakeup → batch window → `cx.notify()` → GPUI repaint) with a debug-only tracing flag, and confirm the batching window (Phase 5) isn't introducing perceptible input lag (target: comfortably under one frame at your target refresh rate).
3. **Throughput/idle-repaint check:** confirm a busy TUI doing synchronized-output updates doesn't generate more than one repaint per logical frame, and confirm a fully idle pane generates zero repaints over a multi-minute window (validates Phase 4's sync-bytes logic and Phase 5's push-only model).
4. **Cross-platform pass:** run the full suite on every OS you target. Expect Windows to need the most follow-up work (ConPTY quirks, no `/proc`-equivalent for CWD/port detection, named-pipe IPC instead of Unix sockets) — budget it as its own pass, not a side effect of the Unix work.
5. **Resource leak pass:** repeat Phase 9's orphan-process/thread check as a standing regression test, not a one-time check.

**Definition of done:** all of the above pass on every target platform, and you have a written note of any platform-specific gaps (e.g. "Windows dev-server port detection stubbed, falls back to regex-only") so they're tracked rather than silently missing.

---

## Quick Reference — Threading & Ownership Rules (keep this visible throughout)

- Raw PTY bytes and the VTE `Processor::advance()` call only ever happen on the reader thread.
- The `Term<Listener>` mutex (`FairMutex`) is locked briefly and never across blocking calls or paint passes.
- `Entity<T>` mutation happens only on the GPUI main thread, only from the batched event-drain in Phase 5, or from direct user-input handlers in Phase 7.
- Communication direction is one-way per channel: reader thread → `UnboundedSender<Event>` → GPUI; GPUI input handlers → `mpsc::Sender<Msg>` → writer thread. Don't create a third path that bypasses these.
- Platform-specific code (`#[cfg(target_os = ...)]`) should only appear at two seams: process shutdown, and CWD/port detection. If you find it spreading elsewhere, that's a sign the `PtyBackend` trait boundary has a leak.