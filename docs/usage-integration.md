# Usage Integration — Design & Plan

> Quota/usage surface for `apps/mobile` and `apps/desktop` (GPUI). Follows existing settings architecture from `docs/settings-window.md` and `apps/mobile/screens/settings/*`.

Status: **planned, not started.** This doc is the implementation spec — build against it. Backend already shipped: `apps/server/providers/src/usage/*` + `apps/server/api/src/services/usage.service.ts` + `apps/server/api/src/routes/usage.ts` (endpoints `GET /api/usage`, `GET /api/providers/:id/usage` — `apps/server/api/src/app.ts:42`).

---

## 1. Review of Existing Settings Design (take note)

### Mobile — `apps/mobile/screens/settings/settings-screen.tsx:17`
- **Pattern:** `SettingsScreen` is a landing list (4 `Pressable` cards `connection/account/projects/deleted-chats` with `lucide-react-native` icons, `ChevronRight`, summary text derived from `useAuth`, `project$` observables). Selecting a section pushes a sub-screen (`EnvironmentsSettings`, `AccountSettings`, etc) with `ScreenHeader onBack`. BackHandler (`useEffect` `41`) pops section or `setActiveTab("home")`.
- **Styling:** `theme.colors.background`, `GlassSurface`, `bg-card border-border rounded-2xl`, `text-foreground-secondary`. No direct per-page state — reads from `stores/useAuthStore.ts:21` `auth$` (`@legendapp/state`) via `useValue`, and `useProviderCatalog` (`hooks/useProviders.ts:6`) for provider catalog.
- **Auth:** `AccountSettings` (`screens/settings/account-settings.tsx:64`) filters `catalog.providers.filter(p.authMethod!=="none")`, shows per-provider status dot (`Check` green / `Circle` muted), email, `Login/Re-login/Pair` button wired to `auth.login` / `loginWithLocalServer` / `loginCodebuff`. Gemini row additionally has `TextInput` + Save for `Google Cloud project ID` (`auth.saveProjectId`).
- **Data flow:** `loadAuthStatus()` on mount (`hooks/useAuth.ts:32`), `auth$` observables, no query client for auth. Projects use `project$` similarly.

### Desktop — `docs/settings-window.md:1` + `apps/desktop/src/settings_window.rs:14`, `crates/console-ui/src/settings/`
- **Pattern:** Separate top-level window (`src/state/settings.rs` singleton handle), `SettingsShell::new` (`settings_shell.rs:17`) left sidebar 210px with 4 tabs `[Accounts, Connection, Projects, DeletedChats]` + right content. Each page is `RenderOnce` (`accounts_page.rs:21`) taking immutable snapshots (`providers: Rc<Vec<ProviderCatalogEntry>>`, `auth_status: Option<AuthStatusResponse>`) + callbacks `Rc<dyn Fn(..., &mut Window, &mut App)>` that upgrade `WeakEntity<ConsoleDesktopApp>` (`settings_window.rs:88`).
- **AccountsPage** (`accounts_page.rs:59`) mirrors mobile: filters `auth_method != "none"`, status dot `8px` `theme.accent` / `text_ghost`, provider rows with `Login/Re-login` (`px10 py5 rounded6 border`). Gemini project input is `ComposerInput` entity owned by `SettingsWindow` (`settings_window.rs:38`), seeded from `auth_status.gemini.configured_project_id` (`94`).
- **State ownership:** Main `ConsoleDesktopApp` owns everything; `SettingsWindow` only renders snapshots + sends commands (`login_provider`, `save_gemini_project_id`, etc). Long-running OAuth polling lives on main entity.

**Constraints to preserve:**
- Mobile: keep `useAuth`/`useProviderCatalog`/`auth$` + `provider$` as single sources; new page must use `packages/api/src/hooks/useUsage.ts:1` (`useUsage`, `useAllUsage` — `staleTime 30s`, `queryKey usageKeys`).
- Desktop: keep `SettingsTab` enum (`settings/mod.rs:19`) + `SettingsShell` tabs + `SettingsWindow` command-forwarding pattern. No duplicated state in window; new page receives `Rc<Vec<UsageReport>>` or per-provider snapshots.

---

