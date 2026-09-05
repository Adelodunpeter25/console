# Cross-Platform Notifications (macOS Desktop & Mobile Push) — Architecture & Plan

## 0. Status

- [x] Backend event bus (`notificationService.push` on done/attention) + SSE `GET /api/notifications/stream`
- [x] Mobile local notifications from SSE (`expo-notifications`, `useLocalNotifications`, tap opens session, suppressed when viewing it) — works foreground/backgrounded, not killed-app
- [ ] Remote push for killed-app (device tokens, `device_tokens` table, Expo push dispatcher, APNs/FCM, EAS creds)
- [ ] Desktop native banner + focus (see §3)

## 1. Overview & Objective

When an agent runs a long-running task, users frequently switch away from the desktop app or lock their mobile device. Notifications must be **always enabled by default** (no settings toggles required) to ensure immediate feedback whenever:
1. An agent **finishes a task** (or encounters an error).
2. An agent pauses and requires **tool permission** (e.g. bash execution, file writes).
3. An agent pauses to ask the user a **question** / design decision.

This plan specifies both **Native macOS Desktop Notifications** and **Mobile Push Notifications (iOS & Android via Expo)**.

---

## 2. Notification Triggers & Payloads

| Event | Title | Body Example | Tap / Click Behavior |
|---|---|---|---|
| **Run Completed** | `Console · Task Complete` | `Finished: "Fix pagination threshold and scroll jumping"` | Open / focus app, select session tab |
| **Run Failed** | `Console · Task Failed` | `Error: Process exited with status code 1` | Open / focus app, select session tab |
| **Permission Request** | `Console · Action Required` | `Agent needs permission to execute: bun test` | Open / focus app, reveal permission card |
| **Question Asked** | `Console · Question` | `Agent asked: "Which database adapter do you prefer?"` | Open / focus app, reveal question card |

---

## 3. Desktop macOS Architecture (Always Enabled)

On desktop, notifications are delivered via native macOS Notification Center banners and system sound alerts:

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
       ▼ (Desktop Notification Manager)
[Window Focus Suppression Check]
       │
       ├── Window focused on active session? ──► Suppress (silent in-app update)
       │
       └── Window background / other session?
             │
             ▼
   [Native macOS Notification Banner + Sound]
             │
             ▼ (User clicks banner)
   [Focus Console Window + Select Chat Session]
```

### Desktop Implementation Points
1. **Always Enabled**: No setting toggle needed. Notifications always fire when the window is in the background or viewing another session.
2. **Native Dispatcher (`notify-rust`)**:
   - Deliver notifications through macOS Notification Center using `notify-rust = "4"`.
   - Native sound alerts enabled (`sound_name("Default")`).
3. **Click-to-Focus Interaction**:
   - Clicking a banner focuses the Console window and executes `this.select_and_open_session(session_id, cx)`.

---

## 4. Mobile Push Notification Architecture (iOS & Android)

Mobile notifications alert the user even when the mobile app is completely closed or the phone screen is locked:

```
[Mobile Device (iOS / Android)]
       │
       ▼ (App Launch / Permission Granted)
[Register Expo Push Token]
       │
       ▼ POST /api/notifications/devices
[Server SQLite Device Store]  ──► stored in console-global.db `device_tokens`
       │
       ▲
[Agent Event Fires] (Completed / Permission / Question)
       │
       ▼
[Push Notification Dispatcher] ──► sends push ticket via Expo Push API
       │
       ▼ (Apple APNs / Google FCM)
[Lock Screen Banner & Haptics]
       │
       ▼ (User taps push notification)
[Deep Link: console://session/:id] ──► opens mobile app directly to that chat session
```

### Mobile Implementation Points

Done (local, no infra): `useLocalNotifications` subscribes to SSE, shows on-device banner via `expo-notifications`, tap opens session, suppressed when viewing it.

Todo (remote push):
1. **Device Token Registration**:
   - Mobile app (`apps/mobile`) requests notification permissions on first launch using `expo-notifications`.
   - Sends the Expo Push Token (`ExponentPushToken[...]`) to the server:
     `POST /api/notifications/devices` `{ token: string, platform: "ios" | "android" }`.
   - Server saves tokens in `device_tokens` table in SQLite (`console-global.db`).
2. **Server Push Delivery (`expo-server-sdk`)**:
   - When `notificationService.push` triggers on the backend, the server queries all active device tokens.
   - Dispatches push messages with payload `{ sessionId, type, title, body }` using Expo's HTTP push API.
3. **Deep Linking & Navigation**:
   - Configure notification response listener in `apps/mobile/src/navigation` / root layout:
     ```ts
     Notifications.addNotificationResponseReceivedListener(response => {
       const sessionId = response.notification.request.content.data?.sessionId;
       if (sessionId) {
         router.push(`/session/${sessionId}`);
       }
     });
     ```
   - Tapping the banner instantly opens the exact chat session on your phone.

---

## 5. Shared Backend Notification Service (`apps/server`)

The backend serves as the unified event hub for both Desktop SSE streaming and Mobile Push:

1. **`NotificationService`**:
   - Emits event to active Desktop SSE connections (`/api/notifications/stream`).
   - Dispatches push messages to all registered mobile tokens via Expo Push API.
2. **Agent Runners Hooked**:
   - `apps/server/agent/src/session/session-runner.ts` / `run-hub.ts`:
     - Triggers on run settlement (`status === "done" | "error"`).
     - Triggers on tool permission prompt.
     - Triggers on user question prompt.

---

## 6. Verification & Testing Strategy

1. **Desktop Verification**:
   - Run prompt, switch away from Desktop app. Verify macOS native banner and sound alert appear.
   - Click banner. Verify Desktop app focuses and navigates to the session.
   - Run prompt with Desktop window focused on the chat. Verify banner is suppressed so you aren't spammed while watching.
2. **Mobile Verification**:
   - Run a prompt on Desktop or Web, lock phone screen.
   - Verify lock-screen push notification arrives with sound and haptics.
   - Tap lock-screen notification. Verify mobile app opens and navigates directly into the session.
