# Run Re-attach — Plan

Let clients subscribe to an agent run that is already in flight on the server, so a
dropped SSE connection (or re-entering a session) resumes realtime streaming instead
of falling back to static SQLite snapshots.

Background: after the node→bun migration, mobile SSE connections drop mid-run
(see `docs/bun-api-review.md` history; `Bun.serve` idleTimeout was fixed in
`9e110c2`, but silent sockets remain fragile and the client masks drops as aborts).
Today the only event consumer is whoever opened `POST /sessions/:id/run`; once that
pipe dies there is no way back in.

Key architectural facts that make this easy here:

- The server already persists every completed turn (`modelStreamEnd` →
  `appendMessage`) and tool result incrementally as events arrive — a late joiner
  never needs a full replay from run start.
- Mobile's `applyChatEvent` reducer converges from any event stream;
  `reconstructRuns` rebuilds runs from persisted history.
- Pending questions/permissions are keyed by `requestId` in `RunService`,
  fully transport-independent.

## Target scenario: cross-surface continuation

A user starts a chat on mobile; later they sit down at the desktop and open the
same session. The desktop must show the full persisted transcript **and** stream
the still-running agent in real time — not a stale SQLite snapshot. Conversely,
a run started on desktop must be readable live from mobile. Phase 1 makes this
possible (the attach route is device-agnostic); Phases 2–3 wire each client up
to it.

## Phase 1 — Server: pub/sub emit path + attach route

### Tasks

- [ ] **1.1 Event sequence numbers.** In `run.service.ts`, assign each emitted
  `AgentSessionEvent` a monotonic per-session sequence number (`seq`). Thread it
  alongside the event (wrapper type `{ seq, event }`) without changing the
  `AgentSessionEvent` shape consumed by persistence/notifications.

- [ ] **1.2 Refactor single-consumer loop into pump + subscribers.** Keep the
  `for await (const event of eventStream)` loop in `runAgentStreamInternal` as the
  sole consumer of the agent `EventStream`. Replace the direct `onEvent(event)`
  call with a broadcast to a subscriber set:
  `Map<sessionId, Set<Subscriber>>` where `Subscriber = { id, since?: number,
  write(event, seq): Promise<void> }`. Persistence, notification, and status
  updates stay in the pump exactly as today.

- [ ] **1.3 Bounded replay buffer.** Per active session, keep a ring buffer of the
  last N events (start with N=500). Coalesce consecutive `modelStreamPart` deltas:
  store an accumulating `{text, thinking}` snapshot instead of thousands of delta
  frames; on replay emit one synthetic `modelStreamPart` carrying the accumulated
  text. Clear buffer + subscriber set in the existing `finally` block when the run
  settles.

- [ ] **1.4 Attach endpoint.** New SSE route in `apps/server/api/src/routes/run.ts`:
  `GET /api/sessions/:id/run/stream?since=<seq>`.
  - No active run → 409 JSON (client falls back to loading messages).
  - Active run → register subscriber **before** reading the replay buffer (same
    synchronization as broadcast, so no event is lost between replay-read and
    live-subscribe), flush buffered events with `seq > since`, then stream live.
  - No `since` → skip replay entirely (client loads from SQLite first) and go live.
  - Each frame: `writeSSE({ id: String(seq), event: type, data })`.

- [ ] **1.5 Terminal semantics for attached clients.** When the run finishes or is
  aborted, deliver an explicit terminal frame to all subscribers
  (`event: "done"` / `"aborted"`), then close their streams. `/abort` keeps its
  current behavior (aborts for everyone); attached clients finalize on the
  terminal frame.

- [ ] **1.6 Heartbeat on idle.** In the pump, if no event has been broadcast for
  e.g. 15s, send an SSE comment (`:ping`) or lightweight `heartbeat` event to all
  subscribers of that session. Keeps intermediaries from reaping idle connections
  and gives clients liveness signal for reconnect logic.

- [ ] **1.7 Wire into app.ts** — mount the new route in the same group as
  `runRoutes`; add tests in `apps/server/tests/` following existing per-feature
  style (attach during active run, replay correctness, race between register and
  broadcast, terminal frames, no-active-run 409).

## Phase 2 — Mobile: reconnect + re-attach consumption

### Tasks

- [ ] **2.1 Fix error masking in `native-stream`.** In
  `apps/mobile/modules/native-stream/index.ts`, stop calling
  `onEnd(true)` after `onError`. Distinguish user-cancelled (`onEnd(true)`) from
  network failure (`onEnd(false)` + error surfaced). Update
  `chat-stream-runner.ts` so `hadError` is evaluated correctly and real failures
  render as error bubbles instead of silently "completing".

