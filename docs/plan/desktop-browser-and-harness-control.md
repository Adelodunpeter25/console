# Desktop Browser & Harness Control Plan

## 1. Overview & Goal

Enable the agent on the remote Go server (`server-go`) to control Console's native desktop harness capabilities directly:
1. **Desktop Built-in Browser (`BrowserView`)**: Navigate to dev sites, execute JavaScript to measure DOM/CSS layouts, capture screenshots, and interact with elements without any Chromium/Node/Puppeteer dependencies on the server.
2. **Port Forwarding Discovery**: Inspect remote listening ports and their mapped local desktop URLs.
3. **Project Run Scripts (`console.toml`)**: Query, run, stop, and monitor project services from the Run tab.

### The Real-World Target Workflow
- **Remote Linux VPS (`server-go`)**: Hosts the code, git repo, and dev server processes (e.g. Next.js, Vite, Shopify CLI).
- **Local Machine (`apps/desktop`)**: Runs the GPUI desktop interface, native webview (`BrowserView`), and automatic port-forwarding tunnels.
- **The Workflow**: The agent starts or inspects a dev server, gets the forwarded local port, opens the desktop's built-in browser tab, measures layout dimensions via JavaScript evaluation, fixes the CSS/code on the VPS, and verifies the result with zero guesswork.

---

## 2. Current State

- **Go Server (`apps/server-go`)**:
  - Owns agent loops, tool dispatch (`internal/agent/tools`), and session state.
  - Currently has file, bash, grep, glob, memory, and web search/fetch tools.
  - Has **no direct tools** for querying active port forwards or commanding desktop webviews.
- **Desktop Client (`apps/desktop`)**:
  - Manages `BrowserView` tabs via GPUI webview bindings.
  - Automatically manages port-forward tunnels (`state/port_forward.rs`) mapping VPS remote ports to `http://localhost:<local_port>`.
  - Manages `console.toml` script execution in the Run panel.
- **The Gap**:
  - The agent currently has to resort to ad-hoc bash scripts (e.g. running temporary Node/Puppeteer scripts in `/tmp`) when it needs to inspect rendered web pages.
  - The agent is blind to port forwards and run scripts already configured in Console.

---

## 3. System Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                        LOCAL DESKTOP (GPUI)                            │
│                                                                        │
│  - Native BrowserView (WKWebView / WebView2 / WebKit)                  │
│  - Local Port Forwarding Manager (maps VPS remote port -> localhost)   │
│  - Project Run Panel (managed processes & logs)                        │
└───────────────────────────────────▲────────────────────────────────────┘
                                    │ Bidirectional Client Stream / RPC
┌───────────────────────────────────▼────────────────────────────────────┐
│                       REMOTE VPS (server-go)                           │
│                                                                        │
│  - Agent Loop & System Prompt                                          │
│  - Bash Execution (dev servers, git, builds)                           │
│                                                                        │
│  New Agent Tools:                                                      │
│  ├── 1. `browser` (navigate, eval, measure, screenshot, click)         │
│  ├── 2. `ports` (list active forwards, request port forward)           │
│  └── 3. `project_scripts` (list scripts, trigger run, stop, status)    │
└────────────────────────────────────────────────────────────────────────┘
```

### Communication Protocol
1. **Server-Initiated Client Requests**:
   - When the Go agent calls the `browser` tool, the Go server dispatches a tool action to the connected Desktop client via the existing SSE/WebSocket event channel.
   - The desktop executes the action on the active/target `BrowserView` (or spawns a tab in the workspace).
   - The desktop returns the evaluation result, DOM measurement, or screenshot base64 payload to the Go server to fulfill the tool call.

---

## 4. Proposed Tool Specifications

### Tool 1: `browser` (Native Desktop Webview Control)
Allows the agent to view and verify rendered UI in the desktop's built-in browser.

```json
{
  "name": "browser",
  "description": "Control the built-in desktop browser view to inspect pages, measure layout elements, run JavaScript, or take screenshots.",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["navigate", "eval", "measure", "screenshot", "click", "scroll"],
        "description": "The browser action to execute."
      },
      "url": {
        "type": "string",
        "description": "URL to navigate to (required for 'navigate')."
      },
      "script": {
        "type": "string",
        "description": "JavaScript code to evaluate in page context (required for 'eval')."
      },
      "selector": {
        "type": "string",
        "description": "CSS selector to measure or click (used with 'measure' or 'click')."
      },
      "scroll": {
        "type": "object",
        "properties": {
          "x": { "type": "number" },
          "y": { "type": "number" }
        },
        "description": "Scroll delta (used with 'scroll')."
      }
    },
    "required": ["action"]
  }
}
```

#### Supported Actions:
- **`navigate`**: Opens/updates the desktop workspace `BrowserView` to the given URL (e.g. `http://localhost:9292`).
- **`measure`**: Helper that returns `getBoundingClientRect()`, computed styles (e.g. `width`, `min-width`, `overflow`), and scroll dimensions for a given CSS selector.
- **`eval`**: Evaluates custom JS expressions on the page and returns the JSON result.
- **`screenshot`**: Captures the current webview frame and passes the image to the multimodal model context.
- **`click`**: Dispatches a click event to the target DOM selector.
- **`scroll`**: Scrolls the viewport.

