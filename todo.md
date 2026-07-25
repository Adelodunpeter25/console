# Console — Active TODO

## Backend: per-session SQLite persistence (project-linked)
- [x] Each session persists to its own `projects/<project-id>/sessions/<session-id>.db` file
- [x] Session DB stores selected `model_id` + `provider` (session_meta)
- [x] `loadSession` reads header + messages from the per-session DB
- [x] Global DB keeps a `sessions` index for fast `listSessions`
- [x] `appendMessage(s)` / `updateTitle` / `updateModel` keep both in sync
- [x] `deleteSession` removes the session DB file + global index row
- [x] `deleteProject` removes the project's whole storage directory
- [x] Orphaned session DBs are found via project-dir scan fallback

## macOS app: robust SSE/JSON parsing + Urls module
- [x] Byte-level SSE frame parser (split on `\n\n`/`\r\n\r\n`), spec-correct field handling
- [x] Unknown event types skipped silently (typed `UnknownEventTypeError`)
- [x] Malformed payloads surface as synthetic error events; stream keeps flowing
- [x] Shared `JSONDecoder.agent` reused across frames
- [x] `Sources/ConsoleCore/Urls/ConsoleURLs.swift` centralizes URL construction
- [x] `Config.swift` slimmed to mutable origin only; `ApiClient` uses `ConsoleURLs`

## macOS app: vendor StreamMarkdown package
- [x] Copy Codevisor `StreamMarkdown` sources into `apps/console-macos/Packages/StreamMarkdown`
- [x] Confirm macOS 13 + Swift 5.9 compatibility (umbrella pkg already targets both)
- [x] Add local package dependency in `Package.swift`
- [x] Wire `StreamingMarkdownView` into chat assistant rendering

## Previously shipped (context)
- SSE per-line decode fix (`95f64c6`)
- Sidebar redesign + macOS 13 compat (`283064f`, `d646fc9`)
- Ghostty vendored but temporarily disabled (`e9174a4`)
