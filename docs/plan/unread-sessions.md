# Unread sessions — a dot on chats that finished while you were away

Status: **plan only, nothing built.** The dot icon exists as `apps/desktop/assets/icons/unread.svg` (copied from the scratch option `03-dot.svg`) but is not registered or used anywhere yet.

Decisions already made (these replace the old open questions):

1. **Needs attention keeps the alert icon and means "the chat needs you": the agent is asking a question, or its run ended in an error.** It does not cover "finished and you have not looked" (that is the dot). Errored chats stay under needs attention (see "Needs attention in the backend").
2. **The Dock badge switches to counting unread chats**, read from the server so it survives restarts and agrees with the sidebar.
3. **"Mark as unread" is in scope**, in the right-click menu.

## Why

A chat can finish, or stop and need you, while you are looking at another chat or another app. Today the sidebar shows the same green check for every finished chat, whether you have seen the result or not. There is no way to tell "finished and I read it" from "finished and I have not looked".

## What exists today (verified in the code)

**There is no persistent unread state. Nothing on the server or in any client records that a chat's result was seen.** Searching the schema, the protos and all three clients for `unread`, `read_at`, `last_read` and `seen` finds nothing about sessions. The only `last_seen_at` is on paired *devices* (`auth_devices`), which is unrelated.

What is close, and what it is not:

| Thing | Where | Why it is not unread state |
| :--- | :--- | :--- |
| `sessions.status` = `working` / `needs_attention` / `done` / `idle` | `db/schema.go`, `run/turns.go`, `run/service.go` | A run state, not a seen state. A chat is `done` the moment the run ends, and stays `done` forever, whether or not anyone looked. |
| `NotificationEvent` over `/api/notifications/stream` | `proto/.../notification.proto`, `run/notify.go` | A one-shot push. Nothing stores it. A client that is offline or not connected when it fires never hears about it. |
| `OUTSTANDING` set behind the macOS Dock badge | `desktop/src/state/macos_notifications.rs` | **In memory, macOS desktop only.** It counts sessions with a posted banner the user has not opened. It is lost on restart, is not shared with other devices, and is never shown in the sidebar. |
| `settled_at`, `pinned_at` | `sessions` table, `SessionHeader` | The right *shape* to copy: a nullable timestamp on the session row that already travels to every client in the header. |

So unread has to be added. The pattern to follow is `settled_at`.

## Definition

A chat is **unread** when a run finished or needed attention **after** the last time any client showed that chat to the user.

Two timestamps on the session row make this exact:

- `last_activity_at`: set when a run settles into `done` or `needs_attention` (the moments a notification is raised today).
- `last_read_at`: set when a client reports that the user viewed the chat.

`unread = last_activity_at > last_read_at`, computed on the server and sent as one boolean so no client re-derives it (same rule as the pull-request plan: the server computes, clients render).

Rules that fall out of this:

1. **Only a finished or blocked run makes a chat unread.** Starting a run, streaming, tool calls and the user's own messages never do.
2. **Viewing clears it.** Opening the chat, or having it already open and focused when the run ends, counts as read. It must not flash unread for a chat the user is watching.
3. **Read state is per user, shared by every device.** Reading on the phone clears the dot on the desktop. This is why it lives on the server and not in a client.
4. **New and empty chats are never unread.** No run, no activity.
5. **A chat that is running is not unread.** The spinner shows instead.
6. **Settled chats are never unread.** Settling is the user filing it away, so it counts as read.

## What the sidebar shows

The status slot on the left of a row (16px wide, already reserved) shows one icon, chosen in this order:

| State | Icon | Colour |
| :--- | :--- | :--- |
| Running | spinner | accent while focused, grey otherwise |
| Needs attention (a pending question, or a run that ended in an error and has not been opened) | alert | warning while focused, grey otherwise |
| **Unread (finished, not yet seen)** | **`unread.svg`, the dot** | **accent** |
| Finished and seen | nothing (empty slot) | n/a |

The dot is the **"done without being read"** state. Once the chat is read the dot disappears and the slot is empty, so a long list of old finished chats is quiet and only the ones that need a look carry a mark. This replaces the current behaviour of showing the check on every finished chat; the `done.svg` check would no longer be drawn in the sidebar (it stays in the icon set).

Colour: unlike the grey icons on other unfocused rows, the dot keeps its accent colour on unfocused rows. Its whole job is to be noticed, and an unfocused row is exactly where an unread chat sits.

