# Port Forwarding SSE Streaming Architecture Plan

## 1. Overview & Problem Statement

Currently, Console desktop and mobile clients synchronize forwarded ports by periodically polling the `GET /api/ports` HTTP endpoint every 2 seconds (`apps/desktop/src/state/app.rs`).

This polling architecture creates several critical issues:
1. **Server-Side Probe Storm**: Every single `GET /api/ports` request calls `PortRegistry.list()`, which executes an asynchronous HTTP socket probe (`isListening()`) on every registered port. With multiple windows or clients, the backend is continuously probing internal ports every 2 seconds.
2. **Latency & Jitter**: When a development process (Vite, Next.js, Express, Flask) starts and emits a URL in the terminal, there is up to a 2-second delay before the port forward appears in the UI.
3. **Battery & CPU Drain**: Polling prevents the desktop client runtime (Tokio + GPUI) and server from idling.

## 2. Target Architecture

Replace the 2-second polling loop with a persistent **Server-Sent Events (SSE)** stream: `GET /api/ports/stream`.

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
                                               │   (SseStreamReader)     │
                                               └────────────┬────────────┘
                                                            │
                                                            ▼
                                               ┌─────────────────────────┐
                                               │  Desktop App State      │
                                               │  (TitleBar Popover UI)  │
                                               └─────────────────────────┘
```

### 2.1. Server-Side Specification (`apps/server`)

1. **`PortRegistry` Event Propagation (`port-registry.service.ts`)**:
   - `PortRegistry` already extends `EventEmitter`.
   - Emit `"change"` events whenever:
     - A new port is automatically detected from terminal output (`registerDetected`).
     - A port is manually forwarded (`forward`).
     - A port is removed or detected as closed (`remove`, `removeOwner`).

2. **SSE Streaming Endpoint (`apps/server/api/src/routes/ports.ts`)**:
   - Route: `GET /api/ports/stream`
   - Headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
   - Lifecycle:
     - **Initial Handshake**: On connection open, immediately query `portRegistry.list(host)` and push an initial snapshot event (`event: "ports"`).
     - **Delta / Update Push**: On `"change"` event from `PortRegistry`, query the updated list and push `event: "ports", data: JSON.stringify(ports)`.
     - **Heartbeat**: Emit `event: "ping"` every 15 seconds to keep intermediary proxies and routers alive.
     - **Cleanup**: Unsubscribe the listener when the client disconnects or aborts.

### 2.2. Client-Side Specification (`apps/desktop`)

1. **Core Service API (`console-core/src/services/port.rs`)**:
   - Add `pub async fn stream(&self) -> Result<Pin<Box<dyn Stream<Item = Result<Vec<ForwardedPort>>> + Send>>>`
   - Connects to `/api/ports/stream` using `HttpTransport` and parses incoming payloads with `SseStreamReader::parse_typed_stream`.

2. **State & Window Integration (`apps/desktop/src/state/`)**:
   - Remove the background `loop { timer(2s); fetch }` polling task in `state/app.rs`.
   - In `state/port_forward.rs` (or `state/app.rs`), spawn a persistent stream task:
     - On receiving a `ports` payload: update `forwarded_ports_by_project` and call `cx.notify()`.
     - On disconnect / error: back off and automatically reconnect with exponential jitter (e.g. 1s -> 3s -> 5s).
     - On active environment change (`activate_environment`): abort the existing stream and connect to the new server's `/api/ports/stream`.

## 3. Step-by-Step Implementation Spec

### Step 1: Update Server Port Registry
- In `apps/server/api/src/services/port-registry.service.ts`:
  - Ensure `this.emit("change")` fires on all mutations (`createEntry`, `remove`, `removeOwner`).

### Step 2: Implement SSE Endpoint
- In `apps/server/api/src/routes/ports.ts`:
  - Add `portRoutes.get("/ports/stream", (c) => streamSSE(c, async (stream) => { ... }))`.

### Step 3: Add `console-core` Stream Client
- In `apps/desktop/crates/console-core/src/services/port.rs`:
  - Add `pub async fn stream(&self)` method.

### Step 4: Wire Stream in Desktop State
- In `apps/desktop/src/state/port_forward.rs`:
  - Replace polling with `init_port_stream` task.
- In `apps/desktop/src/state/app.rs`:
  - Initialize the port stream on app launch and environment switch.

## 4. Verification

1. **Server Unit Tests (`apps/server/tests/ports.test.ts`)**:
   - Test `GET /api/ports/stream` receives the initial ports array.
   - Test that calling `POST /api/ports/forward` causes a new SSE event to be emitted with the updated list.
2. **Desktop Integration Verification**:
   - Start a local server (e.g. `bun run dev` on port 3000).
   - Verify that the title-bar ports popover updates instantly (0ms latency).
   - Verify zero background polling requests appear in server logs while idle.
