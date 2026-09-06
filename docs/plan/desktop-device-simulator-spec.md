# Desktop Architecture: Native iOS Simulator & Android Emulator Specification

## 1. Overview & Vision

Console Desktop is a high-performance native desktop application written in **Rust** using **GPUI** (backed by Metal on macOS). 

Rather than relying on web browser streaming (which introduces WebSocket framing, H.264 compression overhead, and WebCodecs latency), the native Rust desktop architecture allows us to achieve **ultra-low-latency, zero-copy display mirroring and direct input control** for both **iOS Simulators** and **Android Emulators / Physical Devices**.

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Window: Console Workspace                                                                        │
├───────────────┬────────────────────────────────────────┬─────────────────────────────────────────┤
│ Left Sidebar  │ Center Cockpit                         │ Right Sidebar (Device / Inspector)      │
│               │                                        │ ┌─────────────────────────────────────┐ │
│ • Today       │ ┌────────────────────────────────────┐ │ │ [Changes] [Files] [📱 Devices] [>_]  │ │
│   - Chat A    │ │ Active Agent Transcript            │ │ ├─────────────────────────────────────┤ │
│   - Chat B    │ │                                    │ │ │ Device: [ iPhone 16 Pro (Booted) ▾] │ │
│               │ │ "I have launched the iOS app and   │ │ ├─────────────────────────────────────┤ │
│ • Yesterday   │ │  tapped the login button."         │ │ │ ┌─────────────────────────────────┐ │ │
│   - Chat C    │ ├────────────────────────────────────┤ │ │ │                                 │ │ │
│               │ │ Composer                           │ │ │ │   📱 Live Interactive Device    │ │ │
│               │ │                                    │ │ │ │      Surface (Metal / GPUI)     │ │ │
│               │ └────────────────────────────────────┘ │ │ │ │                                 │ │ │
│               │                                        │ │ │ └─────────────────────────────────┘ │ │
│               │                                        │ │ │ [ ⌂ Home ] [ 🔒 Lock ] [ 📸 Snap ]  │ │
│               │                                        │ │ └─────────────────────────────────────┘ │ │
└───────────────┴────────────────────────────────────────┴─────────────────────────────────────────┘
```

---

## 2. Platform Ingestion Strategies

### A. iOS Simulator (macOS Native Pipeline)
On macOS, iOS Simulators run directly on the host kernel via `CoreSimulator`:
1. **Display Mirroring**:
   - **Zero-Copy Metal Texture Rendering**: The simulator's display framebuffer is backed by an `IOSurface`. In Rust on macOS, we can import the `IOSurfaceID` directly into a Metal texture (`MTLDevice.newTextureWithDescriptor:iosSurface:plane:`) and bind it directly to GPUI's Metal render pipeline.
   - **Zero CPU/GPU Encoding Overhead**: No H.264 encoding or decoding is required on macOS; the pixels are rendered directly from shared GPU memory.
   - **Alternative / Fallback**: `xcrun simctl io booted screenshot` or `idb video-stream` for headless frame capture.
2. **Input Injection & Lifecycle**:
   - Device discovery, boot, shutdown, app install, and app launch via `xcrun simctl`.
   - Touch events, swipes, and keystrokes via `idb_companion` or direct Indigo Mach HID event injection.

### B. Android Emulator & Physical Devices (Cross-Platform Pipeline)
1. **Display Mirroring**:
   - **`scrcpy` Native Protocol**: Run a background `adb` connection that pushes the lightweight `scrcpy-server.jar` to the Android device.
   - The device streams raw H.264/H.265 video packets over an ADB TCP forwarding socket.
   - **Rust Decoding**: Decode the stream in Rust using macOS hardware VideoToolbox (via `core-foundation` / `video-toolbox-sys`) or `ffmpeg-next`, then update GPUI's image texture.
2. **Input Injection & Lifecycle**:
   - Touch, multi-touch gestures, key events, and text typing sent as binary control packets over the `scrcpy` control socket.
   - Device discovery and APK deployment via `adb` (`adb devices`, `adb install -r`, `adb shell am start`).

---

## 3. Architecture & Module Design

```
apps/desktop/
├── crates/
│   ├── console-core/
│   │   └── src/services/device/
│   │       ├── mod.rs                # DeviceService facade & state machine
│   │       ├── manager.rs            # Lifecycle (discovery, boot, shutdown, active device)
│   │       ├── traits.rs             # DeviceBackend trait definition
│   │       ├── ios/
│   │       │   ├── simctl.rs         # xcrun simctl CLI & process controller
│   │       │   ├── iosurface.rs      # macOS IOSurface -> Metal texture bridge
│   │       │   └── idb_client.rs     # idb companion IPC / HID injector
│   │       └── android/
│   │           ├── adb.rs            # ADB process wrapper and discovery
│   │           ├── scrcpy_client.rs  # scrcpy protocol & control packet parser
│   │           └── decoder.rs        # Hardware video decoder pipeline
│   └── console-ui/
│       └── src/device/
│           ├── mod.rs                # Device panel export
│           ├── device_panel.rs       # GPUI right-sidebar tab panel
│           ├── device_viewport.rs    # Interactive Metal surface / render quad
│           ├── device_bezel.rs       # Hardware frame & buttons (Power, Volume, Home)
│           └── device_picker.rs      # Dropdown selector for available devices
```

### Core Abstractions (`console-core`)

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum DevicePlatform {
    IosSimulator,
    Android,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DeviceDescriptor {
    pub id: String,           // UDID or ADB Serial
    pub name: String,         // "iPhone 16 Pro", "Pixel 8"
    pub platform: DevicePlatform,
    pub state: DeviceState,   // Booted, Shutdown, Booting
    pub screen_width: u32,
    pub screen_height: u32,
}

#[async_trait]
pub trait DeviceBackend: Send + Sync {
    async fn attach(&mut self, id: &str) -> Result<()>;
    async fn detach(&mut self) -> Result<()>;
    async fn tap(&self, x: f32, y: f32) -> Result<()>;
    async fn swipe(&self, start: (f32, f32), end: (f32, f32), duration_ms: u32) -> Result<()>;
    async fn key(&self, key_code: u32) -> Result<()>;
    async fn send_text(&self, text: &str) -> Result<()>;
    async fn screenshot(&self) -> Result<Vec<u8>>;
    async fn describe_ui(&self) -> Result<serde_json::Value>;
}
```