The dot follows the server's `unread` flag and nothing else; the sidebar does not hide it for the selected row. Chats you open are marked read at the moment they become active, which is what clears it. This keeps "Mark as unread" honest: marking the chat you are looking at shows the dot at once and it stays until you leave and come back.

The dot is the same `unread.svg` file everywhere (desktop and Android draw it from the same asset).

## Needs attention in the backend

`needs_attention` already means two things in the code, and you want both to keep meaning it:

| Raised by | Where | Banner (macOS notification) |
| :--- | :--- | :--- |
| The agent asks a question and waits | `run/service.go`: `decisions.Notify` sets `needs_attention`, flips back to `working` when answered | title "Needs Attention", body is the question |
| A run ends with an error | `run/turns.go`: `runErr != nil` sets `needs_attention`; `IsAttentionKind` includes `EventError` | title "Agent Error", body is the error text |

So the **status and the kinds stay as they are**: no new status, no new banner kind, nothing renamed on the wire. The sidebar alert, the Dock badge and both banners keep working for questions and errors alike.

**Banner wording.** The question banner's title becomes **"Needs Your Attention"** (it is "Needs Attention" now). It is one string in `run/notify.go`, plus the three tests that assert it: `console-proto/tests/notification_test.rs`, `server-go/tests/api/notifications_test.go` and `android/tests/NotificationFixtureTest.kt`, and the notification fixtures under `proto/testdata`. The error banner keeps "Agent Error". These are macOS (and Android) system notifications, not anything drawn inside the app.

### The bug this plan has to fix first

An errored chat does **not** stay in needs attention today. When a run ends in an error the server writes `needs_attention`, but the next session-list read (`routes/sessions.go`, the list route and the detail route) sees "no active run" and rewrites any `working` or `needs_attention` chat to `done`. That settle-on-read exists to repair a crashed or aborted run that was left "working" forever. It also wipes a legitimate error within seconds, so the alert flashes and then the chat turns plain `done`.

Fix, with the rule that falls out of it:

- **Needs attention = a pending question, or an errored run that has not been opened yet.** It clears when the user opens the chat (the read call) or sends a new message (status goes `working`).
- **Settle-on-read keeps flipping `working` to `done`** (the crash repair it was written for) but **no longer flips an unread `needs_attention`.** It only flips `needs_attention` once the chat has been read (`last_read_at` is not older than `last_activity_at`).
- **The read route settles it.** `POST /api/sessions/:id/read` also turns a lingering `needs_attention` into `done` when no run is active, so opening an errored chat clears the alert immediately and does not wait for the next list poll.
- **Side effect worth knowing:** a server crash while a question was pending leaves `needs_attention` set with no run behind it. Under this rule it stays until the user opens that chat, then clears. That is acceptable: the chat is worth a look anyway.

Sidebar precedence is **running, then needs attention, then unread, then nothing.** An errored chat you have not opened shows the alert, not the dot, and after you open it the slot goes empty.

## Server

1. **Schema.** Add `last_activity_at INTEGER` and `last_read_at INTEGER` to `sessions`, using the additive-migration loop already in `db/schema.go` (the one that added `pinned_at` and `settled_at`). Both nullable; existing rows start as `NULL`, which reads as "not unread" so an upgrade does not light up every old chat.
2. **Mark activity.** In `run/service.go` and `run/turns.go`, where `setStatus` writes `done` or `needs_attention`, also set `last_activity_at = now`. These are the same places that call `notifyDone` and `notifyEvent`, so unread and banners cannot disagree.
3. **Mark read.** `POST /api/sessions/:id/read` sets `last_read_at = now` and returns the updated header. Idempotent. Modelled on `/settle`.
3a. **Mark unread.** `POST /api/sessions/:id/unread` sets `last_activity_at = now` (leaving `last_read_at` alone, so `last_activity_at > last_read_at` holds) and returns the updated header. It sets the timestamp rather than clearing `last_read_at` because a chat that has never run since the upgrade has no activity time at all. It does not touch `updated_at`. It is refused (409) for a running chat, which shows the spinner instead, and for a settled chat, which is filed away; the menu hides the item in both cases.
4. **Header field.** Add `bool unread = 17;` to `SessionHeader` in `session.proto`, filled from the two timestamps in `sessionHeaderToProto`. The timestamps themselves are not sent, only the boolean. Regenerate the Go code with the existing `buf` setup, and add a fixture under `proto/testdata/session/` (the repo already tests proto fixtures per client).
5. **Push the change.** Add a silent `kind: "read_state"` notification (like `resumed` and `session_deleted`, no banner) carrying the session id, so a device reading a chat clears the dot on the others without waiting for the 30-second list poll.
6. **Do not touch `updated_at`.** Marking read must not reorder the sidebar, which sorts on `updated_at`. This is the same trap `UpdateStatusOnly` exists for.
7. **Activity while a client is viewing.** The server cannot know who is watching, so it always sets `last_activity_at`; the viewing client immediately reports a read. To avoid a visible flicker, clients send the read before drawing the row (see Desktop).

