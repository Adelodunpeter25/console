# Android Optimization & Fix Plan

Plan for Kotlin coroutines, state management, Compose performance, and memory optimizations across `apps/android`.

## Tasks

- [x] **Task 1: Fix Non-Atomic State Updates in State Holders** (`data/store/ChatStateHolder.kt`, `data/store/AppState.kt`, `data/store/ProjectState.kt`, `data/store/ProviderState.kt`, `data/store/MiscState.kt`)
- [x] **Task 2: Fix SSE Streaming Token Dropping** (`data/stream/ChatStreamClient.kt`)
- [x] **Task 3: Enforce Main-Safety in `HttpTransport`** (`data/api/HttpTransport.kt`)
- [x] **Task 4: Cap Terminal Buffer Growth to Prevent OOM** (`data/store/MiscState.kt`)
- [x] **Task 5: Make `ScrollMemory` Thread-Safe** (`core/util/ScrollMemory.kt`)
- [x] **Task 6: Optimize Date & Time Formatting** (`core/util/Time.kt`)
- [x] **Task 7: Prevent Coroutine Cancellation Swallowing** (`core/forward/LocalForwardManager.kt`, `core/notification/NotificationCoordinator.kt`)
- [x] **Task 8: Optimize Keystroke Trigger & Duration Formatting** (`core/util/ComposerAutocomplete.kt`, `core/util/Format.kt`)
- [ ] **Task 9: Eliminate $O(N^2)$ List Growth in `reconstructRuns`** (`core/chat/ReconstructRuns.kt`)
- [ ] **Task 10: Memoize Markdown Block Parsing** (`feature/chat/markdown/CustomMarkdown.kt`)
