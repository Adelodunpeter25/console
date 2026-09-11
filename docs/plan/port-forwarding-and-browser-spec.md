# Port Forwarding & Embedded Web Browser Specification

## 1. Executive Summary & Vision

Console enables seamless web development workflows by pairing an **Embedded Native Web Browser** (`BrowserView` in `apps/desktop`) with a **Server-Owned Port Forwarding Service** (`apps/server`).

When an autonomous agent or developer runs a web application (e.g., Next.js on `3000`, Vite on `5173`, Express on `8080`), Console:
1. **Detects dev ports owned by its own terminal/bash sessions** via per-session output scanning + a loopback probe. No host-wide socket scan.
2. **Exposes each dev port on its own server proxy port** (dumb TCP-level pipe, e.g. dev `5173` → proxy `45173`), so any browser just opens a URL and it works — desktop, remote, and mobile.
3. **Presents one-click previews directly inside Console**, eliminating context switching to external browser windows.

Non-goals for v1: SSH tunneling, path-prefix proxying (`/api/proxy/:port/*`), HTML/cookie rewriting, SSE, QR/LAN sharing, auth tokens.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Environment Server (apps/server - Bun / TypeScript)                                              │
│                                                                                                  │
│  ┌────────────────────────┐    ┌───────────────────────────┐    ┌─────────────────────────────┐  │
│  │ Session-Scoped         │    │ Dumb-Pipe Proxy Listeners │    │ Port Registry               │  │
│  │ Discovery              │    │ • :45173 → 127.0.0.1:5173 │    │ • 5173 -> proxy :45173    │  │
│  │ • PTY output regex     │    │ • :45174 → 127.0.0.1:3000 │    │ • probe-verified listening  │  │
│  │ • bash job output regex│    │ • HTTP passthrough        │    │ • client sees {port, url}   │  │
│  │ • loopback TCP probe   │    │ • WS frame relay (HMR)    │    │ • dies with owner session   │  │
│  └───────────┬────────────┘    └─────────────┬─────────────┘    └──────────────┬──────────────┘  │
└──────────────┼───────────────────────────────┼─────────────────────────────────┼─────────────────┘
               │ GET /api/ports                │ Plain HTTP/WS on proxy ports    │ poll every ~3s
               ▼                               ▼                                 ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Console Clients (Desktop Rust / GPUI + Mobile)                                                   │