- [ ] **2.2 Sequence tracking.** Record the last seen `seq` per stream (parse the
  SSE `id:` field — the native Android reader currently ignores non-`data:` lines;
  extend it to capture `id:` lines and pass them through).

- [ ] **2.3 Reconnect with resume.** On unexpected stream error mid-run, retry by
  calling the attach endpoint with `since=<lastSeq>` using exponential backoff
  (e.g. 1s/2s/4s, 3 attempts). Finalize the run locally only when:
  - the server returns 409 (no active run), or
  - a terminal frame arrives, or
  - retries are exhausted.

- [ ] **2.4 Attach on session entry.** When entering a session whose server header
  reports `status === "working"` but no local native stream exists for it, load
  persisted messages first, then open the attach stream (no `since` — converge via
  events from now on). Set local `running=true` while attached so the composer
  stop button stays correct.

- [ ] **2.5 Unify running state.** Derive `running` from "local stream alive OR
  attached to a server run" rather than only the send-initiated stream. Ensure
  `finalizeSessionRun` fires exactly once per run (guard against double-finalize
  from reconnect paths).

- [ ] **2.6 Verify on device.**
  - Kill Wi-Fi briefly mid-run → reconnects, streaming resumes, no duplicated
    content.
  - Background the app during a long tool run → return → still realtime.
  - Abort from a second surface (if desktop attached) → both finalize.

## Phase 3 — Desktop re-attach (cross-surface continuation)

Goal: start on phone, continue live on desktop (and vice versa).

### Tasks

- [ ] **3.1 Attach endpoint in `console-core`.** Add an SSE-consuming service in
  `apps/desktop/crates/console-core/src/services/run.rs` mirroring the existing
  run service: connect to `GET /api/sessions/:id/run/stream?since=<seq>` and feed
  events into the same dispatch path used by locally-started runs.

- [ ] **3.2 Attach on session entry.** When the GPUI app opens a session whose
  server header reports `status === "working"` but no local run is active, load
  persisted history first (existing session service), then open the attach
  stream and converge via incoming events. Set the session's running/stop state
  as if the run were local so the stop button and working indicators behave
  correctly.

- [ ] **3.3 History-first convergence.** Ensure replaying events on top of loaded
  history cannot duplicate content: reuse the same reducer/guards the desktop
  applies to its own run events (`applyChatEvent` parity in Rust); ignore or
  merge events already reflected in persisted messages.

- [ ] **3.4 Terminal + abort handling.** On terminal frames (`done`/`aborted`),
  finalize the attached session exactly like a locally-started run (status flip,
  header refresh, query invalidation equivalent). `/abort` from the desktop must
  keep working against the server-side active run even though the desktop did
  not start it.

- [ ] **3.5 Verify cross-surface flows.**
  - Start a run on mobile → open desktop mid-run → transcript loads then streams
    live; run completes normally on both surfaces.
  - Same in reverse (desktop → mobile).
  - Abort from the non-initiating surface → both finalize.
  - Permission/question requests raised mid-run surface on the attached surface
    and can be answered there (requestId-keyed decisions make this work).

- [ ] **3.6 Focused polling of projects/sessions.** Mobile keeps its lists fresh
  via TanStack Query polling (`queries.ts`: `refetchInterval`, disabled in
  background). The desktop has no query layer (GPUI/Rust), so replicate the
  behavior with a simple poll loop:
  - On app focus (window focused for the first time): immediately refetch
    `/api/projects` and the active project's `/api/sessions`.
  - While focused: keep polling both endpoints every ~30s.
  - When unfocused/minimized: pause polling entirely (`refetchIntervalInBackground:
    false` parity).
  - On regaining focus: fire an immediate refresh, then resume the interval
    timer from zero (avoid a stale-gap where the 30s tick lands right after
    focus).
  - Reuse the fetched headers to seed/refresh session statuses (same role as
    mobile's `setStatuses`), so cross-surface status flips surface even without
    an attach stream open.

## Phase 4 — Hardening (follow-up)

- [ ] Multi-surface: allow simultaneous desktop+mobile attachment (falls out of
  the subscriber set naturally — verify no shared mutable state assumes one
  consumer).
- [ ] Metrics/logging: log attach counts, replay sizes, and reconnect reasons in
  daemon logs to diagnose future transport issues.
