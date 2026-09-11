# Console Device Architecture: Server-Hosted iOS Simulator & Android Emulator Specification

## 1. Overview & Vision

Console delivers full lifecycle control and interactive streaming for **iOS Simulators** and **Android Emulators / Physical Devices**.

Instead of embedding platform-specific capture and JNI/Metal bindings directly in the native client binaries, Console employs an **Environment Server-First Architecture** hosted entirely inside `apps/server` (Bun / TypeScript). 

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Console Server Subsystem (apps/server - Bun / TypeScript)                                        │
│                                                                                                  │
│  ┌─────────────────────────┐   ┌─────────────────────────────┐   ┌────────────────────────────┐  │
│  │ Platform Discovery      │   │ Callstack agent-device Core │   │ Media & Streaming Tunnel   │  │
│  │ • xcrun simctl list     │   │ • Semantic Accessibility    │   │ • WebSocket Stream (scrcpy)│  │
│  │ • adb & AVD Manager     │   │ • Selector Chains & Match   │   │ • Frame Buffering / MJPEG  │  │
│  │ • Diagnostic Wizard     │   │ • Metro Runtime Bridge      │   │ • Normalized Input Proxy   │  │
│  └────────────┬────────────┘   └──────────────┬──────────────┘   └─────────────┬──────────────┘  │
└───────────────┼───────────────────────────────┼────────────────────────────────┼─────────────────┘
                │ HTTP REST / JSON-RPC          │ Agent Tool Bus                 │ Streaming WebSocket
                ▼                               ▼                                ▼
┌────────────────────────────────┐ ┌─────────────────────────────┐ ┌────────────────────────────────┐
│ Desktop Client (Rust / GPUI)   │ │ Autonomous Agent Loop       │ │ Mobile Client (React Native)   │
│ • Interactive Stream Canvas    │ │ (Antigravity, Codex, Devin) │ │ • Live Timeline Screenshots    │
│ • Pointer / Touch Normalizer   │ │ • device_snapshot           │ │ • Approval Prompts & Steer     │
│ • Physical Hardware Buttons    │ │ • device_interact           │ │ • Remote Device Inspector      │
└────────────────────────────────┘ └─────────────────────────────┘ └────────────────────────────────┘
```

This ensures that:
1. **Multi-Client Parity**: **Desktop (GPUI)**, **Mobile (Expo / React Native)**, **Web**, and **Remote CLI/SSH servers** share the exact same simulator sessions and tools.
2. **Deterministic Agent Automation**: Autonomous agents interact via semantic accessibility trees and selector chains rather than brittle vision-only coordinate guessing.
3. **Zero Native C/Metal Client Bloat**: Desktop and mobile applications remain clean presentation surfaces that receive a standardized stream and dispatch normalized pointer events.

---

## 2. Key References & Upstream Blueprints

When implementing or extending this subsystem, consult these primary references:

1. **T3 Code Simulator & Emulator Architecture ([PR #10677](https://github.com/pingdotgg/t3code/pull/10677))**:
   - Environment server execution model, 3-step opt-in onboarding wizard (enable -> tool diagnostic -> agent grant).
   - Scoped loopback proxying, allowlisted routes, input permission boundaries, and cold AVD boot state machines.
2. **Callstack `agent-device` Framework ([GitHub Repository](https://github.com/callstack/agent-device))**:
   - Deterministic Node.js/Bun driver for iOS and Android devices, session leases, and app lifecycle management.
3. **Callstack `agent-device` Client API Documentation ([Documentation](https://oss.callstack.com/agent-device/docs/client-api))**:
   - `createAgentDeviceClient()`, `client.snapshot()` accessibility traversal, `parseSelectorChain()`, `client.batch.run()`, and `agent-device/metro` runtime bridges.

---

## 3. Server Architecture (`apps/server`)

The device orchestration layer lives in `apps/server/api/src/devices/` and `apps/server/agent/src/tools/devices/`.

### A. Core Engine: Callstack `agent-device`
The server instantiates an `AgentDeviceClient` instance per active session:
```ts
import { createAgentDeviceClient } from "agent-device";

export class DeviceManager {
  private client = createAgentDeviceClient({ session: "console-device-session" });

