# Console Device Architecture: Server-Hosted iOS Simulator & Android Emulator Specification

## 1. Overview & Vision

Console delivers full lifecycle control and interactive streaming for **iOS Simulators** and **Android Emulators / Physical Devices**.

Instead of embedding platform-specific capture and JNI/Metal bindings directly in the native client binaries, Console employs an **Environment Server-First Architecture** hosted entirely inside `apps/server` (Bun / TypeScript). 

> **Important Usage Note**:
> **The primary, immediate purpose of this subsystem is direct interactive use by the human developer.**
> The initial implementation focuses 100% on providing a rock-solid, low-latency device viewport right inside Console Desktop (and Mobile), enabling the developer to boot devices, see live screen updates, and interact directly (click, swipe, type, hardware buttons).
>
> **Autonomous Agent Tool Integration (`device_interact`, `device_batch`, etc.) is explicitly DEFERRED** until the core server streaming pipeline, lifecycle management, and Desktop UI interaction are fully implemented, verified, and stabilized.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Console Server Subsystem (apps/server - Bun / TypeScript)                                        │
│                                                                                                  │
│  ┌─────────────────────────┐   ┌─────────────────────────────┐   ┌────────────────────────────┐  │
│  │ Platform Discovery      │   │ Callstack agent-device Core │   │ Media & Streaming Tunnel   │  │
│  │ • xcrun simctl list     │   │ • Device Lifecycle & Leases │   │ • WebSocket Stream (scrcpy)│  │
│  │ • adb & AVD Manager     │   │ • Metro Runtime Bridge      │   │ • Frame Buffering / MJPEG  │  │
│  │ • Diagnostic Wizard     │   │ • (Agent Trees - Deferred)  │   │ • Normalized Input Proxy   │  │
│  └────────────┬────────────┘   └──────────────┬──────────────┘   └─────────────┬──────────────┘  │
└───────────────┼───────────────────────────────┼────────────────────────────────┼─────────────────┘
                │ HTTP REST / JSON-RPC          │ (Deferred Agent Tools)         │ Streaming WebSocket
                ▼                               ▼                                ▼
┌────────────────────────────────┐ ┌─────────────────────────────┐ ┌────────────────────────────────┐
│ Desktop Client (Rust / GPUI)   │ │ Autonomous Agent Loop       │ │ Mobile Client (React Native)   │
│ • Interactive Stream Canvas    │ │ (DEFERRED TO PHASE 4)       │ │ • Live Remote Stream Viewer    │
│ • Pointer / Touch Normalizer   │ │ • Future: device_interact   │ │ • Device State Inspector       │
│ • Physical Hardware Buttons    │ │ • Future: device_batch      │ │ • Hardware Actions             │
│ • [PRIMARY INTERFACE]          │ │                             │ │                                │
└────────────────────────────────┘ └─────────────────────────────┘ └────────────────────────────────┘
```

This ensures that:
1. **Developer-First Live Interaction**: The user can test their mobile apps side-by-side with code and chat in the Console workspace.
2. **Multi-Client Parity**: **Desktop (GPUI)**, **Mobile (Expo / React Native)**, **Web**, and **Remote CLI/SSH servers** share the exact same simulator sessions.
3. **Zero Native C/Metal Client Bloat**: Desktop and mobile applications remain clean presentation surfaces that receive a standardized stream and dispatch normalized pointer events.

---

## 2. Key References & Upstream Blueprints

When implementing or extending this subsystem, consult these primary references:

1. **T3 Code Simulator & Emulator Architecture ([PR #10677](https://github.com/pingdotgg/t3code/pull/10677))**:
   - Environment server execution model, 3-step opt-in onboarding wizard (enable -> tool diagnostic -> agent grant).
   - Scoped loopback proxying, allowlisted routes, input permission boundaries, and cold AVD boot state machines.
2. **Callstack `agent-device` Framework ([GitHub Repository](https://github.com/callstack/agent-device))**:
   - Deterministic Node.js/Bun driver for iOS and Android devices, session leases, Metro bridges, and app lifecycle management.
3. **Callstack `agent-device` Client API Documentation ([Documentation](https://oss.callstack.com/agent-device/docs/client-api))**:
   - `createAgentDeviceClient()`, `client.sessions.open()`, `client.apps.installFromSource()`, and `agent-device/metro` runtime bridges.

---

## 3. Server Architecture (`apps/server`)

The device orchestration layer lives in `apps/server/api/src/devices/`.

### A. Core Engine: Callstack `agent-device`
The server instantiates an `AgentDeviceClient` instance per active session:
```ts
import { createAgentDeviceClient } from "agent-device";

export class DeviceManager {
  private client = createAgentDeviceClient({ session: "console-device-session" });

  async listDevices(): Promise<DeviceDescriptor[]> {
    // Queries xcrun simctl and adb for booted and available cold AVDs
  }

  async bootDevice(deviceId: string): Promise<void> {
    // Boots the iOS Simulator or Android AVD
  }

