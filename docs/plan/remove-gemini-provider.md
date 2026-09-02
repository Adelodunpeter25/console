# Plan: Remove Gemini Provider

**Status:** Draft  
**Author:** OpenCode  
**Date:** 2026-09-02  
**Scope:** Remove `gemini` (`Google Gemini CLI` / `cloudcode-pa.googleapis.com`) provider entirely. Keep `antigravity`, `opencode`, `codex`, `cline`.

> Antigravity currently shares ~70% of the Gemini wire format (`cloudcode-pa` SSE, `convertMessages`/`convertTools`/`streamCore`, `GeminiOAuthCredential` types). The plan explicitly calls out what is **shared vs. gemini-only** so removal does not regress Antigravity.

---

## 1. Goals / Non-Goals

### Goals
- `ProviderId` no longer contains `"gemini"`; no catalog entry, no stream fn, no OAuth, no usage provider, no constants.
- No `gemini-*` models in `DEFAULT_FALLBACK_MODEL` / fixtures / tests.
- Desktop (Rust + GPUI) and mobile no longer surface Gemini in pickers, settings, or icons.
- Clean deletion: remove `apps/server/providers/src/gemini/`, `apps/server/providers/src/usage/gemini.ts` (+ gemini-only parts of `constants.ts` / `auth/login.ts` / `auth/token-store.ts` / `auth/token-refresh.ts`), not just dead-code the catalog.

### Non-Goals
- Removing the shared CCA helpers (`apps/server/providers/src/shared/*`, `types/cca.ts`) — Antigravity continues to use them. Rename only if desired (optional follow-up).
- Changing Antigravity semantics (keep `antigravity` provider as-is; it uses `daily-cloudcode-pa.googleapis.com` + session envelope).
- Data migration for `~/.console/gemini-creds.json` beyond a logged warning / optional `logout` cleanup.

---

## 2. Current Surface Area (inventory)

### 2.1 Single source of truth
| File | Lines | Action |
|------|-------|--------|
| `packages/types/src/model.ts:6,9` | `ProviderId`, `OAuthProviderId` | Remove `"gemini"` union member |
| `packages/types/src/api.ts` | `AuthStatusResponse = Record<OAuthProviderId, ...>` | Auto-shrinks once `OAuthProviderId` shrinks; verify no hard-coded `gemini` key |

### 2.2 Server — provider registry & catalog
| File | Lines | Notes |
|------|-------|-------|
| `apps/server/agent/src/commands/provider-registry.ts:3,12,37-55,86-93,213-221` | `DEFAULT_GEMINI_MODELS`, `DEFAULT_FALLBACK_MODEL="gemini-3-flash"`, `PROVIDER_CATALOG.gemini`, `fetchModelsForProvider` gemini branch | Delete gemini entry; pick new fallback (e.g. `"gemini-3-flash"` is gemini-only — replace with `"claude-sonnet-4-6"` or first antigravity model). Update `listProviders` / fallback ternary. |
| `apps/server/agent/src/commands/builtins.ts:57` | help text `"gemini, antigravity, opencode"` | Update string |
| `apps/server/agent/src/service/types.ts:39` | comment mentioning Gemini | Update comment |

