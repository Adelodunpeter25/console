## Summary

Two commits fixing and completing the desktop Tauri app's core logic — the Rust command layer and the frontend store layer — so they fully match the Console backend API. No UI changes in the second commit; the first commit includes minimal ChatScreen rendering fixes needed to make streaming work.

## Commit 1: `fix(desktop): repair SSE streaming, browse API, and persist backend URL`

**Critical fixes:**
- **SSE streaming was completely broken.** The parser stripped the `data: ` prefix from each line *before* pushing it, then tried `.strip_prefix("data: ")` on the already-stripped join — returning `None` every time, so zero events were ever forwarded to the frontend. The UI only updated via the post-run `loadSession` reload. Fixed by parsing the joined buffer directly, plus a trailing-frame flush.
- **`browse_directory` return type didn't match the server.** The command declared `Vec<FsTreeEntry>` but the server returns `{ path, parentPath, entries }` — every call would have failed to deserialize at runtime. Added a `BrowseResult` model matching the server shape; also fixed the frontend `BrowseResult` interface (wrong field name `currentPath` → `path`).

**High-priority fixes:**
- **Backend URL persisted** to `<app_data_dir>/config.json, loaded in the `setup` hook before the frontend reads it, so it survives restarts instead of resetting to `http://localhost:3000`.
- **Shared `reqwest::Client`** via a `Lazy` static, reused across all commands instead of rebuilding one per call.
- **Session-scoped event channel** — agent events now emit on `agent-event:<sessionId>` and the frontend listens per-session, preventing cross-talk between switched/concurrent runs.
- **Live tool/thinking rendering** — `handleEvent` now handles `toolExecutionEnd` and accumulates `streamingThinking`; `MessageBubble` renders all assistant content parts (text, thinking, toolCall) and tool results with error styling.

**Polish:**
- Stable React keys (assistant message `id` instead of array index).
- Chat footer shows project name instead of raw id.

## Commit 2: `feat(desktop): complete store coverage for all backend APIs`

The Rust command layer was already a 100% match to the backend (all 23 routes wired), but the store layer only consumed 11 of 26 `tauriApi` methods. This commit adds the three missing stores and tightens all return types:

- **`useAuthStore`** — `getAuthStatus`, `getLoginUrl`, `handleOAuthCallback` with login-flow state (`pendingProvider`, `loginUrl`, `callbackResult`) and automatic status refresh after a successful callback.
- **`useProviderStore`** — `listProviders`, `getProviderModels` with per-provider model caching.
- **`useFsStore`** — `browseDirectory`, `pickFolder`, `getDirectoryTree`, `readFile`, `writeFile`, `deleteFile`, `createDirectory`, `deleteDirectory` with cache invalidation on writes/deletes and per-path busy tracking.
- **`useProjectStore`** — added `updateSession` (title/model/provider) keeping `sessionsByProject` in sync.
- **`tauriApi` return types tightened** — replaced `unknown` with typed interfaces (`LoginUrlResult`, `OAuthCallbackResult`, `ProviderModelsResult`, `PickFolderResult`, `DirectoryTreeResult`, `ReadFileResult`, `WriteFileResult`, `DeleteFileResult`, `CreateDirectoryResult`, `DeleteDirectoryResult`) matching the server's exact `data` payloads.

**Result: all 26 `tauriApi` methods / 23 server routes now have typed store consumers.**

Also deleted `todo.md` as requested.

## Verification

- `cargo check` → finished, no warnings or errors
- `npm run typecheck` (`tsc --noEmit`) → passes
- `npm run build` (`tsc && vite build`) → built successfully

## API coverage table

| Server route | Rust command | Store consumer |
|---|---|---|
| `GET /health` | `ping_server` | `useServerStore.testConnection` |
| `GET /api/auth/status` | `get_auth_status` | `useAuthStore.loadStatus` |
| `POST /api/auth/login/url` | `get_login_url` | `useAuthStore.startLogin` |
| `POST /api/auth/login/callback` | `handle_oauth_callback` | `useAuthStore.completeLogin` |
| `GET /api/sessions` | `list_sessions` | `useProjectStore.loadSessions` |
| `POST /api/sessions` | `create_session` | `useProjectStore.createSession` |
| `GET /api/sessions/:id` | `get_session` | `useChatStore.loadSession` |
| `PATCH /api/sessions/:id` | `update_session` | `useProjectStore.updateSession` |
| `DELETE /api/sessions/:id` | `delete_session` | `useProjectStore.deleteSession` |
| `POST /api/sessions/:id/run` | `run_agent` | `useChatStore.sendMessage` |
| `POST /api/sessions/:id/abort` | `abort_run` | `useChatStore.abort` |
| `GET /api/projects` | `list_projects` | `useProjectStore.loadProjects` |
| `POST /api/projects` | `add_project` | `useProjectStore.addProject` |
| `GET /api/providers` | `list_providers` | `useProviderStore.loadProviders` |
| `GET /api/providers/:id/models` | `get_provider_models` | `useProviderStore.loadModels` |
| `GET /api/fs/browse` | `browse_directory` | `useFsStore.browseDirectory` |
| `POST /api/fs/pick-folder` | `pick_folder` | `useFsStore.pickFolder` |
| `GET /api/fs/tree` | `get_directory_tree` | `useFsStore.getDirectoryTree` |
| `GET /api/fs/file` | `read_file` | `useFsStore.readFile` |
| `POST /api/fs/file` | `write_file` | `useFsStore.writeFile` |
| `DELETE /api/fs/file` | `delete_file` | `useFsStore.deleteFile` |
| `POST /api/fs/dir` | `create_directory` | `useFsStore.createDirectory` |
| `DELETE /api/fs/dir` | `delete_directory` | `useFsStore.deleteDirectory` |