  async interact(action: DeviceAction): Promise<ActionResult> {
    // Dispatches user pointer taps, swipes, text entry, or keypresses
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
| `POST` | `/api/devices/:id/interact` | Executes a user tap, swipe, keystroke, or text entry. |
| `GET` | `/api/devices/:id/screenshot` | Captures a live screenshot image. |

### WebSocket Endpoint (`/api/devices/:id/stream`)
- **Downstream (Server → Client)**:
  - `FrameHeader { width, height, timestamp, format: "jpeg" | "h264" }` + Binary Frame Payload.
  - `DeviceStatusMessage { state: "booting" | "ready" | "error", details?: string }`.
- **Upstream (Client → Server)**:
  - `PointerEvent { type: "down" | "move" | "up", x: 0.0..1.0, y: 0.0..1.0, button: number }`.
  - `KeyEvent { type: "press", key: string, modifiers: string[] }`.
  - `HardwareButtonEvent { button: "home" | "back" | "volume_up" | "volume_down" | "power" }`.

---

## 5. Onboarding Diagnostics & Setup Flow

Following the T3 Code blueprint, opening the Device panel triggers a non-blocking diagnostic check:

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
│ [ Cancel ]                                   [ Open Device ] │
└─────────────────────────────────────────────────────────────┘
```

- **Safe Failure Feedback**: Clear error explanations when Xcode tools or ADB are missing, with actionable install hints.
- **View Close vs. Power Off**: Closing a tab in the desktop/web client disconnects the stream; shutting down the simulator is an explicit power-off button.

---

## 6. Primary Client Implementation: Desktop Viewport (`console-ui`)

### Desktop Viewport (`apps/desktop` - GPUI / Rust)
- **Component**: `DeviceViewer` in `crates/console-ui/src/devices/`.
- **Display**: Pure GPUI element rendering incoming JPEG/RGBA frames decoded from WebSocket into an `Image` or texture surface.
- **Aspect Ratio Preservation**: Viewport scales dynamically to fit the right sidebar width while maintaining the device's native screen aspect ratio.
- **Direct Interaction**: GPUI `on_mouse_down`, `on_mouse_move`, `on_mouse_up` converts local coordinates to normalized `(0.0..1.0)` space and sends JSON payloads over WebSocket to the server.
- **Hardware Controls Rail**: Physical buttons on the right/bottom bezel:
  - ⌂ **Home** (Cmd+Shift+H on iOS, Back/Home on Android)
  - 🔒 **Lock / Power**
  - 🔊 **Volume Up / Down**
  - 🔄 **Rotate Screen**
  - 📸 **Capture Screenshot into Chat Composer**

---

## 7. Deferred: Autonomous Agent Tools (Phase 4)

*Note: The following tools will be implemented ONLY after the human-interactive server and desktop pipelines are fully verified and operational.*

When enabled in Phase 4, the server will expose:
1. `device_list`: Returns running and available devices with platform metadata.
2. `device_open`: Boots a device or launches an app target / deep link.
3. `device_snapshot`: Returns the foreground accessibility tree and visual screenshot.
4. `device_interact`: Executes pointer or keyboard interaction using semantic selectors or coordinates.
5. `device_batch`: Runs an atomic multi-step interaction macro (`agent-device/batch`).
6. `device_close`: Detaches view or shuts down device.

---

## 8. Phased Implementation Roadmap

### Phase 1: Server Device Core & Diagnostics (Current Focus)
- [ ] Add `agent-device` dependency to `apps/server/package.json`.
- [ ] Implement `DeviceService` in `apps/server/api/src/services/device.service.ts` for device discovery and lifecycle.
- [ ] Implement diagnostics endpoint (`GET /api/devices/diagnostics`) checking `simctl`, `adb`, and disk space.

### Phase 2: Server Streaming & Input Pipeline (Current Focus)
- [ ] Implement WebSocket media stream server in `apps/server/api/src/devices/stream.route.ts`.
- [ ] Wire `scrcpy-server` process supervisor for Android devices.
- [ ] Wire `simctl io stream` / `idb` frame capture for iOS Simulators.
- [ ] Implement normalized coordinate transformer and event dispatcher for user clicks and typing.

### Phase 3: Desktop UI Integration (`console-ui`) (Current Focus)
- [ ] Add **📱 Devices** tab to the right sidebar in `console-ui`.
- [ ] Implement device switcher dropdown, status pill (Booting / Ready), and hardware rail buttons.
- [ ] Connect WebSocket client in `console-core` to stream live frames into the GPUI viewport.
- [ ] Wire user mouse click, drag, and keyboard listeners to forward interaction events to the server.
- [ ] Add "Send Screenshot to Composer" button.

### Phase 4: Metro Bridge & Agent Tool Integration (Deferred)
- [ ] Integrate `agent-device/metro` to detect active Metro bundlers on `http://localhost:8081`.
- [ ] Implement autonomous agent tools (`device_snapshot`, `device_interact`, `device_batch`) in `apps/server/agent/src/tools/devices/`.