### 2.3 Server — provider implementation
| File | Action |
|------|--------|
| `apps/server/providers/src/gemini/stream-fn.ts` | **Delete file** |
| `apps/server/providers/src/gemini/index.ts` | **Delete file** (or directory) |
| `apps/server/providers/src/index.ts:5-6,51,54` | Remove `export { geminiStreamFn }`, remove `loginGemini` re-export, clean `GeminiOAuthCredential` re-export if gemini-only (keep type if still used by antigravity — it is shared; leave type but rename if desired) |
| `apps/server/providers/src/constants.ts:1-14,38,51-59,82-94` | Delete `GEMINI_CLI_CLIENT_ID/SECRET`, `GEMINI_BASE_URL`, `DEFAULT_GEMINI_CLI_VERSION`, `GEMINI_SCOPES`, `GEMINI_OAUTH_CONFIG`, `getGeminiCliUserAgent`, `getGeminiCliHeaders`; keep `ANTIGRAVITY_*` + generic OAuth URLs. `ANTIGRAVITY_SCOPES` currently spreads `GEMINI_SCOPES` — inline the 3 scopes. |
| `apps/server/providers/src/auth/login.ts` | Shares logic. Remove `GEMINI_OAUTH_CONFIG` import, `loginGemini()` export, `provider === "gemini"` branches in `completeAuthFlowWithCode` / `loadCodeAssist` / `onboardUser` (projectId/duetProject/mimeType branches), debug env path `gemini-onboard-poll`. Keep `loginAntigravity` + generic `loginWithConfig`. Simplify `OAuthConfig` type to `typeof ANTIGRAVITY_OAUTH_CONFIG`. |
| `apps/server/providers/src/auth/token-store.ts:2,6,9,23-29` | Remove gemini fallback paths (`~/.gemini/oauth_creds.json`, `~/.config/gemini/oauth_creds.json`, `GEMINI_CREDENTIALS_PATH` handling) and gemini-specific comment; keep `antigravity` path only. Default param `"gemini"` → `"antigravity"`. |
| `apps/server/providers/src/auth/token-refresh.ts:12-13,35,37` | Remove `GEMINI_CLI_CLIENT_ID/SECRET` branch; keep antigravity only |
| `apps/server/providers/src/auth/provider-config.ts:3` | Remove gemini projectId comment/branch if present |
| `apps/server/providers/src/discovery/fetch-models.ts:6,12,14,42,52` | Generic fetch for gemini/antigravity — keep but remove `GEMINI_BASE_URL` default; branch now only antigravity (or make antigravity the only OAuth CCA discovery) |
| `apps/server/providers/src/usage/gemini.ts` | **Delete file** |
| `apps/server/providers/src/usage/index.ts:1` | Remove `export { googleGeminiCliUsageProvider }` |
| `apps/server/providers/src/usage/google-antigravity.ts:170` | Keep (antigravity-only); verify no gemini case remains |
| `apps/server/providers/src/shared/convert-messages.ts`, `convert-tools.ts`, `stream-core.ts`, `sse-parser.ts` | **Keep** — now antigravity-only helpers. Optional rename `stream-core` comment header removing "Gemini CLI and Antigravity" → "Antigravity (CCA SSE)". Do not delete. |
| `apps/server/providers/src/antigravity/stream-fn.ts:4,23,42-63` | Keep; remove `GeminiFunctionDeclaration` import if gemini-only type, otherwise keep shared type. Remove gemini-specific fallback model comments. |
| `apps/server/providers/src/types/{index,cca,oauth}.ts` | Keep `GeminiOAuthCredential` name (used by antigravity) or rename to `GoogleOAuthCredential` in a follow-up; not required for deletion |

### 2.4 Server — API layer
| File | Lines | Action |
|------|-------|--------|
| `apps/server/api/src/services/usage.service.ts:11,38-39,50,110,143` | `googleGeminiCliUsageProvider` import + `case "gemini"` + `getAllUsage` list | Remove gemini branch, remove import, shrink provider list to `["antigravity","codex"]`, guard `getUsage` to allow only those |
| `apps/server/api/src/services/auth.service.ts:6,42,46,56-66,95,132` | `GEMINI_OAUTH_CONFIG`, pending tokens, `gemini` status | Remove gemini from `getAuthStatus` return shape, `getLoginUrl`/`handleCallback` validation, `getProjectId`/`setProjectId` allowlist |
| `apps/server/api/src/routes/auth.ts:35,53,77,90` | `if (provider !== "gemini" ...)` guards | Reduce to `antigravity` + `codex` (keep `antigravity` where project-id routes apply) |
| `apps/server/api/src/routes/usage.ts:24`, `providers.ts:29` | usage/provider route gating | Remove gemini |
| `apps/server/api/src/services/session.service.ts:22`, `run.service.ts:115,140` | default `modelId || "gemini-2.5-pro"` | Replace with new default (e.g. antigravity model or opencode) |
| `apps/server/api/src/utils/ignored.ts:20` | `".gemini"` | Keep if still relevant to user projects; optional removal. Not harmful — leave. |
| `apps/server/agent/src/systemprompt/discover-agents-md.ts:21,27,58` | `GEMINI.md`, `".gemini"` | Keep — discovery of user files, not provider |
| `apps/server/scripts/demo-agent.ts:4,8,16,26-27,32,35,102-103` | demo uses `geminiStreamFn` | Switch to `createAntigravityStreamFn()` or `opencodeStreamFn` |

