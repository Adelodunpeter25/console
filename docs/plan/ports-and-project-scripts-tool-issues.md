# Ports and Project Scripts Tool Remediation Plan

This document outlines the evaluation, identified gaps, and remediation plan for the `ports` and `project_scripts` agent tools in the server.

---

## 1. Overview & Evaluation Summary

Both `ports` and `project_scripts` provide solid baseline behavior with truthful error reporting, explicit input validation, and fail-loud semantics when unavailable. However, key ergonomic and lifecycle gaps exist:

1. **`ports` is one-way / state-leaking:** Ports can be forwarded via `ports forward <port>`, but there is no tool action to `unforward` / `stop` / `remove` a forward.
2. **`ports list` lacks visibility into active system listeners:** Returns empty when no forwards exist, even if local services are listening on ports.
3. **`project_scripts` readiness & output signal:** Output on start captures process runner invocations (e.g., `make` command echoes) rather than structured readiness or real-time application health signals.
4. **`project_scripts` process termination ambiguity:** Stopped processes report `exitCode: null` without distinguishing clean exits from SIGTERM/SIGKILL termination.
5. **Generic empty-state messages:** Querying `project_scripts status` with a `script_id` filter returns generic `"No script runs found."` instead of mentioning the targeted script ID.

---

## 2. Issues & Proposed Solutions

### Issue 1: `ports` Missing `unforward` / `stop` Action
- **Problem:** When an agent forwards a port (e.g. `2003` -> `45000`), the forward remains active until the reaper expires or the server restarts. There is no tool command to teardown an active port forward.
- **Solution:**
  - Add `unforward` (or `stop`) action to `ports` tool schema and tool implementation.
  - Require `port` or `forward_id` parameter.
  - Call the ports provider to stop listening and close tunnels/proxies.
  - Return clear confirmation e.g. `Port forward for port 2003 stopped successfully.`

### Issue 2: Local Listener Visibility in `ports list`
- **Problem:** `ports list` only lists active forwards managed by Console. It doesn't reveal which localhost ports are currently listening, forcing trial-and-error port forwarding.
- **Solution:**
  - Enhance `ports list` (or add an optional flag/parameter `include_listening: true` or section) to scan / report detected active localhost listening ports alongside established forwards.

### Issue 3: `project_scripts` Output Streaming & Readiness Signal
- **Problem:** Background server start output immediately after `start` only reflects initial wrapper command echo (`make ...`) rather than server readiness.
- **Solution:**
  - Add support for readiness checks (e.g., checking if expected port opened, regex matching in stdout/stderr, or `wait_for` readiness timeout).
  - Prominently surface both `stdout` and `stderr` tail buffers in status output.

### Issue 4: Process Termination Status Clarity
- **Problem:** Terminating a script with SIGTERM leaves `exitCode: null` and `status: "stopped"`, making it ambiguous whether the process died unexpectedly or was deliberately stopped.
- **Solution:**
  - Include `signal: "SIGTERM"` or explicit `status: "terminated"` / `"killed"` field in run metadata when stopped by user/agent request.

### Issue 5: Specific Empty-State Reporting for Script Filters
- **Problem:** `project_scripts status` with `script_id="dev-server"` returning no results prints `"No script runs found."`, making typos indistinguishable from non-running scripts.
- **Solution:**
  - Format message as `No runs found for script '<script_id>'.` when `script_id` is supplied.

---

## 3. Implementation Steps

### Phase 1: `ports` Tool Improvements
1. **Schema Update:** Add `unforward` (and alias `stop`) to the `action` enum in `ports` tool definition.
2. **Provider API:** Ensure the internal ports manager / service exposes `Unforward(port int)` / `CloseForward(port int)`.
3. **Tool Handler:** Implement `unforward` branch in `ports_tool.go`.
4. **Listener Inspection:** Add helper to detect local listening ports for display in `ports list`.
5. **Tests:** Add unit and integration tests in `tests/tools/ports_tool_test.go` covering forward -> list -> unforward -> list lifecycle.

### Phase 2: `project_scripts` Tool Improvements
1. **Filter Feedback:** Update `project_scripts status` handler to output specific message when filtering by `script_id`.
2. **Process Termination Tracking:** Record signal / termination cause on run metadata upon `stop`.
3. **Readiness & Logs:** Improve log tail formatting (both stdout & stderr) and add optional readiness polling/wait option to `project_scripts start`.
4. **Tests:** Add unit tests in `tests/tools/project_scripts_test.go` verifying filtered status empty states and stop lifecycle metadata.
