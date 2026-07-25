# Console macOS — Feature Roadmap

This tracks the incremental restoration of UI features from the original
Codevisor project, adapted to use the HTTP-based `ConsoleCore` API layer
instead of the old ACP/`CodevisorServerClient` infrastructure.

## ✅ Completed

- **HTTP API layer** — `ApiClient` with all endpoints (auth, sessions, projects, providers, files, SSE streaming)
- **Data models** — Swift `Codable` types matching the Rust/Node backend 1:1
- **View models** — `AppViewModel` (global state) + `SessionViewModel` (per-session streaming)
- **Basic sidebar** — session list, new session sheet, pull-to-refresh
- **Basic chat** — user/assistant/tool messages, streaming text, thinking indicator
- **Basic composer** — prompt editor with model/provider pickers, send/abort
- **Basic settings** — server URL, auth status, provider catalog
- **Code editor** — file viewer/editor using [CodeEditor](https://github.com/ZeeZide/CodeEditor) with syntax highlighting

## 🚧 In Progress

### 1. Terminal Pane — Ghostty terminal integration
- [ ] Add `libghostty-spm` dependency (`GhosttyTerminal` product)
- [ ] Create `TerminalPaneView` using `TerminalViewState` + `TerminalSurfaceView`
- [ ] Wire terminal into session view as a toggleable bottom pane
- [ ] Support `.exec` backend (local shell) and `.inMemory` backend (server-managed PTY)
- [ ] Terminal focus management (`terminalFocusOnAppear`)
- [ ] Terminal configuration (font family, size, cursor style, shell integration)
- [ ] Color scheme sync with app appearance (light/dark)
- [ ] Terminal title in tab/pane header
- [ ] Terminal resize handling

### 2. Workspace/Pane System — tabs, splits, drag-to-reorder
- [ ] `Pane` protocol — pluggable pane types (terminal, chat, code, diffs)
- [ ] `PaneGroupModel` — tab state, selection, visibility, persistence
- [ ] `WorkspaceSplitView` — resizable split tree with drag dividers
- [ ] `WorkspaceTabBar` — browser-style tabs with drag-to-reorder, close, identity badges
- [ ] `PaneTabDrag` — drag-and-drop tabs between panes
- [ ] Keyboard shortcuts — split horizontal/vertical, close tab, next/previous tab
- [ ] Persist pane layout across app restarts

### 3. Rich Transcript Rendering — the chat experience
- [ ] `AssistantTurnView` — worked-items disclosure ("Worked for Xs"), auto-collapse on finish
- [ ] Streaming markdown rendering with incremental parsing
- [ ] `ToolCallRow` — expandable tool calls with shimmer-while-running
- [ ] Diff counters (+N/−N) with rolling numeric animation
- [ ] `DiffView` — syntax-highlighted diffs with Myers algorithm, line numbers, add/remove coloring
- [ ] `PlanDocumentView` — "Proposed Plan" card with markdown rendering
- [ ] `MessageCopyButton` — copy message text on hover
- [ ] Virtualized transcript for performance (large sessions)
- [ ] Retry status display for transient failures (529 overload, etc.)
- [ ] Subagent tracking — show background tasks spawned by the current turn

### 4. Theme System — full palette, glass, animations
- [ ] Restore `Theme` with full semantic token set (40+ colors)
- [ ] VSCode theme parsing and palette derivation
- [ ] `ThemedRootModifier` — theme injection at every window root
- [ ] `ThemedSurface` — glass/material surfaces (sidebar vibrancy, popover chrome, card shadows)
- [ ] `Motion` — centralized animation tokens (springs, entrances, reduce-motion)
- [ ] `ShimmeringText` — shimmering "Thinking…" sweep effect
- [ ] `AdaptivePanelLayout` — responsive sidebar (dock at wide, drawer at narrow)
- [ ] `HoverTracking` — AppKit-based hover for transparent regions
- [ ] Theme picker in settings with live preview
- [ ] Terminal theme sync

## 📋 Planned

### 5. Project Management
- [ ] `AddProjectFlow` — add project with native directory picker
- [ ] `GitCloneSheet` — clone a repo via URL
- [ ] `ProjectSetupPanel` — project configuration
- [ ] `RemoteDirectoryBrowserSheet` — browse remote directories
- [ ] Project icons and metadata in sidebar

### 6. Todo/Goal/Plan UI
- [ ] `TodoPanelView` — todo checklist pinned above composer (codex `update_plan`, Claude TodoWrite)
- [ ] Progress tracking and current step indicator
- [ ] `GoalBannerView` — session objective, usage, pause/resume/clear
- [ ] `PlanDocumentView` — plan mode rendering with approve/reject
- [ ] Goal creation/replacement through composer goal-mode toggle

### 7. Attachments
- [ ] `AttachmentViews` — image/file attachment rendering with async loading and caching
- [ ] `AttachmentLightbox` — QuickLook fullscreen viewer
- [ ] Drag-and-drop file attachments into composer
- [ ] Paste image attachments
- [ ] Attachment thumbnails in transcript

## Architecture Notes

All features are built on the HTTP-based `ConsoleCore` layer:
- `ApiClient` → `URLSession` → backend REST API + SSE
- `AppViewModel` → global state (`@MainActor`, `ObservableObject`)
- `SessionViewModel` → per-session streaming state
- No ACP, no `CodevisorServerClient`, no vendored GhosttyKit

Dependencies:
- [CodeEditor](https://github.com/ZeeZide/CodeEditor) — syntax-highlighted code editor
- [libghostty-spm](https://github.com/Lakr233/libghostty-spm) — Ghostty terminal (requires Xcode 16+)
