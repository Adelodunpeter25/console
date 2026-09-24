# Global content search (⌘⇧F) — Go backend implementation

## Status

**Backend adapter complete.** The Go `fff` adapter (`apps/server-go/internal/fff/fff.go`)
now exposes everything a "search file contents across the workspace, with
case, whole-word, and regex toggles" feature needs, and the agent's `grep`
tool already uses it. **No HTTP route or desktop UI exists yet** — this doc
also lays out the small remaining slice to wire a real ⌘⇧F panel to this
adapter.

## Why fff and not a hand-rolled searcher

`fff` (https://github.com/dmtrKovalenko/fff) is already vendored as a C-ABI
shared library (`apps/server-go/scripts/fetch-fff-lib.sh` →
`third_party/fff/libfff_c.{dylib,so}`) and already backs the agent's
`glob`/`grep` tools and the `@`-mention file picker
(`apps/server-go/internal/routes/assist.go`). It keeps a persistent,
incrementally-updated index per workspace root (ripgrep-quality content
search, frecency-ranked file search), so a "search everywhere instantly"
UI is a native call away rather than a full filesystem walk per keystroke.
Writing a custom Go searcher would mean re-implementing content indexing,
incremental re-scan on file changes, and match-range/regex diagnostics —
all of which fff already does natively and fast. The Go work here is glue,
not a search engine.

## What changed in the adapter (`internal/fff/fff.go`)

- **Content indexing enabled** on instance creation, so grep hits fff's
  in-memory index instead of a cold per-call scan.
- **`CaseMode`** (`smart` | `sensitive` | `insensitive`) with
  `ParseCaseMode`. Smart/sensitive map to fff's native `smart_case` flag;
  explicit insensitive is rewritten as an inline `(?i)` regex flag (fff's
  C ABI has no separate "always insensitive" bit).
- **Whole-word matching** via `TransformQuery`, which wraps the pattern in
  `\b(?:...)\b` and escapes literal (plain-mode) queries first so special
  characters in the literal text stay literal.
- **Precise match metadata**: `GrepMatch` now carries byte-range
  `MatchRanges`, column/end-column, byte offset, before/after context
  lines, file size/mtime, frecency scores, and a binary/definition flag —
  everything an editor-style highlighted-match UI needs, not just a line
  of text.
- **Regex diagnostics**: `RegexError` surfaces fff's fallback-to-literal
  behavior when a user's regex doesn't compile, so the UI (or the agent
  tool) can tell the user their pattern was invalid instead of silently
  returning literal-text matches.
- **Pagination**: `GrepOptions.Cursor` / `GrepResult.NextCursor` /
  `HasMore` so a UI can page through large result sets instead of forcing
  one huge response.
- **Result totals**: `TotalMatched`, `FilesSearched`, `TotalFiles`,
  `FilteredFiles` for a results-panel header ("123 matches in 40 files").

## Lifecycle safety (the part most likely to bite a hand-rolled version)

Previously `Manager.GetOrCreate` returned a raw `*Instance` pointer. If the
LRU eviction policy destroyed that instance (native `fff_destroy`) while a
caller was still mid-search on another goroutine, that was a use-after-free
in C.

Fixed with a **lease pattern**:

- `Manager.GetOrCreate(root)` now returns a `*Lease`, not `*Instance`.
- A lease holds the instance's `RWMutex` in read mode for the duration of
  each call (`Search`/`Glob`/`Grep`/`GrepWithOptions`/`Health`), and callers
  must call `Release()` (or `Close()`) when done — `defer lease.Release()`
  at every call site.
- Eviction (`evictLRULocked`) and `CloseAll` acquire the instance's lock in
  write mode before calling `Destroy`, so they block until every
  outstanding lease finishes, and no new lease can be acquired on an
  instance already slated for destruction.
- `Release` is idempotent (safe to call more than once, and safe on a nil
  lease), so a `defer` plus an early return can't double-release.
- `Manager.CloseAll()` is called from the server's shutdown path
  (`internal/routes/routes.go`) alongside `runSvc.AbortAll()` /
  `bashJobs.KillAll()` / `scriptsSvc.StopAll()`, so process shutdown always
  tears down native fff instances cleanly.

## Tool bridge (`internal/agent/tools/file_tools.go`)

The `grep` agent tool now accepts `caseMode` and `wholeWord` in addition to
the existing `mode`/`caseInsensitive`/`contextLines`/`maxResults`, uses the
lease API (`defer lease.Release()`), and appends a note to the result text
when fff reports a regex fallback. `glob` was updated to the same lease API.
The walk-based fallback (no native library / instance creation failure)
is unchanged and still runs when fff is unavailable.

## Tests

- `apps/server-go/tests/tools/fff_search_test.go` — pure-Go coverage of
  `ParseCaseMode`, `TransformQuery` (case modes, whole-word wrapping and
  escaping, validation errors), and disabled-manager error handling. Runs
  with no native library.
- `apps/server-go/tests/agent/agent_test.go`:
  `TestGrepCaseModeAndWholeWordFff` — end-to-end through the `grep` tool
  against the real native library, asserting sensitive vs. insensitive
  case matching and whole-word vs. partial-word matching produce different
  result sets. Gated on `FFF_LIB_PATH` like the existing
  `TestGlobGrepFff`, so it's skipped unless the native lib is fetched
  (`bash apps/server-go/scripts/fetch-fff-lib.sh`).

Run just this slice:

```
cd apps/server-go
go test ./tests/tools/... -run TestParseCaseMode
go test ./tests/tools/... -run TestTransformQuery
FFF_LIB_PATH="$(pwd)/third_party/fff/libfff_c.dylib" \
  go test ./tests/agent/... -run TestGrepCaseModeAndWholeWordFff -v
```

## Remaining work to ship ⌘⇧F in the product

The adapter and tool bridge are done. Step 1 below has now landed.

1. **HTTP route** — done. `GET /api/fs/grep` in `internal/routes/fs.go`
   calls `FsService.Grep` (`internal/services/fs_service.go`), which does
   `manager.GetOrCreate` + `Lease.GrepWithOptions` directly (bypassing the
   agent-tool text formatting) and returns a structured `types.GrepResult`
   JSON payload with per-match `matchRanges` for highlighting, grouped
   naturally by `relPath`/`fileName` for the UI to bucket into per-file
   sections. Query params: `root` (required), `q` (required), `mode`
   (`regex`|`plain`|`fuzzy`, default `regex`), `caseMode`
   (`smart`|`sensitive`|`insensitive`, default `smart`), `wholeWord`
   (`true`/`false`), `contextLines`, `limit` (max matches, default 200,
   capped at 2000), `cursor` (pagination). Returns `503` with
   `services.ErrFffUnavailable` when the fff index manager isn't wired in
   for this platform/build, so the UI can render an empty/unavailable state
   rather than treating it as a query error.
2. **Desktop UI**: a results panel/dialog in `apps/desktop`, wired to
   ⌘⇧F, with case/whole-word/regex toggles bound to `caseMode`/`wholeWord`/
   `mode` query params, similar in spirit to `quick_open_palette.rs`. See
   the mockup notes stored in project memory (tags: `search-ui`,
   `global-search`) for the target layout — single search box, three
   inline toggle icons (Aa / ab / .*), "N matches in M files" header,
   results grouped by file with bold match highlighting from
   `matchRanges`.
3. **Cloud workspaces**: confirm the same route works for cloud-backed
   workspaces — likely already true since `fffManager` operates on
   `root` paths and the server owns the filesystem in both local and cloud
   deployments, but worth a smoke test against a cloud session root.
4. **Debounce/cancel-in-flight**: the UI should cancel a stale in-flight
   search when the user keeps typing; the HTTP handler does not yet thread
   Fiber's request context into the fff call, so a follow-up should confirm
   an abandoned query doesn't hold a lease longer than necessary.

(2)–(4) don't block on more adapter or route work — the route already
returns everything needed for a rich search-results UI.

