# Codemode — Claude Code Standard Tool Orchestration in JavaScript

Codemode replaces the model's multi-tool prompt surface with a single JavaScript REPL tool (`code`), allowing the model to orchestrate harness tools programmatically. One model turn can run several searches, perform edits, spawn subagents, and run tests via shell commands—returning only what matters to the context window.

This matches the standard established by Claude Code while integrating cleanly into our existing Go server harness (`apps/server-go`).

---

## 0. Key Principles & Claude Code Alignment

| Feature | Prior Plan | Updated Plan (Claude Code Standard) |
|---|---|---|
| **Tool Surface** | Added `code` alongside direct tools (`read_file`, `edit_file`, `bash`) | **Replaces standard tools.** The model receives a single `code` tool. Direct tools become JavaScript functions inside scripts. |
| **Shell Access** | Excluded in v1 | **Core feature (`$`cmd`)** with `.text()`, `.json()`, and `.nothrow()`, routing to the existing sandboxed `bash` executor. |
| **File Primitives** | Verbose `await tools.read_file({ path })` | **Ergonomic global functions**: `file(p)`, `write(p, s)`, `edit(p, old, new)`. |
| **Tool / MCP Discovery** | Re-sent schemas or MCP groups | **Dynamic discovery**: `searchTools(query)` and `describeTool(name)` allow MCP tools to be discovered on-demand without prompt schema bloat. |
| **Subagents** | Excluded | **`spawnContext({ prompt, role })`** allows sub-agents to be orchestrated directly from scripts. |
| **Runtime** | Goja (pure Go) | **`internal/services/jskernel` (moejs)**: already battle-tested in `browser-use` and `console`, native event loop, async/await, memory limits, and interrupts. |
| **Gating** | Slash command only | **Harness flag** (`CONSOLE_HARNESS_CODEMODE=1`) in `apps/server-go/internal/agent/flags/flags.go`. |

---

## 1. Feature Flag & Gating

Gated behind `flags.Flags` in `apps/server-go/internal/agent/flags/flags.go`:

```go
type Flags struct {
    SparseLineNumbers bool
    LazyTools         bool
    // CodeMode replaces standard tool declarations with a single JavaScript
    // orchestration tool. Standard tools become callable JS primitives.
    CodeMode          bool
}

func FromEnv(getenv func(string) string) Flags {
    return Flags{
        SparseLineNumbers: getenv(HarnessEnvPrefix+"SPARSE_LINE_NUMBERS") == "1",
        LazyTools:         getenv(HarnessEnvPrefix+"LAZY_TOOLS") == "1",
        CodeMode:          getenv(HarnessEnvPrefix+"CODEMODE") == "1",
    }
}
```

When `CONSOLE_HARNESS_CODEMODE=1` is active:
- Standard file and execution tools (`read_file`, `edit_file`, `write_file`, `bash`, `bashJob`, `glob`, `grep`, `list_dir`) are stripped from the model's tool declarations in `internal/run/turns.go`.
- The internal `tools.Registry` retains all of them so they can be dispatched by the JS runtime.
- The model is declared only:
  1. `code` (the JS REPL tool)
  2. `code_reset` (clears the session kernel bindings)
  3. `ask` / `askMany` (user interaction prompts, when interactive)

---

## 2. Sandbox & Runtime Architecture

### 2.1 In-Process Sandbox via `jskernel`
- Runs in the `apps/server-go` process using the existing `internal/services/jskernel` package (powered by `moejs`).
- **Frozen built-ins**: `import`, `require`, `eval`, host file system, and raw networking are blocked.
- **Top-level await & Promise support**: natively handled by `moejs/eventloop`.
- **Resource ceilings**:
  - Max code size: 200 KB (`DefaultMaxCodeBytes`).
  - Max execution time: 600 seconds (10 minutes).
  - Heap memory limit: 128 MiB (`DefaultMemoryLimit`).

### 2.2 Execution Path & Safety
Calls made from inside a script are ordinary tool calls routed through `loop.Executor.Execute`:

```
code tool call
  └─ codemode.Runtime.Eval(ctx, code, budget)
       └─ jskernel runtime (one persistent instance per session)
            └─ JS helper (e.g. edit(p, old, new) or $`cmd`)
                 └─ Executor.Execute(call)
                      ├─ permissions.Resolve(mode, tier)
                      ├─ approver.Approve(...)        ← approval prompt / classifier
                      ├─ Guard.Check(call)             ← project root enforcement
                      ├─ OnBeforeExecute(...)          ← pre-write snapshot for UI diffs
                      └─ tool implementation (bash, edit_file, etc.)
```

1. **Permissions & Tiers**: Every underlying tool call is checked against the active approval mode. Exec and write operations cannot bypass tier restrictions.
2. **Path Guards**: File operations stay strictly confined to the project directory.
3. **Change Tracking**: File edits snapshot changes for the desktop diff review seamlessly.
4. **No Dangling Calls**: Asynchronous tasks left executing after a script evaluation terminates are rejected.

---

## 3. JavaScript API Surface

The script executes in an environment pre-populated with concise orchestration helpers:

### 3.1 Shell Commands (`$`)
Shell execution using tagged template literals or function calls:
```js
// Run a command
await $`go test ./tests/...`

// Extract stdout text
const branch = (await $`git branch --show-current`).text().trim()

// Parse JSON output
const pkg = (await $`cat package.json`).json()

// Safe execution (do not throw on non-zero exit code)
const res = await $`git diff --quiet`.nothrow()
if (res.code !== 0) {
    log("Working tree is dirty")
}
```

Methods on the command execution result:
- `.text()`: returns stdout as a trimmed string.
- `.json()`: parses stdout as JSON.
- `.nothrow()`: suppresses thrown errors on non-zero exit status and returns `{ code, stdout, stderr }`.

### 3.2 Concise File Primitives
- `file(path)`: reads file content as a string (routes to `read_file`).
- `write(path, content)`: writes full file content (routes to `write_file`).
- `edit(path, oldText, newText)`: performs exact string replacement (routes to `edit_file`).
- `glob(pattern)`: returns matching file paths (routes to `glob`).
- `grep(pattern, options)`: searches file contents (routes to `grep`).
- `list(path)`: lists directory entries (routes to `list_dir`).

### 3.3 Dynamic MCP & Tool Discovery
Solves prompt bloat without sacrificing MCP or auxiliary tool extensibility:
- `searchTools(query)`: searches registered tools and returns matching names and summaries.
- `describeTool(name)`: fetches the complete JSON parameter schema for a specific tool on demand.
- `tools.<name>(input)`: invokes any registered tool (built-in or MCP) with its input object.

### 3.4 Progress, Subagents & Utilities
- `log(...args)`: appends to the script's output log returned to the model/user.
- `preview(content)`: emits an intermediate preview snippet.
- `spawnContext({ prompt, role })`: launches a subagent with isolated context and returns its completed summary.
- `store(key, value)` / `load(key)`: persists session-scoped variables across evaluations in the session transcript.

---

## 4. The Model-Facing Tool Schema

The model sees a single orchestration tool:

```jsonc
{
  "name": "code",
  "description": "Execute JavaScript code in the session REPL to orchestrate file operations, shell commands, searches, and tools. Built-ins include $`cmd`, file(), write(), edit(), glob(), grep(), list(), tools.X(), searchTools(), describeTool(), and log().",
  "parameters": {
    "type": "object",
    "properties": {
      "code": {
        "type": "string",
        "description": "The JavaScript code to execute."
      },
      "timeout_ms": {
        "type": "integer",
        "description": "Evaluation timeout in milliseconds (max 600000)."
      }
    },
    "required": ["code"]
  }
}
```

The tool output returns:
1. The return value / completion expression.
2. The stdout/log stream from `log()` and command executions.
3. A compact execution trace summarizing tool calls made during the script (e.g. `✓ file: 12ms`, `✓ bash (git status): 45ms`).

---

## 5. Target Packages & Implementation Plan

### Phase 1: Feature Flag & Runtime Scaffold
1. Add `CodeMode bool` to `apps/server-go/internal/agent/flags/flags.go` reading `CONSOLE_HARNESS_CODEMODE`.
2. Create `apps/server-go/internal/agent/codemode/`:
   - `runtime.go`: wraps `jskernel.Runtime` with the codemode environment.
   - `shell.go`: implements the `$` tagged template runner with `.text()`, `.json()`, `.nothrow()`.
   - `files.go`: implements `file`, `write`, `edit`, `glob`, `grep`, `list` routing to `Executor`.
   - `tools.go`: implements `tools.X()`, `searchTools()`, and `describeTool()`.
   - `tool.go`: implements the harness `code` and `code_reset` tools.

### Phase 2: Session Wire-up & Tool Replacement
1. In `apps/server-go/internal/run/turns.go`:
   - Check `flags.CodeMode`.
   - When enabled, suppress standard file/exec tool declarations from the model's prompt schemas.
   - Register the `code` and `code_reset` tools into the model declaration list.
   - Ensure the internal `tools.Registry` retains all implementations for execution by the codemode bridge.

### Phase 3: Verification & Integration Testing
1. Unit tests in `apps/server-go/tests/codemode/`:
   - Basic script evaluation and return values.
   - `$` shell execution with `.text()`, `.json()`, and error handling (`.nothrow()`).
   - `file()`, `write()`, and `edit()` interacting with test files.
   - Dynamic tool discovery via `searchTools()` and execution via `tools.<name>()`.
   - Permission enforcement: ensuring mid-script calls respect approval modes and project guards.
