# Port Forwarding & Embedded Web Browser Specification

## 1. Executive Summary & Vision

Console enables seamless web development workflows by pairing an **Embedded Native Web Browser** (`BrowserView` in `apps/desktop`) with a **Server-Hosted Port Forwarding & Proxy Service** (`apps/server`).

When an autonomous agent or developer runs a web application (e.g., Next.js on `3000`, Vite on `5173`, Express on `8080`), Console:
1. **Automatically detects listening dev ports** from terminal streams and host listening sockets.
2. **Proxies traffic over an authenticated tunnel** (`/api/proxy/:port/*`), enabling previewing across local desktops, remote cloud environments, and mobile clients.
3. **Presents one-click previews directly inside Console**, eliminating context switching to external browser windows.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Environment Server (apps/server - Bun / TypeScript)                                              │
│                                                                                                  │
│  ┌────────────────────────┐    ┌───────────────────────────┐    ┌─────────────────────────────┐  │
│  │ Port Discovery Service │    │ HTTP & WS Reverse Proxy   │    │ Dev Server Port Registry    │  │
│  │ • Process socket scan  │    │ • /api/proxy/:port/*      │    │ • 3000 -> Next.js / React   │  │
│  │ • Terminal output regex│    │ • WebSocket upgrade tunnel│    │ • 5173 -> Vite (HMR)        │  │
│  │ • Manual forwarded set │    │ • Header rewriting (host) │    │ • 8080 -> Backend / API     │  │
│  └───────────┬────────────┘    └─────────────┬─────────────┘    └──────────────┬──────────────┘  │
└──────────────┼───────────────────────────────┼─────────────────────────────────┼─────────────────┘
               │ GET /api/ports                │ Streamed Web Content            │ Real-time Port Events
               ▼                               ▼                                 ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Console Desktop Client (Rust / GPUI)                                                             │
│                                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ Right Sidebar Tab: [ Files ] [ Changes ] [ 🌐 Browser ] [ 📱 Devices ] [ Subagents ]        │  │
│  ├────────────────────────────────────────────────────────────────────────────────────────────┤  │
│  │ ┌────────────────────────────────────────────────────────────────────────────────────────┐ │  │
│  │ │ Active Ports: [ 3000 (Next.js) ▾ ] [ 5173 (Vite) ] [ + Forward Port ]                   │ │  │
│  │ └────────────────────────────────────────────────────────────────────────────────────────┘ │  │
│  │ ┌────────────────────────────────────────────────────────────────────────────────────────┐ │  │
│  │ │ ◀  ▶  ↻  [ 🔒 http://localhost:3000                                           ]  ↗     │ │  │
│  │ ├────────────────────────────────────────────────────────────────────────────────────────┤ │  │
│  │ │                                                                                        │ │  │
│  │ │                         Native Webview Surface (BrowserView)                           │ │  │
│  │ │                                                                                        │ │  │
│  │ └────────────────────────────────────────────────────────────────────────────────────────┘ │  │
│  └────────────────────────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Server Architecture (`apps/server`)

The port forwarding subsystem is located in `apps/server/api/src/services/port-forward.service.ts` and `apps/server/api/src/routes/ports.ts`.

### A. Dual-Engine Port Discovery

1. **Terminal Output Scanner (Passive)**:
   - Terminal PTY output and bash tool executions are scanned in real-time via regex for common server bind patterns:
     - `http://localhost:([0-9]{4,5})`
     - `http://127.0.0.1:([0-9]{4,5})`
     - `http://0.0.0.0:([0-9]{4,5})`
     - `Local:\s+http://localhost:([0-9]{4,5})` (Vite, Next.js, Astro conventions)
   - When detected, the port is registered immediately with a label inferred from the command (e.g. `npm run dev` → `Next.js / Vite`).

2. **System Socket Inspector (Active)**:
   - A background poller scans listening TCP sockets for the workspace process tree using:
     - macOS / Linux: `lsof -iTCP -sTCP:LISTEN -P -n` or `/proc/net/tcp`.
   - Distinguishes Console's internal server port (`3000`) from user-spawned child dev servers.
   - Detects when dev servers terminate and cleanly prunes stale ports.

### B. HTTP & WebSocket Reverse Proxy (`/api/proxy/:port/*`)

For remote environments (SSH, containers, cloud dev machines) and local sandbox isolation, Console proxies web application traffic:
- **HTTP Routing**:
  - `GET /api/proxy/:port/path` → `http://127.0.0.1::port/path`
  - Rewrites `Host`, `Origin`, and `Referer` headers to match the target local dev server.
- **WebSocket Upgrade (HMR)**:
  - Intercepts WebSocket upgrade requests at `/api/proxy/:port/*` and pipes frames directly to the dev server's WebSocket port (e.g. Vite HMR, Next.js Fast Refresh).
- **Security & Scope Gating**:
  - Only loopback ports (`127.0.0.1`) spawned by child processes or explicitly forwarded by the user are accessible.

---

## 3. Wire Protocol & REST Endpoints

### 1. `GET /api/ports`
Returns the list of currently listening and forwarded ports:
```json
{
  "success": true,
  "data": [
    {
      "port": 3000,
      "processName": "node",
      "label": "Next.js Dev Server",
      "url": "http://localhost:3000",
      "proxyUrl": "/api/proxy/3000/",
      "isAutoDetected": true,
      "createdAt": "2026-09-11T12:00:00.000Z"
    },
    {
      "port": 5173,
      "processName": "vite",
      "label": "Vite Frontend",
      "url": "http://localhost:5173",
      "proxyUrl": "/api/proxy/5173/",
      "isAutoDetected": true,
      "createdAt": "2026-09-11T12:01:00.000Z"
    }
  ]
}
```

### 2. `POST /api/ports/forward`
Manually forwards a port:
```json
{
  "port": 8080,
  "label": "Backend API"
}
```

### 3. `DELETE /api/ports/:port`
Stops forwarding a port and removes it from the active registry.

### 4. `GET /api/ports/events` (SSE Stream)
Emits real-time updates when ports open or close:
```json
event: port_opened
data: { "port": 5173, "label": "Vite", "url": "http://localhost:5173" }

event: port_closed
data: { "port": 5173 }
```

---

## 4. Desktop Client Integration (`apps/desktop`)

### A. Embedded `BrowserView` Mounting
- The native browser is mounted as the **`Browser`** tab inside `RightSidebar` (`InspectorTab::Browser`).
- Backed by `WebviewHost` (`WKWebView` on macOS, WebView2 on Windows via Wry/GPUI).
- Includes keyboard shortcuts: `⌘L` (focus address), `⌘R` (reload), `⌘[` / `⌘]` (back/forward), and `⌥⌘I` (Web Inspector / DevTools).

### B. Forwarded Ports Quick-Bar
Directly above the `BrowserView` navigation toolbar, an active ports chip-bar displays:
- Green indicator dot for active listening ports.
- Chip for each active port: `[ ● 3000 (Next.js) ]`, `[ ● 5173 (Vite) ]`.
- Clicking a port chip immediately navigates `BrowserView` to that address.
- `[ + ]` button opens a popover to manually enter any custom port.

### C. Clickable URLs in Terminal
When the terminal prints a localhost URL (e.g. `Local: http://localhost:5173/`), clicking the URL automatically switches the Right Sidebar to the **Browser** tab and navigates directly to that address.

---

## 5. Mobile Client Integration (`apps/mobile`)

1. **Remote Previews**:
   - Mobile client queries `GET /api/ports` to display active web applications.
   - Users can open the web app preview in their mobile browser using the proxy route or local network IP.
2. **QR Code Sharing**:
   - Console Desktop and Mobile generate a local LAN QR code for any forwarded port to test responsive designs on physical phones.

---

## 6. Implementation Roadmap

### Phase 1: Server Port Discovery & Proxy (Current Focus)
- [ ] Implement `PortForwardService` in `apps/server/api/src/services/port-forward.service.ts`.
- [ ] Add terminal output regex detection to auto-register open dev servers.
- [ ] Add `/proc/net/tcp` or `lsof` socket poller to detect active listening ports.
- [ ] Create `/api/ports` REST endpoints and `/api/proxy/:port/*` reverse proxy middleware.

### Phase 2: Desktop UI Ports Bar (`console-ui`)
- [ ] Mount active ports chip bar above `BrowserView` toolbar in `crates/console-ui/src/browser/view.rs`.
- [ ] Fetch ports from `console-core` and wire click-to-navigate action.
- [ ] Add manual "Forward Port" popover dialog.
- [ ] Add clickable localhost link routing from `TerminalView` to `BrowserView`.

### Phase 3: Live Sync & Real-time Notifications
- [ ] Connect SSE stream in `console-core` to invalidate port state on port open/close.
- [ ] Show subtle notification when a new dev server starts ("Port 3000 is running → Preview").