## 2. Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Placement | **New `Usage` tab/section** in existing Settings (not standalone screen) | Quota is per-account, lives next to login status. Matches oh-my-pi `pi usage` being account-adjacent. Reuses settings navigation + scroll/empty states. |
| Mobile nav | Add 5th landing card `usage` with `BarChart3` icon, summary `"n providers • X% used"` | Consistent with `SECTION_META` map (`settings-screen.tsx:17`). One tap → `UsageSettings` sub-screen. |
| Desktop nav | Add `SettingsTab::Usage` (`mod.rs:19`) + 5th entry in `settings_shell.rs:35` tabs array `[Usage, BarChart3]` | Symmetric with mobile; shell already supports N tabs. No modal — keeps existing window lifecycle. |
| Data source | **Server-owned quota** via `GET /api/usage` + `GET /api/providers/:id/usage` (already mounted `app.ts:42`) | Credentials never leave server; mobile/desktop already use `ConsoleApiProvider` + TanStack Query for providers. Reuses `packages/api/src/services/usage.service.ts:1` (`getProviderUsage`, `getAllUsage`). |
| Polling | Server cache 60s (`usage.service.ts:8` `CACHE_TTL_MS`), client `staleTime 30s` + `refetchOnWindowFocus` + manual pull-to-refresh | Balances `retrieveUserQuota`/`fetchAvailableModels` cost (paid tiers) with freshness. oh-my-pi debounces similarly. |
| UI primitives | Mobile: `GlassSurface` + `ProgressBar` + `UsageLimitRow`; Desktop: `div` + `theme` + `progress` + `Label` from `console-ui` | Reuses existing primitives; no new deps. |

---

## 3. Architecture

### 3.1 Shared types (already shipped)
`packages/types/src/usage.ts:1` — `UsageReport`, `UsageLimit`, `UsageWindow`, `UsageAmount`, `UsageProvider` (ported from `oh-my-pi/packages/ai/src/usage.ts:1`). `packages/api/src/hooks/useUsage.ts:1` provides `useUsage(provider)` / `useAllUsage()`.

### 3.2 Mobile module layout
```
screens/settings/
  usage-settings.tsx        # new — main Usage page
  components/
    usage-limit-row.tsx     # bar + label + resetsAt
    usage-provider-card.tsx # header: provider name, email, status dot + limits
hooks/useUsage.ts            # already exists (packages/api) — wrap or import directly
```

`SettingsScreen` (`settings-screen.tsx:17`) extended:
```ts
type SettingsSection = "connection"|"account"|"projects"|"deleted-chats"|"usage"
SECTION_META.usage = { title:"Usage", icon: BarChart3 }
summary.usage = allUsage ? `${limits.length} limits • ${mostPressured?.amount.used ?? "?"}% used` : "No quota data"
```

### 3.3 Desktop module layout
```
crates/console-ui/src/settings/
  usage_page.rs            # new — RenderOnce UsagePage
  mod.rs                   # add SettingsTab::Usage, pub use
  settings_shell.rs        # add 5th tab

src/settings_window.rs     # add Usage fetch + state forwarding
src/state/usage.rs         # new — holds Option<UsageReport> per provider, fetch task, lastFetchedAt (like auth)
```

### 3.4 Data flow (both)

```
Server: creds (~/.console/*-creds.json) → refreshIfNeeded → UsageProvider.fetchUsage → UsageReport
        ↕ GET /api/usage (UsageService 60s cache)
Client: useAllUsage() [TanStack 30s] → render → pull-to-refresh invalidates queryKeys
```

Actions:
- `onRefresh` → `queryClient.invalidateQueries(usageKeys.all)` (mobile) / `app.fetch_usage()` (desktop).
- Not logged in → provider card shows `"Not connected"` + `Login` CTA (reuses `AccountSettings` button style).
- Token expired / `resolveAccessToken` null → `report: null` → show `"No quota — re-login"`.

---

## 4. Page Specs

### 4.1 Mobile — `usage-settings.tsx`
**Inputs:** `const {data: allUsage} = useAllUsage()` (`Record<ProviderId, UsageReport|null>`), plus `auth.status` for login state, `providerCatalog` for display names.

**States:**
- Loading: `ActivityIndicator` (same as `AccountSettings:84` `catalog.loadingProviders`).
- Empty (no OAuth providers logged in): centered `BarChart3` + `"Sign in to see quota"` + button to `section="account"`.
- Per-provider `UsageProviderCard`:
  - Header: provider icon + `displayName`, `email`/`projectId` from `report.metadata` (`gemini.ts:244` `currentTierId`, `google-antigravity.ts:405`), status dot `getUsageStatus(remainingFraction)` color (`ok` green / `warning` amber / `exhausted` red).
  - For each `limit` in `report.limits` (sorted by `remainingFraction` ascending — most pressured first, as `google-antigravity.ts:399` does):
    - Row: `limit.label` (`Gemini gemini-2.5-pro` / `Usage (Google)` / `30 days`), `tier` badge, `window.label` + `resetsAt` relative (`"resets in 5h"` via `parseIsoTimestamp` `shared.ts:1`, `DAY_MS/WEEK_MS`).
    - Bar: `width = (1 - remainingFraction)*100%`, color by `limit.status`.
    - Amounts: `remaining 33.9% (11.2k tokens)` or `used 66% / 100%` — use `limit.amount` fields (`used`, `remaining`, `limit`).