## Desktop

1. **Show it.** `SidebarSessionItem` draws `IconName::Unread` in the status slot per the table above. Register `Unread => "icons/unread.svg"` in `primitives/icons.rs` next to `Done`.
2. **Report reads.** When a chat becomes the active chat in a pane, or a run finishes while its pane is focused and the window is active, call the new read route. Reuse the exact "is the user looking at this session" test the notification code already has (`decide_notification` with `is_window_active` and the active pane's session in `state/notifications.rs`), so the dot and the banner suppression stay in agreement.
3. **Stay in sync.** Handle the new `read_state` event in the existing notification stream handler, and take `unread` from the header on every list refresh and poll.
4. **Mark as unread.** Add "Mark as unread" to the session context menu (`primitives/context_menu/session_context_menu.rs`), above Pin, calling the new `unread` route. Hidden for running and settled chats.
5. **Dock badge counts unread chats (decided).** Replace the in-memory `OUTSTANDING` set in `macos_notifications.rs` with a count computed from the session headers: sessions that are `unread` **or** `needs_attention` (a pending question or an unopened error still deserves a badge). Because it comes from the server, it survives a restart and matches the sidebar. Keep clearing a chat's delivered banner when it is opened. Two details:
   - The headers refresh every 30 seconds today, which is too slow for a badge. On any banner event (`needs_attention` or `done`) and on `read_state`, trigger one debounced session-list refresh so the dot and badge update immediately.
   - The badge is macOS only; the other platforms keep their no-op stubs.

## Android

The header gains `unread` through the shared proto, and the same `read` route is called when a chat opens. The home list's `SessionRow` shows the dot where it shows a status today, and its row menu gets "Mark as unread". The Kotlin fixture tests (`*FixtureTest.kt`) decode `proto/testdata`, so the new fixture is covered there.

## Edge cases and decisions

- **Aborted runs** do not set activity (the notification code already skips them).
- **Resumed runs** (an idle chat waking itself for finished background work) end in `done` and raise activity like any other run. That is correct: new output exists that the user has not seen.
- **Many devices, one chat open on two:** whichever reports the read first clears it for both. Fine.
- **Clock skew:** both timestamps come from the server clock, so comparing them is safe.
- **Offline desktop:** it learns the state from the next list fetch. Nothing is lost because the state lives on the server.
- **Upgrade:** `NULL` timestamps mean nobody sees a dot on old chats. Only runs finishing after the upgrade can produce one.

## Open questions

None blocking. Decided: errored chats stay under needs attention, the banner title is "Needs Your Attention", the Dock badge counts unread chats, and "Mark as unread" is in.

## Tests

- **Server:** a run finishing sets `last_activity_at`; `unread` is true until `/read`; `/read` and `/unread` do not change `updated_at`; `/unread` makes a read chat unread and is refused for running and settled chats; settled and running chats report not unread; the migration leaves old rows not unread; a run that ends in an error sets `needs_attention` and it **survives a session-list read** (the bug above); it clears on `/read` and on a new message; a read `needs_attention` with no active run still settles to `done`; a stale `working` chat with no run still settles to `done`.
- **Proto:** `header_unread.json` fixture decoded by the Rust and Kotlin fixture tests.
- **Desktop:** a pure function for the status-slot choice (running, attention, unread, none) so the ordering above is covered by a plain test, and a pure function for the badge count (unread or needs_attention, settled excluded).

## Not in scope

Per-message read tracking, "unread count" per chat, and push notifications for unread chats on a device that was offline. The dot answers one question: is there a finished result here I have not looked at.