---

### Tool 2: `ports` (Port Forwarding & Discovery)
Allows the agent to see what ports are listening and how they are mapped locally.

```json
{
  "name": "ports",
  "description": "Inspect remote listening ports and their forwarded local URLs on the desktop.",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["list", "forward"],
        "description": "Action: 'list' all forwards or 'forward' a specific port."
      },
      "port": {
        "type": "integer",
        "description": "Remote port number to forward (required for 'forward')."
      }
    },
    "required": ["action"]
  }
}
```

#### Example Output:
```json
[
  { "remote_port": 9292, "local_url": "http://127.0.0.1:9292", "status": "active" },
  { "remote_port": 3000, "local_url": "http://127.0.0.1:3000", "status": "active" }
]
```

---

### Tool 3: `project_scripts` (Run Tab Integration)
Allows the agent to interact with scripts configured in `console.toml`.

```json
{
  "name": "project_scripts",
  "description": "List, start, stop, or check status of project run scripts defined in console.toml.",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["list", "start", "stop", "status"],
        "description": "Action to perform on project scripts."
      },
      "script_id": {
        "type": "string",
        "description": "Script ID from console.toml (e.g., 'dev', 'build', 'test')."
      }
    },
    "required": ["action"]
  }
}
```

---

## 5. End-to-End Example: Debugging a Responsive CSS Bug

1. **User**: *"The announcement slider on mobile is overflowing horizontally on my shopify dev store. Fix it."*
2. **Agent**:
   - Calls `ports(action: "list")` → Discovers `http://127.0.0.1:9292` is forwarded.
   - Calls `browser(action: "navigate", url: "http://127.0.0.1:9292")` → Built-in desktop browser tab opens.
   - Calls `browser(action: "measure", selector: ".announcement-bar-slider")` → Gets exact width (`701px`) and parent width (`390px`).
   - Identifies the `min-width: auto` flex/grid child overflow issue.
   - Edits `assets/base.css` with `min-width: 0` and `minmax(0, 1fr)`.
   - Calls `browser(action: "measure", selector: ".announcement-bar-slider")` again → Confirms width is now `390px` and text is centered at `99px`.
   - Calls `browser(action: "screenshot")` to verify visual layout.
   - Reports the fix with measured evidence.

---

## 6. Implementation Milestones

### Milestone 1: Port & Script Tools (`server-go`) [COMPLETED]
- [x] Add `ports` tool in Go server querying active session port forward state and supporting manual port forwarding.
- [x] Add `project_scripts` tool reading `console.toml` and interacting with the process manager (`list`, `start`, `stop`, `status`).
- [x] Register tools in `DefaultTools()` and bind session services in agent turn executor.
- [x] Unit test suite covering all actions and error handling.

### Milestone 2: Desktop Client Tool Bridge (`apps/desktop`)
- [ ] Implement desktop RPC handlers for browser actions:
  - `evaluate_javascript` bridge on `BrowserView`.
  - DOM measurement script injection helper.
  - Native webview screenshot/snapshot export.
  - Auto-opening / focusing `WorkspaceTabConfig::Browser`.

### Milestone 3: Server Tool Dispatch & System Prompt
- [ ] Add the `browser` tool in `apps/server-go/internal/agent/tools/browser_tool.go`.
- [ ] Wire client-delegated tool calls through the session stream.
- [ ] Update system prompt instructions to guide the agent to use `browser` + `ports` when debugging UI/web layouts.
