# Settings Window — Design & Plan

> Desktop settings surface for `apps/desktop` (GPUI). Four sections, mirroring mobile's
> settings screens: **Accounts**, **Connection**, **Projects**, **Deleted chats**.
>
> Status: **planned, not started.** This doc is the implementation spec — build against it.

---

## 1. Decision record

| Decision | Choice | Rationale |
|---|---|---|
| Presentation | **Separate top-level window** (not a modal) | OAuth login takes minutes (browser round-trip + server token exchange); modals block the whole app during that. Deleted chats is a browsing surface that benefits from seeing the sidebar simultaneously. Mobile's settings are full screens — a separate window is the desktop translation of "screen". |
| State ownership | **Main app entity owns everything** (`ConsoleDesktopApp`) | Single source of truth. The settings window only renders snapshots and sends commands via `WeakEntity<ConsoleDesktopApp>` upgrades (pattern already used across the codebase). |
| UI framework | **gpui-component's built-in `Settings` component** (`Settings::new(id).pages([...])`) | Ships sidebar with search filtering, pages → groups → items hierarchy, built-in field types (`bool`/`dropdown`/`number`/`string`), and `SettingItem::render(custom_element)` for fully custom content (account rows, deleted-chat rows). It's an embeddable element — works identically as a window root or overlay if we ever change our minds. |
| Where components live | `crates/console-ui/src/settings/` (new module) | Pages are reusable UI; the app crate (`src/settings_window.rs`) hosts them in a real window and feeds view models. console-ui already depends on console-core, so pages can take core types directly (`ProviderCatalogEntry`, `AuthStatusResponse`, …). |

---

## 2. Architecture

### 2.1 Window lifecycle

- `src/state/settings.rs` (app crate): holds `Option<WindowHandle>`. `open_settings(cx)`:
  - if handle exists → focus it, return;
  - else `cx.open_window(...)` whose root is a new lightweight `SettingsWindow` entity wrapped in gpui-component's `Root` (**each window needs its own Root** — main.rs shows the pattern).
- Bounds persisted through existing `persistence/window.rs` under a dedicated key.
- Closing the main window closes the settings window (check in main-window shutdown path).

### 2.2 Data flow

```
SettingsWindow entity ──commands──▶ ConsoleDesktopApp (main entity)
        ▲                                        │
        └──────────renders snapshots─────────────┘
```

- The settings window **never** keeps its own copy of domain state (auth status, projects,
  environments). It reads from the main entity per frame (entities are app-global in GPUI;
  windows are just rendering contexts) and emits commands: `LoginCodebuff`, `StartOAuth(provider)`,
  `SaveProjectId`, `ActivateEnvironment`, `RestoreSession`, …
- Every mutation on the main entity calls `cx.notify()` → both windows re-render from shared state.
- Long-running flows (OAuth wait, codebuff polling) run as spawned tasks owned by the main app
  entity so they survive the settings window closing mid-flow.

### 2.3 Module layout

```
crates/console-ui/src/settings/
├── mod.rs                    shared types: ProbeState, EnvironmentRow (view-models live here —
│                             console-ui cannot depend on the app binary)
├── settings_shell.rs         composes the four pages into gpui-component `Settings`
│                             (SettingPage/SettingGroup/SettingItem::render wrappers)
├── accounts_page.rs          AccountsPage
├── connection_page.rs        ConnectionPage (+ inline environment editor states)
├── projects_page.rs          ProjectsPage
└── deleted_chats_page.rs     DeletedChatsPage

src/settings_window.rs        app crate: window lifecycle, SettingsWindow root entity,
                              maps domain state → page view models, routes commands back
src/state/settings.rs         open/close/focus, single-instance handle, bounds persistence
```

Pages are `RenderOnce` structs taking immutable snapshots + command callbacks — same pattern as
`ComposerView` / `WorkingIndicator`. Editing inputs (Gemini project id, env URL/name fields) use
`ComposerInput` entities owned by the app, following the composer pattern.

---

## 3. Page specs

### 3.1 Accounts (`accounts_page.rs`)

**View model inputs**
- `providers: Rc<Vec<ProviderCatalogEntry>>` filtered to `auth_method != "none"`
- `status: Option<AuthStatusResponse>` (from `client.auth.status()`)
- per-provider busy flags (`logging_in: HashSet<provider>`) + error text

**Rows**: provider display name, status dot (green logged-in w/ email · gray not), action button
per auth method: `Login` → codebuff flow (codebuff provider) or browser-OAuth flow; `Re-login`
when already logged in; Gemini row additionally gets a **project id input + Save**
(`client.auth.save_project_id(gemini, id)`), pre-filled from `configured_project_id`.

**Logic to implement (app layer, `src/state/auth.rs`)**
- Codebuff device-code: `codebuff_start()` → open system browser to `loginUrl`
  (`open::that` crate or reuse the osascript exec helper used by the folder picker) → poll
  `codebuff_poll(fingerprintId, fingerprintHash, expiresAt)` every **2s**, swallowing transient
  errors, until `completed == true` or deadline → reload status. Timeout = `expiresAt`.
