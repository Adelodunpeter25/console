# Port Forwarding SSE Streaming Architecture Plan

## 1. Overview & Problem Statement

Currently, the Console desktop client synchronizes forwarded ports by polling the
`GET /api/ports` HTTP endpoint every 2 seconds
(`apps/desktop/src/state/app.rs`, `init` background task).

This polling architecture creates several critical issues:
1. **Server-Side Probe Storm**: Every single `GET /api/ports` request calls
   `PortRegistry.list()`, which executes an asynchronous HTTP socket probe
   (`isListening()`) on every registered port. With multiple windows or clients,
   the backend is continuously probing internal ports every 2 seconds.
2. **Latency & Jitter**: When a development process (Vite, Next.js, Express,
   Flask) starts and emits a URL in the terminal, there is up to a 2-second
   delay before the port forward appears in the UI.
3. **Battery & CPU Drain**: Polling prevents the desktop client runtime
   (Tokio + GPUI) and server from idling.

## 2. Target Architecture

Replace the 2-second polling loop with a persistent **Server-Sent Events (SSE)**
stream: `GET /api/ports/stream`. This mirrors the existing `fs/watch`
(`apps/server/api/src/routes/fs.ts`) and `git/status/watch`
(`apps/server/api/src/routes/git.ts`) streams — same `streamSSE` lifecycle,
same `onAbort` cleanup, same 15s `ping` heartbeat.

```
  ┌───────────────────────┐                    ┌─────────────────────────┐
  │      PortRegistry     │                    │     Hono SSE Route      │
  │ (terminal/job/manual) ├────["change"]─────►│  GET /api/ports/stream  │
  └───────────────────────┘                    └────────────┬────────────┘
                                                             │
                                                      (SSE text/event-stream)
                                                             │
                                                             ▼
                                                ┌─────────────────────────┐
                                                │   Desktop Console-Core  │
                                                │  (eventsource + filter  │
                                                │   on event == "ports")  │
                                                └────────────┬────────────┘
                                                             │
                                                             ▼
                                                ┌─────────────────────────┐
                                                │  Desktop App State      │
                                                │  (TitleBar Popover UI)  │
                                                └─────────────────────────┘
```

### 2.1. Server-Side Specification (`apps/server`)

1. **`PortRegistry` Event Propagation
   (`apps/server/api/src/services/port-registry.service.ts`)**:
   - `PortRegistry` already extends `EventEmitter`, but today it only emits
     `"opened"` from `createEntry` — `remove()` and `removeOwner()` emit
     nothing. Add a single `"change"` event emitted from exactly **two**
     choke points, so every mutation path pushes exactly once:
     - `createEntry()` (covers `forward()` and `registerDetected()` —
       do NOT emit in the callers or pushes will double).
     - `remove()` (covers `removeOwner()`, manual unforward, and the silent
       dead-port cleanup inside `list()`).
   - Keep the `"opened"` event as-is for backward compatibility; the stream
     subscribes only to `"change"`.

2. **Cached snapshot — do NOT call probing `list()` on the hot path**:
   - `list()` is expensive by design: it `isListening()`-probes every
     registered port. Calling it on every `"change"` just moves the probe
     storm from per-2s-per-window to per-event.
   - Maintain a cached, already-projected list inside the registry
     (rebuilt in `createEntry`/`remove` at mutation time, when the entry set
     is known without probing). Liveness probing stays where it belongs: a
     slow background reaper (existing `list()` cleanup semantics), not the
     push path.
   - The `"change"` payload carries the delta (`{ added, removed }`) or the
     full cached snapshot; the stream handler forwards it without probing.

3. **SSE Streaming Endpoint (`apps/server/api/src/routes/ports.ts`)**:
   - Route: `GET /api/ports/stream` (registered **before** `/ports/:port`
     so `"stream"` is not parsed as a port param).
   - **Capture `host` once at connect time** from the `Host` header and reuse
     it for every push — the connection is long-lived, per-event header
     reads are wrong.
   - Lifecycle (copy `git.ts` `status/watch`, not a new shape):
     - **Subscribe first, then snapshot**: attach the `"change"` listener
       before sending the initial snapshot so a port opening in between is
       not lost (snapshot-then-push ordering guarantees the client converges).
     - **Initial snapshot**: push cached list as
       `event: "ports", data: JSON.stringify(ports)`.
     - **Update push**: on `"change"`, push the cached list
       (no `isListening()` probes on this path).
     - **Heartbeat**: `event: "ping"` with empty data every 15 seconds.
     - **Cleanup**: `stream.onAbort()` unsubscribes the listener.
   - **Project scoping**: the desktop client currently sends `?projectId=`,
     which the server ignores (all project keys hold the same global list).
     Either accept-and-honor `projectId` on the stream route or explicitly
     drop the param on both sides — do not bake the inconsistency into the
     new route. Recommended: drop it (ports are per-server, not per-project)
     and simplify `forwarded_ports_by_project` to a single list; if
     per-project filtering is wanted later, add it to both `list()` and the
     stream together.

