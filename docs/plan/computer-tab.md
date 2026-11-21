# Computer Tab & Remote Desktop Streaming Plan

## Overview
Enable a full "Computer" tab in Console (Desktop and Android mobile apps) to view and control the remote host machine (Linux headless server with Xvfb/X11 or macOS with ScreenCaptureKit).

Following the proven architecture of `iroh-go` (`github.com/Adelodunpeter25/iroh-go`), we fork `tembo/voxl` into a dedicated Go FFI module (`voxl-go`) that pre-builds native static/shared libraries via GitHub Actions. Console's `server-go` links against `voxl-go` without compiling Rust or C on the server or developer workstations.

## Architecture

### 1. `voxl-go` (Forked from `tembo/voxl`)
- Expose a C-ABI layer (`voxl_ffi`) with functions:
  - `voxl_session_create(config)`
  - `voxl_session_start(session, frame_callback, user_data)`
  - `voxl_session_send_mouse(session, x, y, button_mask, wheel)`
  - `voxl_session_send_key(session, keysym, down)`
  - `voxl_session_resize(session, width, height)`
  - `voxl_session_stop(session)`
  - `voxl_session_destroy(session)`
- GitHub Actions CI workflow compiles libraries for:
  - `x86_64-unknown-linux-gnu`
  - `aarch64-unknown-linux-gnu`
  - `aarch64-apple-darwin`
  - `x86_64-apple-darwin`
- Ships precompiled binaries under `libs/<target>/` with Cgo bindings in `voxl.go` / `libs.go`.

### 2. `server-go` Backend Integration
- Manage virtual display lifecycle on Linux:
  - Ensure `Xvfb :99` is active (e.g. `-screen 0 1920x1080x24 -shmem`).
  - Spawn lightweight desktop window manager (e.g. `openbox`).
- WebSocket route `/api/computer/stream`:
  - Outgoing: H.264 NAL units pushed as binary WebSocket frames.
  - Incoming: Mouse movements, clicks, scrolls, keyboard events forwarded to `voxl-go`.
  - Tunnelled through authenticated Console sessions and existing `iroh` peer tunnel.

### 3. Frontend UI (Desktop & Android)
- **Desktop UI**:
  - Add "Computer" tab to sidebar / navigation.
  - Lightweight WebCodecs (`VideoDecoder`) canvas viewer.
  - Native mouse and keyboard capture forwarded over WebSocket.
- **Android Mobile App**:
  - "Computer" screen in Compose.
  - Touch-to-mouse translation:
    - Trackpad mode (relative drag + tap to click).
    - Direct touch mode.
    - Soft keyboard and modifier keys bar (Ctrl, Alt, Esc, Super, Enter).