- Browser OAuth (gemini/antigravity/codex): `login_url(provider)` → `{authUrl, state, redirectUri}`.
  Open browser. **Spawn loopback listener** on port parsed from `redirectUri` (this is desktop's
  equivalent of mobile's Android `LocalAuthServerModule.kt`; the server does NOT listen itself):
  - bind one-shot `TcpListener` (std, in a spawned task) on that port
  - accept exactly one GET, parse `code` + `state` from query, serve "authenticated, close this tab" HTML
  - validate `state` matches issued value
  - forward via `auth.handle_callback(provider, code, state)` — server performs token exchange +
    credential storage; can take seconds (gemini chains userinfo→loadCodeAssist→onboardUser) → show busy spinner
  - hard timeout 2 min (mobile parity); listener must die after first valid hit or timeout
- After any success: reload `auth.status()`.

**Why the listener is unavoidable**: the provider redirects the *browser* to
`localhost:{fixed-port}` on the machine running the browser (= desktop). The server never listens
on those ports outside its own CLI-style flows, and codex PKCE verifiers are held in *server
memory* keyed by `state`, so the code must come back through `POST /api/auth/login/callback`.

### 3.2 Connection (`connection_page.rs`)

**State (new)** `src/state/environments.rs`:
- `environments: Vec<{id, name, url}>`, `active_id: Option<String>`, `probes: HashMap<id, ProbeState>`
- persisted via `persistence/store.rs` key `@console_environments` (JSON). **No MMKV legacy-key
  migration — desktop persistence starts clean.**

**List rows**: name + url host + probe dot (`Unknown` gray / `Ok` green / `Failed` red / `Probing`
pulsing). Actions: activate (radio/current marker), edit, remove.

**Inline editor** (`creating` or `editing(id)` mode): name + URL fields, `Test connection` button →
`probe_backend(url, 3s)` (console-core already exports it); Save requires passing test; dedupe on
normalized URL (scheme-insensitive host compare; trailing-slash trim).

**Switch logic** (`activate_environment`): persist → `client.set_base_url(url)` → **reset semantics**
(mobile `resetServerState` equivalent, but simpler — no query cache exists):
clear `sessions`, `projects`, `branches`(+loaded/pending flags), `pending_permissions`,
`pending_questions`, `agent_notices`, `running_sessions`, `transcript_scroll_positions`,
per-pane transcripts/composers/attachments → re-run bootstrap fetch sequence (sessions, providers,
models, favorites, projects) → reload auth status.

**Clean disconnect**: wipe all environments → relaunch onboarding (below).

### 3.3 Projects (`projects_page.rs`)

Rows from existing `projects: Rc<Vec<ProjectInfo>>`: name, path, Remove button (confirm dialog —
server-side cascade behavior TBD while building). Add via the existing folder-picker flow
(`osascript` approach already in the app) → `client.projects.add(path)`.

### 3.4 Deleted chats (`deleted_chats_page.rs`)

Rows from `client.sessions.list_deleted(None)`: title, deleted date, actions **Restore**
(`restore(id)` → refresh trash list + main sidebar sessions) and **Delete permanently**
(`permanent_delete(id)`, confirm dialog — irreversible). Empty state when no trashed chats.

---

## 4. Onboarding tie-in

When `environments.is_empty() || active_id.is_none()`: full-screen onboarding over the workspace
(name + URL + Test connection + Connect) — mirrors mobile's `backendUrl == null` gate. Reuses the
Connection editor internals. Out of scope for the first pass if we seed a default localhost env.

---

## 5. Backend surface (all already in console-core ✅)

| Need | Call |
|---|---|
| Auth status | `client.auth.status()` |
| OAuth start | `client.auth.login_url(provider)` |
| OAuth finish | `client.auth.handle_callback(provider, code, state)` |
| Codebuff login | `client.auth.codebuff_start()` / `codebuff_poll(...)` |
| Gemini project id | `client.auth.save_project_id/get_project_id` |
| Deleted chats | `client.sessions.list_deleted/restore/permanent_delete` |
| Projects | `client.projects.list/add/remove` |
| Env probe | `probe_backend(url, timeout)` (utils) |
| URL switch | `client.set_base_url(url)` |

Remaining console-core gap: none for v1. Environments persistence is app-layer (persistence/store.rs),
not a server concern.

---

## 6. Build order

1. **Shell** — `settings.rs` window lifecycle + `settings_shell.rs` composing 4 empty pages into
   gpui-component `Settings`; wire sidebar footer `open-settings` button (exists, no handler today).
2. **Deleted chats page** — pure CRUD; exercises the whole snapshot/command pattern end-to-end.
3. **Projects page** — same shape + folder picker wiring.
4. **Connection page** — environments store + editor + reset-on-switch semantics (most architectural weight).
5. **Accounts page** — codebuff flow first (no listener needed), then loopback OAuth listener.
6. **Onboarding gate** — optional last.

---

## 7. Definition of done

- [ ] Sidebar gear opens a focused, singleton settings window; opening twice focuses instead of duplicating.
- [ ] All four pages render from main-entity state; no duplicated state in the settings window.
- [ ] Accounts: codebuff login completes end-to-end; OAuth completes via loopback listener with state validation and 2-min timeout; Gemini project id saves and pre-fills.
- [ ] Connection: add/edit/probe/save requires passing test; switching URLs resets all caches and refetches; clean disconnect returns to onboarding.
- [ ] Projects: add via picker, remove with confirm.
- [ ] Deleted chats: restore returns chat to sidebar; permanent delete is confirmed and irreversible.
- [ ] Closing main window closes settings window; bounds persist across launches.
- [ ] `cargo check --workspace` clean; terminal + api tests still green on the server side.