---

## 4. GPUI Viewport & Interaction Model (`console-ui`)

1. **Aspect Ratio Preservation**:
   - The device viewport dynamically scales to fit the available right sidebar width while strictly maintaining the device's native screen aspect ratio.
2. **Coordinate Normalization**:
   - Pointer clicks and drag gestures inside the GPUI viewport are normalized to `(0.0..1.0)` space relative to the active display bounds, then mapped to actual device points/pixels before sending to the backend.
3. **Hardware Controls Bar**:
   - Bottom / Top rail buttons:
     - ⌂ **Home** (Cmd+Shift+H on iOS, Back/Home on Android)
     - 🔒 **Lock / Power**
     - 🔊 **Volume Up / Down**
     - 🔄 **Rotate Screen**
     - 📸 **Capture Screenshot into Chat Composer**
4. **Agent Concurrency Indicator**:
   - When an autonomous agent is driving the device (executing UI tests or navigating an app), a subtle visual badge ("Agent is interacting with device...") appears with an option for the user to pause or take over.

---

## 5. Phased Implementation Plan

### Phase 1: Device Discovery & Manager Foundation
- Implement `DeviceDescriptor` and `DeviceService` in `crates/console-core`.
- Implement `simctl.rs` for iOS discovery (`xcrun simctl list --json`) and boot/shutdown operations.
- Implement `adb.rs` for Android device discovery (`adb devices -l`).
- Add Device tab toggle in the right sidebar in `console-ui`.

### Phase 2: iOS Simulator Zero-Copy Surface Ingestion
- Implement macOS `IOSurface` binding and Metal texture integration in `console-core`.
- Connect Metal texture directly to a custom GPUI `RenderImage` element in `console-ui`.
- Implement HID event injection (click-to-tap, drag-to-swipe) via `idb` / `simctl`.

### Phase 3: Android `scrcpy` Ingestion & Video Decoder
- Integrate ADB socket tunnel and `scrcpy-server` deployment in `console-core`.
- Add hardware H.264 video frame decoder (VideoToolbox on macOS).
- Implement Android touch and key control packets.

### Phase 4: UI Refinement & Hardware Controls
- Build device bezel SVG styling and physical button actions (Home, Volume, Lock).
- Add device switcher dropdown with live status badges.
- Implement "Attach Screenshot to Composer" shortcut.

### Phase 5: Agent Tools & Autonomous UI Driving
- Expose device control tools to the Console agent engine (`apps/server`):
  - `device_tap(x, y)`
  - `device_type(text)`
  - `device_swipe(startX, startY, endX, endY)`
  - `device_screenshot()`
  - `device_describe_ui()`
- Provide end-to-end verification and performance benchmarks.

---

## 6. Verification & Performance Targets
- **iOS Display Latency**: < 16ms (60 FPS zero-copy Metal texture update).
- **Android Display Latency**: < 35ms (hardware decoded H.264 stream).
- **Memory Footprint**: < 60 MB additional RAM overhead in desktop process.
- **Reliability**: Seamless recovery when simulators reboot, app crashes occur, or cables disconnect.