### 2.2. Client-Side Specification (`apps/desktop`)

1. **Core Service API
   (`apps/desktop/crates/console-core/src/services/port.rs`)**:
   - Add
     `pub async fn watch(&self) -> Result<Pin<Box<dyn Stream<Item = Result<Vec<ForwardedPort>>> + Send>>>`
   - Connects to `/api/ports/stream` using `HttpTransport`.
   - **Do NOT use `SseStreamReader::parse_typed_stream`** — it JSON-parses
     *every* event's `data`, including the `ping` heartbeats with empty
     bodies, which would error every 15s. Use the `fs.rs::watch_events`
     pattern instead: raw `eventsource()` + `filter_map` keeping only
     `event.event == "ports"` and JSON-parsing those payloads, silently
     ignoring `ping` and anything else.

2. **State & Window Integration (`apps/desktop/src/state/`)**:
   - Remove the background `loop { timer(2s); fetch }` polling task in
     `state/app.rs` (`init`, ~line 1260).
   - In `state/port_forward.rs`, spawn a persistent stream task in the
     `notifications.rs` reconnect-loop shape (`loop { match watch() { Ok(stream)
     => drain, Err => log + sleep(3s) } }`):
     - On receiving a `ports` payload: update the forwarded-ports state and
       call `cx.notify()`. Keep the existing changed-check so identical
       snapshots don't re-render.
     - On disconnect / error: sleep with backoff and reconnect (match the
       3s notification-loop cadence unless a reason to differ appears).
   - **Store the stream task handle on app state** (same rule as
     `inspector_fs_watch` in `app.rs`: one live stream per target, replacing
     the handle on target change is what prevents duplicate-stream
     accumulation). On active environment change (`activate_environment`),
     drop the old handle and connect to the new server's
     `/api/ports/stream`, or old streams leak against the old server.
   - Keep one-shot `list()` as a fallback for servers without the stream
     route (fetch-then-stream, as `right_sidebar.rs` already does for
     fs/git watchers).

## 3. Step-by-Step Implementation Spec

### Step 1: Update Server Port Registry
- In `apps/server/api/src/services/port-registry.service.ts`:
  - Add cached snapshot state rebuilt in `createEntry` and `remove`.
  - Emit `this.emit("change")` from `createEntry` and `remove` only —
    not from `forward`, `registerDetected`, `removeOwner`, or `list`.

### Step 2: Implement SSE Endpoint
- In `apps/server/api/src/routes/ports.ts`:
  - Add `portRoutes.get("/ports/stream", (c) => streamSSE(c, async (stream) => { ... }))`
    **before** the `/ports/:port` route.
  - Subscribe → snapshot → push-on-change → 15s ping → `onAbort` unsubscribe.
  - Resolve the `projectId` question (recommendation: drop it both sides).

### Step 3: Add `console-core` Stream Client
- In `apps/desktop/crates/console-core/src/services/port.rs`:
  - Add `pub async fn watch(&self)` using the `fs.rs::watch_events`
    eventsource + `filter_map(event == "ports")` pattern.

### Step 4: Wire Stream in Desktop State
- In `apps/desktop/src/state/port_forward.rs`:
  - Add `init_port_stream` reconnect loop (notifications.rs shape).
- In `apps/desktop/src/state/app.rs`:
  - Delete the 2s polling block; initialize the port stream on app launch
    and replace the stored handle on environment switch.

## 4. Verification

1. **Server Unit Tests (`apps/server/tests/ports.test.ts`)**:
   - Test `GET /api/ports/stream` receives the initial ports array.
   - Test that calling `POST /api/ports/forward` causes a new SSE event to
     be emitted with the updated list.
   - Test that `DELETE /api/ports/:port` pushes a removal event (covers the
     previously silent `remove()` path).
   - Assert the push path performs zero `isListening()` probes (e.g. no
     outbound fetch attempts while pushing).
2. **Desktop Integration Verification**:
   - Start a local server (e.g. `bun run dev` on port 3000).
   - Verify the title-bar ports popover updates on the next pushed event
     after the dev server prints its URL (no 2s poll delay).
   - Verify zero `GET /api/ports` requests appear in server logs while idle.
   - Kill the dev server; verify a removal pushes with no polling involved.

## 5. Known Adjacent Limitation (Out of Scope)

- `isListening()` probes via HTTP `GET /` only, so non-HTTP ports (postgres,
  redis, raw TCP) can never register. SSE streaming doesn't change that;
  fixing it means a TCP-connect probe, tracked separately.