### 2.5 Server — session / tests / fixtures
| File | Action |
|------|--------|
| `apps/server/tests/api.test.ts:29,139,170` | `json.data.gemini`, favorite `gemini`, modelId fixtures | Update to `antigravity`/`codex` fixtures |
| `apps/server/tests/commands.test.ts:19,28,88,103` | `/provider gemini` switch | Replace with antigravity or remove |
| `apps/server/tests/discovery.test.ts:18-19,58,69,82,91,95` | models `gemini-3.1-pro-low` | Replace |
| `apps/server/tests/session-storage.test.ts:16,24,147`, `session-cwd-lock.test.ts:19`, `changes-and-git.test.ts:17`, `system-prompt.test.ts:13` | default `gemini-2.5-pro` fixtures | Replace with a remaining provider model |
| `apps/server/agent/src/session/session-ops.ts:178` | `model_id ?? "gemini-2.5-pro"` fallback | Replace |

### 2.6 Desktop (Rust)
| File | Action |
|------|--------|
| `apps/desktop/crates/console-core/src/types/auth.rs:9,17,42` | `OAuthProviderId::Gemini`, `AuthStatusResponse.gemini` | Remove variant + field; update `as_str()` |
| `apps/desktop/crates/console-core/src/services/auth.rs:2` | gemini project-id comment | Update |
| `apps/desktop/src/state/auth.rs:49,203,207` | `save_gemini_project_id` | Remove gemini arm; keep antigravity/codex |
| `apps/desktop/src/state/app.rs:438` | `Provider("gemini")` picker tab | Remove |
| `apps/desktop/src/settings_window.rs:18-19,46,68-69,107,109,116,119,159,164,174,176` | gemini settings UI | Remove |
| `apps/desktop/crates/console-ui/src/settings/accounts_page.rs:17,19,26,29,64,97,177,212` | gemini project input | Remove gemini row; keep antigravity |
| `apps/desktop/crates/console-ui/src/settings/usage_page.rs:128,133` | gemini usage section | Remove |
| `apps/desktop/crates/console-ui/src/common/{model_picker.rs:22, composer_view.rs:297, primitives/mod.rs:69,86, primitives/icons.rs:202,532,548}` | gemini icons / picker | Remove gemini icon/picker entry |
| `apps/desktop/assets/icons/providers/gemini.svg` | **Delete file** |

### 2.7 Mobile
| File | Action |
|------|--------|
| `apps/mobile/stores/useAuthStore.ts:8,42` | `INITIAL_STATUS.gemini`, `projectIds.gemini` | Remove |
| `apps/mobile/stores/useUsageStore.ts:14`, `hooks/useUsageViewModel.ts:23-27` | `key: "gemini"` | Remove |
| `apps/mobile/screens/settings/account-settings.tsx:30,33,35-36,56,101,154-155,167-168` | Gemini project-ID field | Remove gemini section; keep antigravity |
| `apps/mobile/utils/icons/provider-icons.ts:9,11,17`, `components/icons/provider-icon.tsx:11` | gemini icon | Remove |
| `apps/mobile/scripts/generate-icons.sh:7` | default `~/.gemini/...` | Change default to `~/.console/...` or remove |

### 2.8 Docs & misc
| File | Action |
|------|--------|
| `docs/devin-provider-implementation-spec.md:125,442` | mentions gemini as OAuth example | Update example to antigravity/codex |
| `docs/plan/remove-gemini-provider.md` | This plan | — |

No `@google/*` npm dependencies exist (verified); Gemini is raw REST, so no `package.json` changes.

