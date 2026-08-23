# Environments — Multiple Backend Endpoints with Fast Switching

Inspired by t3code's Environments screen: a list of named backend URLs with connection
status, an add/edit flow with "test connection", and a home-screen switcher so changing
servers no longer requires a trip through Settings.

## 1. Current state (verified)

- The backend URL is a single MMKV key `@console_backend_url` (`useServerConnection.ts`),
  applied via `configureConsoleApi({ baseUrl })` (`packages/api/src/client.ts`).
- Switching URLs already clears stale state when the URL differs:
  `clearChatCache()` (zustand persist `console-chat-cache`) + `queryClient.clear()`
  (`useServerConnection.ts:66-69`).
- A connection test already exists: trim/scheme-normalize + 6s `AbortController` probe
  against `/health` (`useServerConnection.ts:59-95`; server route in
  `apps/server/api/src/app.ts:28`).
- Auth is **server-side**: the mobile app stores no token; `useAuthStore.loadStatus()`
  queries the connected backend for login status. No client-side credential migration is
  needed per environment — auth must simply be re-checked after a switch.
- Home header renders `ScreenHeader ... showSettings onSettingsPress` (`home-screen.tsx:106`).

## 2. Decisions

| Question | Decision | Rationale |
| --- | --- | --- |
| Storage | MMKV (existing `appStorage`), single JSON key | 3–5 rows of config; sqlite adds a native dep + migrations for nothing |
| Cache on switch | Keep clear-on-switch (existing behavior) | Chat cache is capped (25 sessions / 50 messages) and refetches from the server; per-URL partitioning is deferred until switching frequency proves it necessary |
| Status dot | Last-probed result only, refreshed on screen focus / app foreground | No background polling or websocket heartbeat infrastructure exists |
| Switch confirmation | `confirmAlert` before activating another env | Switching clears local chat history; silent loss feels like a bug |
| Scope | Mobile first | Desktop ports later; keep all logic in UI-agnostic hooks/stores |

## 3. Data model

One MMKV key `@console_environments`:

```jsonc
{
  "environments": [
    { "id": "env_9f3a", "name": "Moonbase Terminal", "url": "https://moonbase.tail9f3a.ts.net" }
  ],
  "activeId": "env_9f3a"
}
```

- `id`: `env_` + 4 random hex chars, assigned at creation.
- Migration: if legacy `@console_backend_url` exists and the list is empty, seed one
  environment ("Default") from it and set it active; keep the old key in sync on save so
  downgrades / other readers of the legacy key don't break.
- URL normalization shared with the existing logic: trim, strip trailing slashes, default
  `http://` scheme. Extract to `utils/url.ts` and reuse in both places.

## 4. New store/hook: `stores/useEnvironmentsStore.ts`

Zustand store persisted via MMKV (same pattern as the chat cache):

- State: `environments`, `activeId`, plus probe results `Record<envId, { ok: boolean; checkedAt: number }>`.
- Actions:
  - `addEnvironment(name, url)` / `updateEnvironment(id, {name?, url?})` /
    `removeEnvironment(id)` — removing the active env falls back to no active env
    (disconnected state already handled by `useServerConnection`).
  - `activateEnvironment(id)` — runs the switch sequence below.
  - `probeEnvironment(id)` — bounded `/health` check (reuse the existing 6s-timeout
    probe); stores the result.
- Switch sequence (`activateEnvironment`):
  1. If URL unchanged from active → no-op.
  2. `clearChatCache()` + `consoleQueryClient.clear()` (extract these two lines from
     `useServerConnection.saveConnection` into one shared `resetServerState()` helper).
  3. Set active id; sync `configureConsoleApi({ baseUrl })` and the legacy MMKV key.
  4. Re-run `useAuthStore.loadStatus()` and refetch core queries (providers/projects)
     against the new backend.

## 5. Screens & UI

### 5.1 Environments screen (`screens/settings/environments-settings.tsx`)

Reworks/replaces the endpoint editor in `connection-settings.tsx`:

- List of environments: name, URL, status dot (green/red/gray from last probe),
  chevron to detail; tap row opens detail.
- `+` button → add form: name, URL, "Test connection" button (inline ok/fail feedback,
  existing iconography `CheckCircle2` / `XCircle` / `LoaderCircle`).
- Save requires a passed test unless the user confirms saving untested.

### 5.2 Environment detail screen

Same form pre-filled, plus: "Set as active", "Test connection", "Delete"
(delete disabled for the active env while it is the only one).

### 5.3 Home-screen switcher

- `ScreenHeader` gains an optional `showEnvSwitcher` action (server icon next to the
  settings gear, `home-screen.tsx:106`).
- Opens the existing shared bottom sheet (`components/common/shared-bottom-sheet.tsx`)
  listing environments: name + URL host + status dot; current env highlighted.
- Tapping another env: `confirmAlert("Switch environment?", "...clears cached chats")`
  → `activateEnvironment(id)` → close sheet, stay on home (session list refetches).

## 6. File changes

- `apps/mobile/utils/url.ts` (new) — shared `normalizeBackendUrl`.
- `apps/mobile/stores/useEnvironmentsStore.ts` (new) — model + actions above.
- `apps/mobile/hooks/useServerConnection.ts` — extract `resetServerState()` +
  normalization; delegate storage to the environments store; keep its public API intact
  for existing callers.
- `apps/mobile/screens/settings/environments-settings.tsx` (new) — list + add.
- `apps/mobile/screens/settings/environment-detail-screen.tsx` (new) — edit/test/delete.
- `apps/mobile/screens/settings/connection-settings.tsx` — becomes a thin wrapper or is
  replaced by the environments screen in `settings-screen.tsx`.
- `apps/mobile/components/layout/screen-header.tsx` + `home-screen.tsx` — env-switcher
  action + sheet wiring.
- `packages/api/src/client.ts` — unchanged (already runtime-configurable).

## 7. Verification

- Manual smoke (no suitable RN test runner configured):
  1. Add second env with bad URL → test fails visibly.
  2. Fix URL → test passes → save.
  3. Switch envs from the home sheet → chat cache cleared, session list refetches,
     auth status reflects the new server.
  4. Kill + relaunch app → active env restored, correct backend hit.
  5. Delete non-active env; attempt deleting the only env → blocked.
- Regression: legacy single-URL install upgrades to one seeded environment without
  disconnecting.
- Bundling check per repo rules: `cd apps/mobile && npx expo export --platform android`.

## 8. Out of scope (deferred)

- Per-environment chat-cache partitioning (hash env id into `PERSIST_NAME`) — revisit if
  switching proves frequent.
- Live/websocket connection heartbeats for the status dots.
- Desktop app parity (reuse `useEnvironmentsStore`-equivalent logic then).
- Per-env client-side credentials (not needed today: auth state lives server-side).