  async listDevices(): Promise<DeviceDescriptor[]> {
    // Queries xcrun simctl and adb for booted and available cold AVDs
  }

  async getSnapshot(sessionId: string): Promise<DeviceSnapshot> {
    const session = await this.client.sessions.get(sessionId);
    const snapshot = await session.snapshot({ screenshot: { scale: 0.5 } });
    return {
      tree: snapshot.nodes, // Semantic Accessibility Hierarchy
      screenshotBase64: snapshot.screenshot,
      activeApp: snapshot.foregroundApp,
    };
  }

  async interact(action: DeviceAction): Promise<ActionResult> {
    // Dispatches tap, type, swipe, or keypress via agent-device
  }
}
```

### B. Streaming & Media Ingestion
1. **Android**: Server launches `scrcpy-server.jar` via `adb forward` tunnel. Video packets are decoded/demuxed on the server and piped to connected clients over a binary WebSocket connection.
2. **iOS**: Server captures simulator framebuffer via `idb video-stream` or native `simctl io booted stream` frame buffers, broadcasting frames to connected client subscribers.
3. **Loopback Auth Proxy**: Input sockets and stream tuning require explicit session authorization (`operate` scope), preventing unauthorized external processes from driving the local device.

---

## 4. API & Wire Protocol Specification

### REST Endpoints
| Method | Route | Description |
| :--- | :--- | :--- |
| `GET` | `/api/devices` | Lists all active simulators, physical devices, and stopped Android AVDs. |
| `GET` | `/api/devices/diagnostics` | Runs the 3-step setup check (Xcode, Android SDK, KVM, disk space). |
| `POST` | `/api/devices/:id/boot` | Boots a specified iOS Simulator or Android AVD. |
| `POST` | `/api/devices/:id/shutdown` | Powers off the specified device. |
| `POST` | `/api/devices/:id/open-app` | Installs or launches a target bundle/package (`client.apps.open`). |
| `POST` | `/api/devices/:id/interact` | Executes a manual tap, swipe, keystroke, or text entry. |
| `GET` | `/api/devices/:id/snapshot` | Returns the accessibility tree hierarchy and screenshot. |

### WebSocket Endpoint (`/api/devices/:id/stream`)
- **Downstream (Server → Client)**:
  - `FrameHeader { width, height, timestamp, format: "jpeg" | "h264" }` + Binary Frame Payload.
  - `DeviceStatusMessage { state: "booting" | "ready" | "error", details?: string }`.
- **Upstream (Client → Server)**:
  - `PointerEvent { type: "down" | "move" | "up", x: 0.0..1.0, y: 0.0..1.0, button: number }`.
  - `KeyEvent { type: "press", key: string, modifiers: string[] }`.
  - `HardwareButtonEvent { button: "home" | "back" | "volume_up" | "volume_down" | "power" }`.

---

## 5. Onboarding Diagnostics & Permission Model

Following the T3 Code blueprint, opening the Device panel triggers a non-blocking diagnostic wizard:

```
┌─────────────────────────────────────────────────────────────┐
│ 📱 Device Setup & Diagnostics                               │
├─────────────────────────────────────────────────────────────┤
│ 1. Platform Tools Check                                     │
│    ✓ Xcode CLI (xcrun simctl) installed                     │
│    ✓ Android SDK (adb, emulator) found in PATH              │
│    ✓ Disk Space: 48 GB available (Minimum 10 GB required)   │
│                                                             │
│ 2. Available Virtual Devices                                │
│    • iOS: iPhone 16 Pro (iOS 18.0) - Booted                 │
│    • iOS: iPad Air 13-inch (M2) - Stopped                   │
│    • Android: Pixel_8_API_34 - Stopped                      │
│                                                             │
│ 3. Autonomous Agent Permissions                             │
│    [x] Allow agent to capture screenshots and UI snapshots   │
│    [ ] Allow agent to automatically tap and type into device │
│                                                             │
│ [ Cancel ]                                   [ Enable Device ] │
└─────────────────────────────────────────────────────────────┘
```

- **Safe Failure Feedback**: Clear error explanations when Xcode tools or ADB are missing, with actionable install hints.
- **View Close vs. Power Off**: Closing a tab in the desktop/web client disconnects the stream; shutting down the simulator is an explicit power-off button.

---

## 6. Agent Tools Specification

The following tools are registered with Console’s agent system (`apps/server/agent/src/tools/`):

### 1. `device_list`
Lists running and available devices with platform metadata and state.

### 2. `device_open`
Boots a device or launches a specific app target / deep link.
```json
{
  "deviceId": "iPhone-16-Pro-UUID",
  "app": "com.example.myapp"
}
```

### 3. `device_snapshot`
Returns the foreground accessibility hierarchy and visual screenshot.
- Returns semantic nodes: `[{ "id": "btn_login", "label": "Log In", "type": "Button", "rect": [100, 450, 200, 50], "editable": false }]`.

### 4. `device_interact`
Executes pointer or keyboard interaction using either semantic selectors or coordinates:
```json
{
  "deviceId": "Pixel_8_API_34",
  "action": "tap",
  "selector": "label:Log In",
  "fallbackCoordinates": [0.5, 0.65]
}
```

### 5. `device_batch`
Executes an atomic multi-step interaction macro (`agent-device/batch`) to reduce LLM roundtrip latency:
```json
{
  "deviceId": "Pixel_8_API_34",
  "steps": [
    { "command": "tap", "input": { "selector": "id:username_field" } },
    { "command": "type", "input": { "text": "testuser@console.sh" } },
    { "command": "tap", "input": { "selector": "id:password_field" } },
    { "command": "type", "input": { "text": "secret123" } },
    { "command": "tap", "input": { "selector": "id:login_button" } }
  ]
}
```

---

## 7. Client Implementations

### A. Desktop Client (`apps/desktop` - GPUI / Rust)
- **Component**: `DeviceViewer` in `crates/console-ui/src/devices/`.
- **Display**: Pure GPUI element rendering incoming JPEG/RGBA frames decoded from WebSocket into an `Image` or texture surface.
- **Input**: GPUI `on_mouse_down`, `on_mouse_move`, `on_mouse_up` converts local coordinates to normalized `(0.0..1.0)` space and sends JSON payloads over WebSocket.
- **Hardware Rail**: Physical buttons on the right/bottom bezel for Home, Back, Volume, Power, and "Snap to Composer".

### B. Mobile Client (`apps/mobile` - Expo / React Native)
- **Timeline Cards**: Renders inline snapshots of agent UI interactions inside the chat transcript.
- **Remote Viewer**: Optional live WebSocket stream viewer using standard React Native image frames or WebRTC stream.

---

## 8. Implementation Roadmap

### Phase 1: Server Device Core & Diagnostics
- [ ] Add `agent-device` dependency to `apps/server/package.json`.
- [ ] Implement `DeviceService` in `apps/server/api/src/services/device.service.ts`.
- [ ] Implement diagnostics endpoint (`GET /api/devices/diagnostics`) checking `simctl`, `adb`, and disk space.
- [ ] Build `device_list`, `device_open`, `device_snapshot`, `device_interact`, and `device_batch` agent tools.

### Phase 2: Streaming & Input Pipeline
- [ ] Implement WebSocket media stream server in `apps/server/api/src/devices/stream.route.ts`.
- [ ] Wire `scrcpy-server` process supervisor for Android devices.
- [ ] Wire `simctl io stream` / `idb` frame capture for iOS Simulators.
- [ ] Implement normalized coordinate transformer and event dispatcher.

### Phase 3: Desktop UI Integration (`console-ui`)
- [ ] Add **📱 Devices** tab to the right sidebar in `console-ui`.
- [ ] Implement device switcher dropdown, status pill (Booting / Ready), and hardware rail buttons.
- [ ] Connect WebSocket client in `console-core` to stream live frames to GPUI viewport.
- [ ] Add "Send Screenshot to Composer" button.

### Phase 4: Metro & React Native Bridge
- [ ] Integrate `agent-device/metro` to detect active Metro bundlers on `http://localhost:8081`.
- [ ] Expose "Fast Refresh" and "Reload Bundle" action buttons in the device toolbar.
