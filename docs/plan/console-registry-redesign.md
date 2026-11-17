# Console Registry Redesign

## Goal

Make the `console` JS API extensible: new capabilities become self-contained modules instead of edits to `runtime.go`, `manager.go`, and `methodArgs`. Replace the growing `SKILL.md` with runtime help discovery.

## Design

### 1. Method metadata lives beside the handler

Introduce a `Method` type in the console package:

```go
type Method struct {
    Name        string            // e.g. "sessions.create" (fully qualified)
    Description string
    Args        map[string]ArgSpec // arg name -> spec
    Call        func(ctx context.Context, args map[string]any) (*jskernel.Result, error)
}

type ArgSpec struct {
    Type     string // "string", "integer", "boolean", "object", "array"
    Required bool
    Doc      string
}
```

This replaces both the `jskernel.Method` list and the separate `methodArgs` map. Argument validation (`jskernel.OnlyArgs`) is generated from `Args`.

### 2. Modules own their methods

Each domain becomes a module type that produces `[]Method`:

- `portsModule` — ports.list/forward/unforward
- `scriptsModule` — scripts.list/run/status/output/stop
- `sessionsModule` — sessions.* (lifecycle, control, wait, search)
- `projectsModule` — projects.list/add
- `mcpModule` — mcp.*
- `discoveryModule` — models, approvalModes, help, describe

Modules receive only the dependencies they need (e.g. `sessionsModule` gets `*services.SessionService`, `PromptFunc`, `QueueControl`; `portsModule` gets `*terminalsvc.PortRegistry`). No god-object `Runtime` fields.

### 3. Registry assembles the kernel

A `Registry` collects `[]Method` from all modules, applies arg validation wrapping, and hands the flat list to `jskernel.New`. `Runtime` shrinks to: kernel + registry (for help/describe lookups) + session context (sessionID, projectID, cwd, ceiling).

`Manager` keeps its setters but passes dependencies into module constructors instead of one giant `NewRuntime` signature.

### 4. Help API (new methods)

- `console.help({query?, namespace?})` — search method names/descriptions/arg docs. Returns ranked matches: name, description, arg summary.
- `console.describe({name})` — full detail for one method: description, args with types/required/doc, and an example call.

Help indexes the same `Method` metadata — no separate documentation source.

### 5. SKILL.md shrinks

Reduce to: what console is, how methods are called (single object arg, errors throw), how to use `help`/`describe`, safety rules (approval clamping, can't abort own session). Remove per-method documentation; the model fetches it on demand.

## Files

- `apps/server-go/internal/services/console/method.go` (new) — Method, ArgSpec types
- `apps/server-go/internal/services/console/registry.go` (new) — Registry, module interface, help/describe handlers
- `apps/server-go/internal/services/console/modules/*.go` or flat `*_module.go` files — one per domain, handlers moved out of `runtime.go`/`session_*.go`
- `runtime.go` — shrinks to kernel construction + context
- `manager.go` — wires module dependencies
- `SKILL.md` — rewritten short
- Tests: existing `runtime_test.go`, `session_test.go`, `control_test.go`, `mcp_test.go` keep passing; add registry/help tests

## Non-goals (this change)

- No renames of existing `console.*` methods — API stays identical.
- No turn_id/run_id redesign of sessions — separate future change.
- No changes to `jskernel` itself beyond what arg validation needs.

## Verification

- `cd apps/server-go && go build ./...`
- `cd apps/server-go && go test ./internal/services/console/ -v`
- Manual: eval `console.help({query:"session"})` and `console.describe({name:"sessions.create"})` in a test runtime.
