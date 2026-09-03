# Native macOS Desktop Notifications — Architecture & Implementation Plan

## 1. Overview & Objective

When an agent is running a long task, the user frequently switches to other apps (browsers, editors, terminals). Currently, there is no system-level feedback when:
1. An agent **finishes a task** (or errors).
2. An agent pauses and needs **tool permission** (e.g. executing bash commands).
3. An agent pauses to ask the user a **question** / design decision.

The goal is to deliver real native macOS notification banners and sound alerts, attributed to the Console app, with click-to-focus routing that brings Console to the foreground and focuses the relevant chat session.

---

## 2. Event Triggers & Notification Types

| Event | Notification Title | Notification Body Example | Action on Click |
|---|---|---|---|
| **Run Completed** | `Console · Task Complete` | `Finished: "Fix pagination threshold and scroll jumping"` | Focus window, select session tab |
| **Run Failed** | `Console · Task Failed` | `Error: Process exited with status code 1` | Focus window, select session tab |
| **Permission Request** | `Console · Action Required` | `Agent needs permission to execute: bun test` | Focus window, jump to permission card |
| **Question Asked** | `Console · Question` | `Agent asked: "Which database adapter do you prefer?"` | Focus window, jump to question card |

> **Focus Suppression**: Notifications should only trigger if the Console desktop window is currently **unfocused / minimized / in the background**, or if the user is currently viewing a different chat session than the one sending the notification.

---

## 3. End-to-End Architecture

```
[Backend Agent Pipeline]
       │
       ▼ (Run finishes / asks question / needs permission)
[notificationService.push(...)]  ──► apps/server/api/src/services/notification.service.ts
       │
       ▼ (SSE stream)
GET /api/notifications/stream    ──► apps/server/api/src/routes/notifications.ts
       │
       ▼ (Subscribed over HTTP)
[NotificationService::stream]    ──► apps/desktop/crates/console-core/src/services/notification.rs
       │
       ▼ (Background GPUI Task)
[Desktop Notification Manager]  ──► apps/desktop/src/state/notifications.rs
       │
       ├── Window focused on session? ──► Suppress (silent in-app update)
       │
       └── Window background / other tab?
             │
             ▼
      [Native macOS Dispatcher] (notify-rust / mac-notification-sys)
             │
             ▼
   [macOS Notification Center Banner + Sound]
             │
             ▼ (User clicks banner)
   [Focus Console Window + Select Chat Session]
```

---

## 4. Implementation Components

### A. Backend Event Emission (`apps/server`)
1. **Wire `notificationService.push` in agent runners**:
   - `apps/server/agent/src/session/session-runner.ts`:
     - On run settlement (`finish_run` / status `done` / status `error`): push `run:completed` or `run:failed`.
     - On permission prompt: push `run:permission_required`.
     - On question prompt: push `run:question_required`.
2. **Notification SSE Fanout**:
   - Verify `apps/server/api/src/routes/notifications.ts` properly streams `NotificationEvent` items to connected clients.

---

### B. Core Desktop Client (`apps/desktop/crates/console-core`)
1. **Extend `NotificationEvent` model**:
   ```rust
   // apps/desktop/crates/console-core/src/types/notification.rs
   pub struct NotificationEvent {
       pub r#type: String,       // "run:completed" | "run:failed" | "run:permission" | "run:question"
       pub kind: String,         // "info" | "success" | "warning" | "error"
       pub session_id: String,
       pub project_id: Option<String>,
       pub title: String,
       pub body: String,
       pub timestamp: i64,
   }
   ```
2. **Maintain SSE Connection**:
   - Ensure `NotificationService::stream` has auto-reconnect logic with exponential backoff if the server restarts.

---

### C. Native macOS Notification Dispatcher (`apps/desktop`)
1. **Crate Dependency**:
   - Add `notify-rust = "4"` to `apps/desktop/Cargo.toml`.
   - On macOS, `notify-rust` leverages native Notification Center via `mac-notification-sys` / AppleScript fallback, producing native notifications that work for both dev builds and bundled `.app` packages.
2. **Native Dispatcher Module (`apps/desktop/src/notifications/mod.rs`)**:
   ```rust
   pub fn show_native_notification(title: &str, body: &str, session_id: &str) {
       #[cfg(target_os = "macos")]
       {
           use notify_rust::Notification;
           let _ = Notification::new()
               .summary(title)
               .body(body)
               .sound_name("Default")
               .show();
       }
   }
   ```
3. **App Window Focus Check**:
   - In GPUI: check whether `window.is_active()` is false or `active_session_for_pane` != `session_id`.
   - If user is actively typing or watching the session, skip the banner.

---

### D. Settings & User Preference
1. **Settings Page Toggle**:
   - Add a "Notifications" toggle in Settings:
     - **Enable Native Desktop Notifications** (default: `true`).
     - **Play Sound** (default: `true`).
     - **Notify on Task Completion** (default: `true`).
     - **Notify on Questions & Permissions** (default: `true`).
2. **Persistence**:
   - Store settings in `config.json` via existing settings service.

---

## 5. Click-to-Focus Interaction

When a user clicks on a notification banner:
1. macOS activates the Console app window.
2. The URL / action handler extracts `session_id`.
3. Desktop app invokes `this.select_and_open_session(session_id, cx)` to immediately bring up that chat and reveal the answer card, permission prompt, or finished transcript.

---

## 6. Verification Plan

1. **Unit & Build Checks**:
   - `cargo check --bin console`
   - `cd apps/server && bun test`
2. **Manual Verification Scenarios**:
   - **Background Run**: Start a prompt (e.g. `bun test`), switch to Chrome or VS Code. Verify macOS banner appears when agent completes.
   - **Permission Prompt**: Run a prompt requiring tool approval while in another desktop space. Verify banner appears with "Action Required".
   - **Click Navigation**: Click the banner in macOS Notification Center. Verify Console window focuses and switches to that exact session.
   - **Foreground Suppression**: Watch the agent in foreground. Verify no intrusive macOS banners fire while the session is visible.