│                                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ Right Sidebar Tab: [ Files ] [ Changes ] [ 🌐 Browser ] [ 📱 Devices ] [ Subagents ]        │  │
│  ├────────────────────────────────────────────────────────────────────────────────────────────┤  │
│  │ ┌────────────────────────────────────────────────────────────────────────────────────────┐ │  │
│  │ │ Active Ports: [ ● 3000 ] [ ● 5173 ] [ + Forward Port ]                         │ │  │
│  │ └────────────────────────────────────────────────────────────────────────────────────────┘ │  │
│  │ ┌────────────────────────────────────────────────────────────────────────────────────────┐ │  │
│  │ │ ◀  ▶  ↻  [ http://<server-host>:45173                                         ]  ↗     │ │  │
│  │ ├────────────────────────────────────────────────────────────────────────────────────────┤ │  │
│  │ │                                                                                        │ │  │
│  │ │                         Native Webview Surface (BrowserView)                           │ │  │
│  │ │                                                                                        │ │  │
│  │ └────────────────────────────────────────────────────────────────────────────────────────┘ │  │
│  └────────────────────────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Why one proxy port per dev port (not a path prefix)

`http://server:3000/api/proxy/5173/...` breaks absolute asset paths (`/@vite/client`, `/_next/static/...`) — the browser requests those at root, outside the prefix, unless we rewrite HTML. We refuse to rewrite HTML.

A dedicated proxy port means the app thinks it lives at root: absolute paths, cookies, CSP, redirects, and HMR websockets all work with zero app-aware logic. Cost: a proxy port range (default `45000–45999`) must be reachable instead of a single port.

---

## 2. Server Architecture (`apps/server`)

New file: `apps/server/api/src/services/port-registry.ts` (~100 lines). One map:

```ts
// Internal only — never leaves the server process.
interface PortEntry {
  port: number;            // dev port, e.g. 5173
  proxyPort: number;       // allocated from PROXY_PORT_RANGE, e.g. 45173
  terminalId?: string;     // owner PTY session (for lifecycle cleanup)
  jobId?: string;          // owner bash background job (for lifecycle cleanup)
  manual?: boolean;        // registered via POST /api/ports/forward, no owner session
}
```

The client only ever sees `{ port, url }`. Owner ids stay server-side,
used solely to delete entries when the owning session/job dies.

### A. Ownership Tagging (env injection at spawn)

Two spawn sites get a per-session env var (children like `vite` / `next dev` inherit it automatically):

- `api/src/terminal/pty.manager.ts` `startShell()` — `shellEnv()` becomes per-session:
  `{ ...baseEnv, CONSOLE_TERMINAL_ID: session.id }`
- `agent/src/tools/bash/manager.ts` `start()` — adds `CONSOLE_BASH_JOB_ID: jobId` to the job env.

Foreground `exec.ts` `spawnCapture()` spawns are ignored — a dev server never survives a short tool timeout; servers always come from a PTY or a background job.

The env var is an **identity tag only**. It does not detect ports; it labels who owns a candidate.

### B. Session-Scoped Detection (no host scan)

1. **Candidate regex** runs where output already funnels through:
   - PTY: `pty.manager.ts` `handleOutput()` (strip ANSI, buffer partial lines across chunks).
   - Bash jobs: `bash/manager.ts` `drain()`/`append()`.
   - Patterns: `http://localhost:PORT`, `http://127.0.0.1:PORT`, `http://0.0.0.0:PORT`, `Local:\s+http://localhost:PORT` (Vite/Next/Astro conventions). Also match bare `:PORT` forms printed by common dev servers where unambiguous.
2. **Verify with a probe, never trust print alone.** On candidate, TCP-connect (or `fetch(http://127.0.0.1:PORT/)`, 2s timeout). Only probed ports enter the registry. A URL printed before bind, or a crashed server, never advertises. No `starting`/`ready` states leak to clients — an entry is only registered (and only listed) once verified.

### C. Dumb-Pipe Proxy (one `Bun.serve` listener per dev port)

Managed by a small `proxy-manager` next to the registry (~100 lines). Per entry:

- **HTTP:** `fetch(http://127.0.0.1:<port> + path + query, { method, headers, body })`, stream the response body back. Set `Host: 127.0.0.1:<port>`, strip hop-by-hop headers. No HTML/cookie/CSP rewriting — nothing to rewrite at a dedicated root.
- **WebSocket:** on upgrade, open `new WebSocket('ws://127.0.0.1:<port>/path')` and relay frames both ways. No HMR-specific code; Vite HMR / Next Fast Refresh just flow.
- **Gate:** target is always the literal `127.0.0.1`; reject any port not in the registry; validate integer `1–65535`. That plus tag+probe ownership is the entire v1 trust model.

### D. Lifecycle (free, from existing tracking)

- PTY `kill`/exit → delete that session's entries, close their proxy listeners.
- Bash job finish/expire/kill → same.
- Server restart → registry starts empty; sessions re-detect on new output. Nothing persists.
- `DELETE /api/ports/:port` closes the listener and removes the entry. It revokes access — it never kills the dev process.

---

## 3. Wire Protocol & REST Endpoints

Proxy base: fully-qualified URLs built from the server's reachable host, e.g. `http://<server-host>:45173/`. Clients never construct proxy URLs themselves.

### 1. `GET /api/ports`

Returns only probe-verified, session-owned ports. Two fields per entry: `port`
(the real dev port, for display and terminal-click matching) and `url` (the
fully-qualified proxy URL to open — the only URL the client needs):

```json
{
  "success": true,
  "data": [
    { "port": 5173, "url": "http://192.168.1.10:45173/" },
    { "port": 3000, "url": "http://192.168.1.10:45174/" }
  ]
}
```

Clients poll every ~3s. No SSE in v1.

### 2. `POST /api/ports/forward` (manual, optional)

For ports the regex missed. Body: `{ "port": 8080 }`. Server probes `127.0.0.1:8080`; if listening, registers it as manually-owned and opens a proxy listener. Returns the created entry (`{ "port", "url" }`). If nothing is listening, `400`.

### 3. `DELETE /api/ports/:port`

Closes the proxy listener and removes the registry entry. Revokes preview access only.

---

## 4. Desktop Client Integration (`apps/desktop`)

`BrowserView`, `WebviewHost`, address bar, and shortcuts already exist — this phase only wires ports into them.

- **`console-core`**: new `Port` type + `PortService` (`GET /api/ports`, `POST`, `DELETE`) on `ConsoleClient`, polled every ~3s.
- **Ports chip bar** above the existing `BrowserView` toolbar: green dot + `[ ● 5173 ]` chips; click navigates the existing browser to that entry's `url`. `[ + ]` popover calls the manual forward endpoint.
- **Terminal URL clicks**: a localhost URL printed in `TerminalView` routes to the matching entry's `url` (open Browser tab + navigate) instead of the raw localhost address.
- **States**: listed means verified — no `starting` state on the client. Unreachable on click (dev server died between polls) → error toast + refresh. Multiple ports: most recently listed first.
- Shortcuts (`⌘L`, `⌘R`, `⌘[`/`⌘]`, `⌥⌘I`) already exist; add platform equivalents for Windows/Linux when that platform ships.

---

## 5. Mobile Client Integration (`apps/mobile`)

Nothing platform-specific. Same `GET /api/ports` list, same `url` opened in a WebView/link. No LAN URLs, no QR codes in v1 (those need a separate access design — out of scope).

---

## 6. Implementation Roadmap

### Phase 1: Registry + ownership tagging (server)
- [ ] Create `api/src/services/port-registry.ts` (`port → entry` map).
- [ ] Per-session `CONSOLE_TERMINAL_ID` in `pty.manager.ts` `startShell()`; `CONSOLE_BASH_JOB_ID` in `bash/manager.ts` `start()`.
- [ ] Regex candidate scan in `handleOutput()` + job `append()` (ANSI-strip, partial-line buffer).
- [ ] Loopback probe before registering; `GET /api/ports` + `DELETE /api/ports/:port`; 3s client polling contract.

### Phase 2: Dumb-pipe proxy (server)
- [ ] Proxy listener manager: allocate from `PROXY_PORT_RANGE` (default `45000–45999`), `Bun.serve({ fetch, websocket })` per entry.
- [ ] HTTP passthrough (method/path/query/body streaming, `Host` set, hop-by-hop strip).
- [ ] WS upgrade relay; test against Vite HMR + Next dev.
- [ ] Close listener on session/job death and on `DELETE`.

### Phase 3: Desktop + mobile wiring
- [ ] `console-core` `Port` type + `PortService`; 3s poll.
- [ ] Ports chip bar over existing `BrowserView`; click-to-navigate via `url`; `[ + ]` manual forward popover.
- [ ] Terminal localhost-click → `url` routing.
- [ ] Mobile ports list → open `url`.

### Explicitly out of v1
Path-prefix proxy, HTML/header rewriting, SSE, QR/LAN URLs, auth/capability tokens, `lsof`/`/proc/net/tcp` host scan.
