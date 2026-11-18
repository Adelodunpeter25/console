# Automations & Webhooks — an agent that schedules and subscribes to future work

The agent can promise future work and keep the promise. "Do this and that every
5 minutes", "merge this change, then remind me in two days to remove the
feature flag and roll it out to everyone", or "subscribe to CI results and fix
any failure" — the agent does the first half now and schedules or subscribes to
the second half, on the same thread, with the thread's full context.

Tool surface in the `console` JS kernel:

- `console.automation.create({prompt, kind, interval_seconds? | run_at? | in_seconds?})`
- `console.automation.get({id?})`
- `console.automation.update({id, prompt?, kind?, interval_seconds?, run_at?, in_seconds?})`
- `console.automation.pause({id})` / `console.automation.resume({id})`
- `console.automation.delete({id})`
- `console.webhooks.subscribe({topic, prompt, filter?, once?, expires_in_seconds?, secret?})`
- `console.webhooks.list()` / `console.webhooks.get({id})` / `console.webhooks.unsubscribe({id})`

One active automation per session (enforced by a partial unique index). A
session may hold multiple webhook subscriptions.

---

## 0. Decisions (settled)

- **Console kernel methods, not standalone tools.** `console.automation.*` and
  `console.webhooks.*` live in `internal/services/console/` and are registered
  in `consoleCatalog()` (`registry.go`).
- **Natural language in, structured out.** The agent translates timing into
  `once` (`in_seconds` or `run_at`) or `interval` (`interval_seconds`). Cron is
  never exposed; `at` (e.g. weekdays at 8:30) is deferred.
- **Completion conditions are prompt-driven, not code.** "Stop checking when
  the backfill finishes" means the agent calls `console.automation.delete`
  when it observes done.
- **One-shot reminders and webhooks stay in the same thread.** Waking in the
  same session preserves full conversation context.
- **Server-triggered turns carry a `source` tag.** Fired automations set
  `source: "automation"` and fired webhooks set `source: "webhook"` on
  `Prompt`, `session_queue`, and `loop.UserMessage`, keeping them out of user
  prompt history and `$chat` references while preserving them in the
  transcript.
- **The desktop/mobile app is just a client.** Schedules and webhook
  subscriptions live in the server's global SQLite DB and fire whether or not
  any client is connected.
- **Public webhook URLs via OpenTunnel alongside iroh.** `iroh` remains the
  default P2P transport for paired clients, while OpenTunnel runs alongside it
  (unless `CONSOLE_WEBHOOK_TUNNEL=off` or `CONSOLE_PUBLIC_URL` is set) to
  provide a permanent public HTTPS URL for external webhook senders.

## 1. Current Server Implementation

- **Global DB tables** (`internal/db/schema.go`):
  - `automations` with partial unique index `idx_automations_one_active` on
    `(session_id) WHERE status = 'active'`.
  - `webhook_subscriptions` with topic/status and session indexes.
  - `session_queue.source` column so queued server-triggered turns survive
    restarts with their source tag intact.
- **Services**:
  - `internal/services/automations/`: CRUD, pause/resume, slot-anchored
    `AfterFire` (no interval drift), and in-process pub/sub (`Subscribe`).
  - `internal/services/webhooks/`: split into `webhooks.go`, `store.go`,
    `auth.go` (query token, Bearer, `X-Webhook-Secret`, HMAC-SHA256), and
    `match.go` (JSON dot-path + array filters, expiry, pub/sub).
  - Session `SoftDelete` and `PermanentDelete` clean up both tables for the
    deleted session.
- **Run integration** (`internal/run/automations.go`, `internal/run/webhooks.go`):
  - `StartAutomationScanner()` runs an immediate sweep on boot, ticks every
    15s, retries transient errors up to 3 times, and stops cleanly on shutdown.
  - `DeliverWebhook` matches incoming payloads and wakes matching sessions.
- **HTTP & SSE routes** (`internal/routes/automations.go`, `internal/routes/webhooks.go`):
  - Public ingress (above `RequireDeviceToken`): `POST /api/webhooks/:topic`
    and `POST /api/webhooks/:topic/:id`.
  - Authenticated REST + SSE (below `RequireDeviceToken`):
    - `GET /api/automations`, `GET /api/automations/stream`,
      `GET /api/sessions/:id/automation`, `POST /api/automations/:id/pause`,
      `POST /api/automations/:id/resume`, `DELETE /api/automations/:id`.
    - `GET /api/webhooks`, `GET /api/webhooks/stream`, `POST /api/webhooks`,
      `GET /api/webhooks/:id`, `DELETE /api/webhooks/:id`.

## 2. Client UI & Remaining TODOs

- **Proto & transcript badge for server-triggered turns (TODO):**
  - Add `source` to `AgentUserMessage` in `proto/console/v1/message.proto` (and
    wire it in `agent/loop/proto.go` + `routes/proto_json.go`).
  - In Desktop (GPUI) and Android (Compose), render turns with
    `source == "automation"` or `source == "webhook"` with a distinct badge /
    event card (e.g. "Scheduled run" / "Webhook event") instead of a user-sent
    chat bubble.
- **Live SSE state in clients (TODO):**
  - Subscribe to `GET /api/automations/stream` and `GET /api/webhooks/stream`
    so the UI updates immediately without reloading.
  - Show a sidebar indicator on threads with active schedules/subscriptions and
    a thread/composer status card (next run, waiting checks, pause/resume/delete).
- **Completed row cleanup (TODO):**
  - Currently, completed one-shot automations and `once: true` webhooks flip to
    `status = 'done'` and remain in SQLite. Decide whether to delete them
    immediately on completion or purge terminal (`done` / `expired`) rows on a
    retention sweep (e.g. 7 days).
- **Future schedule kinds (TODO):**
  - Add `at` schedule kind (e.g. weekdays at 8:30) if needed.