---

## 3. Phased Execution

### Phase 0 — Preparation (no code)
1. Confirm new `DEFAULT_FALLBACK_MODEL` (recommend `claude-sonnet-4-6` or first `DEFAULT_ANTIGRAVITY_MODELS` entry; update `provider-registry.ts:37`).
2. Decide on `GeminiOAuthCredential` rename: keep name (cheapest) vs. rename to `GoogleOAuthCredential`/`AntigravityCredential` (follow-up, not in this PR).
3. Notify: `~/.console/gemini-creds.json` will become orphaned; document `rm ~/.console/gemini-creds.json` / `console logout --provider gemini` (if exists).

### Phase 1 — Types & registry (1 file, unblocks everything)
1. `packages/types/src/model.ts` — remove `"gemini"` from `ProviderId` + `OAuthProviderId`.
2. `apps/server/agent/src/commands/provider-registry.ts` — delete `DEFAULT_GEMINI_MODELS`, `PROVIDER_CATALOG.gemini`, gemini branch in `fetchModelsForProvider` + fallback ternary; update `DEFAULT_FALLBACK_MODEL` + `AVAILABLE_MODELS` (remove gemini ids).
3. `bunx tsc --noEmit` must pass before continuing — this surfaces all exhaustiveness failures.

### Phase 2 — Server provider core
1. Delete `apps/server/providers/src/gemini/` + `apps/server/providers/src/usage/gemini.ts`.
2. `apps/server/providers/src/index.ts` — remove gemini exports (`geminiStreamFn`, `loginGemini`).
3. `apps/server/providers/src/constants.ts` — delete gemini constants/helpers; inline `ANTIGRAVITY_SCOPES`.
4. `apps/server/providers/src/auth/{login,token-store,token-refresh,provider-config}.ts` — remove gemini branches (see §2.3).
5. `apps/server/providers/src/discovery/fetch-models.ts` — remove `GEMINI_BASE_URL` import/default.
6. Keep `shared/*` untouched (add comment update only).

### Phase 3 — Server API & agent
1. `apps/server/api/src/services/{usage,auth}.ts` + `apps/server/api/src/routes/{auth,usage,providers}.ts` — remove gemini cases/validation.
2. `apps/server/agent/src/commands/builtins.ts`, `service/types.ts`, `session/session-ops.ts`, `systemprompt/*` comments.
3. `apps/server/scripts/demo-agent.ts` — switch demo provider.
4. `apps/server/api/src/services/session.service.ts` / `run.service.ts` — new default model.

### Phase 4 — Desktop
1. `apps/desktop/crates/console-core/src/types/auth.rs` — remove `Gemini` variant + `gemini` field.
2. `apps/desktop/src/state/{auth,app}.rs`, `settings_window.rs`, `crates/console-ui/...` — remove UI branches.
3. Delete `apps/desktop/assets/icons/providers/gemini.svg`.
4. `cargo check --manifest-path apps/desktop/Cargo.toml` must pass.

### Phase 5 — Mobile
1. `apps/mobile/stores/useAuthStore.ts`, `useUsageStore.ts`, `hooks/useUsageViewModel.ts`, `screens/settings/account-settings.tsx`, `utils/icons/provider-icons.ts`, `components/icons/provider-icon.tsx`, `scripts/generate-icons.sh` — remove gemini entries.
2. `bunx tsc --noEmit` + `cd apps/mobile && bunx expo export --platform android` smoke check.

### Phase 6 — Tests & fixtures
1. Update all test fixtures referencing `gemini-2.5-pro` / `gemini-3.1-pro-preview` to a remaining model.
2. Update `tests/api.test.ts` assertion `json.data.gemini` → `json.data.antigravity` (or remove).
3. `cd apps/server && bun tests/<each>.test.ts` per `AGENTS.md` (never full suite unless asked).

### Phase 7 — Docs & cleanup
1. Update `docs/devin-provider-implementation-spec.md` examples.
2. Optional: `apps/server/api/src/utils/ignored.ts` — keep `.gemini` (user project file), no action.
3. Run `make typecheck` and `make check`.

