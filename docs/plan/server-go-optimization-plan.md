# Server-Go Optimization & Fix Plan

Plan for Go concurrency, data structures, and performance fixes across `apps/server-go`.

## Tasks

- [x] **Task 1: Fix SingleFlight Memory Leak & Panic Deadlock** (`internal/providers/shared/singleflight.go`, `internal/providers/antigravity/oauth.go`)
- [x] **Task 2: Fix `searchBrave` Gzip Parsing Bug** (`internal/agent/tools/web_search_tool.go`)
- [ ] **Task 3: Fix SQLite Eviction Closing Active DB Handles** (`internal/db/db.go`)
- [ ] **Task 4: Close Script Subscriber Channels & Eliminate Polling** (`internal/services/projects/project_scripts_service.go`, `internal/routes/scripts.go`)
- [ ] **Task 5: Optimize LLM Streaming Token Coalescing** (`internal/agent/loop/loop.go`)
- [ ] **Task 6: Eliminate `[]rune` Allocations for Token Checks** (`internal/agent/compaction/tokens.go`, `internal/agent/loop/breakdown.go`, `internal/agent/compaction/shake.go`)
- [ ] **Task 7: Eliminate Redundant JSON Marshal in `argsChars`** (`internal/agent/compaction/tokens.go`)
- [ ] **Task 8: Prevent Backing Array Memory Leaks in Slices** (`internal/run/hub.go`, `internal/services/terminal/pty_manager.go`, `internal/tunnel/opentunnel/bridge.go`)
- [ ] **Task 9: Reuse HTTP Transport with Tuned Connection Pooling** (`internal/providers/*`, `internal/services/stt`, `internal/providers/usage/*`)
- [ ] **Task 10: Cap `sync.Pool` Slice Sizes in Terminal Routes** (`internal/routes/terminal.go`)
- [ ] **Task 11: Modernize `sort.Slice` to `slices.Sort` / `slices.SortFunc`** (selected hot paths across `internal/`)