**Provider specifics (from oh-my-pi):**
- **Gemini** (`gemini.ts:8` `GEMINI_TIER_MAP` Pro/Flash/3-Flash): limits per `modelId`, window is `quota` with `resetsAt` from `bucket.resetTime`. Show tier badge `Pro`/`Flash`.
- **Antigravity** (`google-antigravity.ts:339` dedupe `counterKey|tierKey|windowId`): 3 counters `Google`/`Anthropic`/`OpenAI`, each `daily`/`weekly` with `durationMs`. Show counter name as section header.
- **Codex** (`openai-codex.ts:285` `buildUsageLimit`): `primary`/`secondary` (30d/5h) + `Spark` when present, `status` via `buildUsageStatus` (100% still `warning` if `allowed=true`).

**Pull-to-refresh:** `ScrollView` `refreshControl` → `queryClient.invalidateQueries(["usage"])`.

### 4.2 Desktop — `usage_page.rs`
**Struct:**
```rust
pub struct UsagePage {
  pub reports: Rc<HashMap<String, Option<UsageReport>>>, // or Vec<UsageReport>
  pub loading: bool,
  pub last_fetched: Option<SystemTime>,
  pub on_refresh: Rc<dyn Fn(&mut Window, &mut App)>,
  pub on_login: Rc<dyn Fn(String, &mut Window, &mut App)>,
}
```
- Renders same as mobile but with GPUI `div().flex_col().gap(px(12))`, `theme.canvas`/`theme.surface`, `px` sizing, `app_icon(IconName::BarChart, ...)`.
- Bars via `div().h(px(6)).bg(theme.track).child(div().w(percent).bg(statusColor))`.
- Reuses `ProbeState` pattern for `isRefreshing` spinner.

**State:** `src/state/usage.rs` mirrors `auth.rs`: `fetch_usage(cx)` spawns `cx.background_spawn` → `client.usage.get_all()` → updates `app.usage_reports: Option<HashMap>` + `usage_loading: bool`, `cx.notify()`. `SettingsWindow` reads `app.usage_reports` per frame (`settings_window.rs:107` pattern).

### 4.3 Shared empty/error handling
- `report == null` + `auth.loggedIn == false` → `"Not connected"` CTA.
- `report == null` + `auth.loggedIn == true` → `"Quota unavailable — token expired or project missing. Re-login."` (e.g., gemini `forconsole` with `billingEnabled:false` returned `null` in live probe).
- `limits.is_empty()` → `"No limits reported"` (codebuff/opencode return null, not rendered).

---

## 5. Backend Surface (already in console-core ✅, no new server work)

| Need | Call |
|---|---|
| All quota | `GET /api/usage` → `UsageService.getAllUsage()` (`api/src/services/usage.service.ts:55`) |
| Single provider | `GET /api/providers/:id/usage` → `UsageService.getUsage(id)` (`routes/usage.ts:13`) |
| Cache | `CACHE_TTL_MS 60s` server, `staleTime 30s` client |

No new provider-specific endpoints needed. `opencode`/`codebuff` correctly return `null`.

---

## 6. Build Order

1. **Mobile scaffold** — add `usage` to `SettingsScreen` `SECTION_META` + `SettingsSection` union, create `usage-settings.tsx` skeleton (header + loading/empty), wire `useAllUsage` read-only (no bars yet) — exercises navigation.
2. **Mobile limit rows** — implement `UsageProviderCard` + `UsageLimitRow` (bar, `remainingFraction` → `used%`, `resetsAt` via `shared.ts:1` `parseIsoTimestamp` port to JS `Date`).
3. **Desktop shell** — add `SettingsTab::Usage` (`mod.rs:19`), add 5th tab in `settings_shell.rs:35`, create empty `usage_page.rs` (loading/empty), wire `SettingsWindow` snapshot.
4. **Desktop limit rows** — port bar logic from `accounts_page.rs:59` row pattern, add `usage.rs` state + `on_refresh`.
5. **Polish** — pull-to-refresh, `remainingFraction` sorting (most pressured first), status colors, relative `resetsAt` strings, empty/expired token copy.
6. **Verification** — `curl /api/usage` live probe shows 3 gemini limits? Actually live probe showed `gemini:null` (bad `forconsole`), `antigravity: 3`, `codex: 30d 99%`.

---

## 7. Definition of Done

- [ ] Mobile gear → Settings → 5 cards (Connection, Account, **Usage**, Projects, Deleted Chats); tapping Usage shows provider cards with login-aware empty states, 30s staleTime, pull-to-refresh invalidates `usageKeys`.
- [ ] Desktop sidebar gear → Settings window → 5 tabs (Accounts, Connection, **Usage**, Projects, Deleted chats); Usage tab renders same provider cards with GPUI bars, `on_refresh` re-fetches via `ConsoleDesktopApp`.
- [ ] Gemini Pro/Flash, Antigravity Google/Anthropic/OpenAI, Codex primary/secondary+Sparks all render correct `remainingFraction` bars and `resetsAt` (“Daily resets in …”) as in oh-my-pi.
- [ ] Not-logged-in shows Re-login CTA; expired token shows “Quota unavailable”; `cargo check --workspace` + `bun tests/providers-wire.test.ts` still green, `bunx tsc --noEmit` clean.