---

## 4. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Antigravity regresses because shared helper was deleted | High — both providers break | **Keep** `shared/*` + `types/cca.ts` + `types/oauth.ts`; only delete gemini-only files |
| `PROVIDER_CATALOG` exhaustiveness break (`Record<ProviderId, ProviderEntry>`) | Compile error | Phase 1 does types first; `tsc` will list every site to fix |
| Existing sessions on disk reference `model_id: "gemini-2.5-pro"` | Sessions fail to resume / run | `session-ops.ts` / `run.service.ts` fallback handles it; optionally add one-time migration that remaps unknown `provider/model` to new fallback |
| Users still have `~/.console/gemini-creds.json` | Orphaned file, confusion | Document manual `rm`; no auto-delete (avoid surprise). `loadCredential("gemini")` will no longer be called so file is inert |
| Desktop `AuthStatusResponse` is `#[serde(default)]` — removing `gemini` field is breaking for old servers | Desktop shows stale field | Desktop and server are versioned together; bumping both in same release is safe. Old server's `gemini` key will be ignored via `#[serde(default)]` on desktop after removal? Actually removal makes desktop reject the key — but `deny_unknown_fields` is not set, so unknown `gemini` from old server is ignored. Safe. |
| `DEFAULT_FALLBACK_MODEL` still points to gemini model | Runtime panic / invalid model | Must change in Phase 1 |
| Mobile deep link / icon registry out of sync | Blank icon / crash | Remove from `provider-icons.ts` + `provider-icon.tsx` together |

---

## 5. Verification Checklist

- [ ] `grep -ri "gemini" apps/ packages/ --exclude-dir=node_modules` returns only historical comments / `.gemini` ignore entries and `discover-agents-md.ts` `GEMINI.md` (user-file discovery)
- [ ] `grep -ri "GEMINI_" apps/ packages/` returns 0 hits
- [ ] `grep -ri "gemini-creds" apps/ packages/` returns 0 hits
- [ ] `bunx tsc --noEmit` passes
- [ ] `cargo check --manifest-path apps/desktop/Cargo.toml` passes
- [ ] `cd apps/server && bun tests/api.test.ts` passes
- [ ] `cd apps/server && bun tests/commands.test.ts` passes
- [ ] `cd apps/server && bun tests/discovery.test.ts` passes
- [ ] `cd apps/server && bun tests/session-storage.test.ts` passes
- [ ] Desktop: Accounts page shows only Antigravity / Codex / OpenCode / Cline; no Gemini row; no `gemini.svg` in `assets/icons/providers/`
- [ ] Mobile: Account Settings shows no Gemini project-ID field; provider picker has no Gemini
- [ ] `make build-server` succeeds
- [ ] Manual: start server, `GET /api/auth/status` returns `{ antigravity, codex }` only; `GET /api/providers` lists no gemini
- [ ] Manual: create session with default model → not `gemini-*`

---

## 6. Out of Scope / Follow-ups

- Rename `GeminiOAuthCredential` → `GoogleOAuthCredential` and `getGeminiCliHeaders` → `getCcaHeaders` (cosmetic, separate PR).
- Delete `~/.console/gemini-creds.json` automatically on logout — needs a `console logout` CLI command that already handles generic providers.
- Remove `.gemini` from `ignored.ts` / `discover-agents-md.ts` — intentionally kept (user project artifact).

---

## 7. Estimated Effort

| Phase | Effort |
|-------|--------|
| 0 Preparation | 0.5h |
| 1 Types & registry | 0.5h |
| 2 Server provider core | 1–1.5h |
| 3 Server API & agent | 1h |
| 4 Desktop | 1–1.5h |
| 5 Mobile | 0.5h |
| 6 Tests & fixtures | 0.5h |
| 7 Docs & cleanup | 0.5h |
| **Total** | **~6–7h** |

---

## 8. Rollback

- Revert commits (one commit per phase, per `AGENTS.md`). Data is not destructive (creds file left on disk), so rollback is `git revert`.

