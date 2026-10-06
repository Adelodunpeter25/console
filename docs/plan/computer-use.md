# Computer Use Plan

Status: **not started** (planning, 2026-10-06). Roadmap item: "Computer use" in `roadmap.md`.

Browser use is done and stays on the desktop (it drives Console's own CEF tabs; mobile has no browser). Computer use is the next capability: the agent controls the keyboard, mouse and screen of **the machine the Console server runs on**.

## 1. Goal

- The agent can see the screen and operate native apps on the server's machine. First target: a Mac mini running the Go server. Later: the server moves to a Linux (Ubuntu) cloud box, and maybe a Mac mini as well.
- You can **watch the desktop and approve actions** from the desktop app (and later mobile). Computer use is powerful, so permission handling is a core part of the design, not an add-on.
- Example prompt: "turn off the Tailscale login item".

### Non-goals (first slices)

- Replacing browser use. Web tasks keep using the CEF browser tool.
- Running agents inside managed VMs (Cua Spaces / Lume). We control the real machine the server runs on.
- Non-Claude providers (see 3.3: screenshots only reach Claude today).
- A polished in-app remote-desktop viewer. We start with the OS's own VNC / Screen Sharing.

## 2. Design principles

1. **Always ask by default.** Computer-use tools prompt for approval in every approval mode, including `full-access`. Opting out is an explicit, narrow setting, never the default.
2. **The approver sees what will happen.** The prompt shows the exact action and a screenshot, not just a tool name and JSON.
3. **Screen content is untrusted input.** Text on screen can be a prompt injection. Approvals and a tight tool surface are the defence, so the model never gets to approve its own actions (subagents included).
4. **Never saw off the branch you sit on.** Some actions can cut the user's own connection (Tailscale, SSH, Screen Sharing, network settings). These get an extra warning, and possibly a hard block.
5. **Reuse what exists.** The server already speaks MCP over stdio, has tool tiers and an approval round trip. Build the smallest delta.
6. **Keep a kill switch.** One click stops the run and disconnects the computer-use server.

## 3. Current state (verified in code, 2026-10-06)

### 3.1 MCP support already covers most of the plumbing

- `apps/server-go/internal/services/mcp/`: manager, client, adapter, config. Servers live in `<ConsoleStorageDir>/mcp-servers.json`; transports are `http` and **`stdio`**. A stdio server is just `{"command": "...", "args": [...], "env": {...}}`.
- Connections are lazy, one per server, reconnect on the next call if the child dies, 30 s connect timeout, 2 min per call.
- Tools reach the agent through the `loadTools` meta-tool as a group `mcp:<serverId>`, named `mcp__<serverId>__<tool>`. Groups are remembered per session (in memory only).
- Config fields include `tierOverrides` (per-tool `read` / `write` / `exec`) and `enabled`. REST API: `/api/mcp/servers` (list, create, update, delete, connect, disconnect).

### 3.2 Permissions today (`agent/permissions/permissions.go`)

| Mode | read | write | exec |
|---|---|---|---|
| `full-access`, `plan-mode` | allow | allow | allow |
| `accept-edits` | allow | allow | prompt |
| `always-ask` (default) | allow | prompt | prompt |

- A tool's tier decides everything. There are no per-tool rules, no "always allow", no deny, no audit log. Nothing about approvals is persisted.
- MCP tools use the same check. Their tier comes from `tierOverrides`, then the server's `readOnlyHint` annotation (trusted), then name prefixes (`get_`, `list_`, ...), else `write`.
- Approval flow: the executor pauses, the server sends SSE event `permissionRequest` `{requestId, toolCallId, toolName, args, tier, reason?}`, the client answers `POST /api/sessions/:id/approve {requestId, allow}`. Timeout 10 minutes. Desktop renders a `PermissionInteractionCard`; Android a `PermissionPanel`.

### 3.3 Gaps that matter for computer use

1. **Subagents bypass approval.** `agent/loop/subagent.go:148` builds the subagent executor with `permissions.FullAccess`, and subagents inherit the parent's loaded tools. A subagent could call a computer-use tool with no prompt. Must be fixed before shipping.
2. **No "always ask" concept.** No tier forces a prompt in every mode, and a server's own `readOnlyHint` can downgrade a tool.
3. **MCP images are dropped.** `services/mcp/adapter.go` (`renderResult`) turns image content into the text `[image ... omitted]`, so the model never sees an MCP screenshot. The built-in browser tool shows the working format: `{"type":"image","data":<base64>,"mimeType":"image/png"}`, which the Claude converter turns into an Anthropic image block.
4. **Only Claude gets screenshots.** The codex, antigravity and opencode converters replace tool-result images with a placeholder.
5. **Clients never see tool-result images.** `run/turns.go` (`stripResultImages`) strips them from the SSE stream because large base64 stalled the desktop SSE parser. The approval prompt has no image field either.
6. **Session history grows.** Tool results (including images) are persisted into the session; screenshots add up. Check the replay path for an image retention policy.
7. **Android approvals are blind.** `ChatRepository.parsePermission` drops `args`, `tier` and `reason`.
8. **`$` does nothing.** `$` has no behaviour in any composer (only `/` and `@` are triggers), and `roadmap.md` already reserves `$conversation-title` for cross-chat references. Using `$computer` as a trigger would collide with that.
9. **The server is a plain background process.** `console start` re-executes the binary with `setsid` (no launchd, systemd or Docker). Computer use needs a GUI session (macOS) or a virtual display (Linux), so how the daemon is launched matters (section 7).

## 4. Architecture

```
 Desktop app / Android            Console server (on the Mac mini, later Ubuntu)
 +------------------+   HTTPS    +----------------------------------------------+
 | chat, approvals  | <--------> | agent loop -> permissions (always ask)       |
 | screenshot in    |   SSE      |        |                                     |
 | the approval card|            |        v   MCP client (stdio)                |
 +------------------+            |   cua-driver mcp  --->  the machine's screen |
        ^                        +----------------------------------------------+
        |   VNC / Screen Sharing (separate channel, over Tailscale)
        +-------------------------------> the machine's desktop (watch live)
```

- The server starts `cua-driver mcp` locally over stdio, like any stdio MCP server. No SSH or bridge, because the server runs on the machine being controlled.
- Approvals use the existing SSE + `/approve` round trip, extended with a preview image.
- Live viewing is a separate channel (VNC), so the user can watch while deciding. It does not go through the Console protocol.

## 5. Permission model

Layered, simplest first.

1. **Server-level `requireApproval`** on an MCP server config (new). Tools from that server resolve to prompt in every mode, including `full-access` and `plan-mode`, and ignore `readOnlyHint` and name-prefix tiers. Set `tierOverrides` for the Cua tools anyway (everything `exec` except pure reads like a screenshot, to be decided after Phase 0 shows the tool list).
2. **Close the subagent bypass.** Subagents use the parent's mode and honour `requireApproval`.
3. **Better prompts.** Fill the `reason` field with a human summary of the action ("Click 'Remove' at (412, 188) in System Settings") and attach a **screenshot preview**, downscaled and JPEG-encoded. Prefer a small fetch route (`GET /api/sessions/:id/approvals/:requestId/preview`) over inlining base64 in SSE or the 500-event replay ring.
4. **Shorter timeout** for computer-use approvals (a stale screenshot is misleading; 10 minutes is long).
5. **Grants (later).** After per-action prompts work: "allow for this session", "allow for N minutes", scoped to an app. Always revocable, always visible in the UI.
6. **Self-lockout guard.** A small list of risky targets (Tailscale, Screen Sharing / remote login, network and firewall settings, Console itself). Actions touching them get a distinct warning, and in `full-access` still prompt.
7. **Audit log (later).** Persist each approval decision (who, what, when, screenshot hash) so there is a record. Nothing is persisted today.
8. **Kill switch.** A visible Stop that aborts the run (`RejectAllForSession` already exists) and disconnects the MCP server.

## 6. Watching and approving remotely (VNC)

Why: when the server is headless or on another box, the user needs to see the screen to judge an approval.

- **Mac mini:** macOS has Screen Sharing (a VNC server) built in; turn it on in System Settings. The desktop can open `vnc://<mini>` with the system viewer at zero code cost.
- **Ubuntu cloud box (assumed approach, to verify):** run a virtual display (Xvfb or a TigerVNC server) with a light desktop environment, expose it over VNC, and point the computer-use tool at that display. noVNC can show it in a browser or WebView.
- **Network:** reach VNC only over Tailscale (or another private network). Never expose the VNC port publicly, and use a VNC password or SSH tunnel on top.
- **Later:** an in-app viewer tab (RFB client or embedded noVNC) next to the chat, plus a screenshot-only fallback in the approval card for mobile. Android can use the approval screenshot or an external VNC app.

## 7. Platform notes

### macOS (the Mac mini)

- Computer use needs a **logged-in user session with the display awake**. A launchd daemon without a GUI session cannot see or click anything. Run the server as a **LaunchAgent** (or from the login session) and enable auto-login on the mini.
- **Accessibility** and **Screen Recording** permissions are granted per app or binary. Whichever process launches `cua-driver` (the server, or `cua-driver` itself) needs them, granted once. First use may show system prompts that block the call. The 30 s MCP connect timeout may need raising, or the permissions granted before first use.
- Disabling a login item is done through System Settings (General > Login Items); a UI agent can do it, and so could a script, so computer use is not the only route for that example.

### Linux (Ubuntu cloud server, later)

- There is no screen by default. Needs a virtual display, the `DISPLAY` environment available to the server process, and a window manager.
- X11 is the realistic target; Wayland automation is harder.
- Cua's README lists Linux as supported for the driver; its headless behaviour must be verified (Phase 4).

## 8. How the user invokes it

- `$` is not implemented, and the roadmap reserves it for cross-chat references. **Do not use `$` for tool selection.**
- The model currently loads MCP groups itself through the `loadTools` meta-tool (`mcp:cua`). Once the server is registered and enabled, "turn off the Tailscale login item" can work with no special syntax.
- Add a **skill** (`skills/computer-use/SKILL.md`, discovered by `/` autocomplete and the `readSkill` tool) with instructions: always screenshot first, prefer the accessibility tree over pixel coordinates, one action at a time, stop and ask when unsure.
- Optional later: a per-run toggle on the prompt request (`runPromptBody`) and an eager `LoadGroup(mcp:cua)` for sessions that opted in.

## 9. Implementation phases

Start small; each phase is independently useful and shippable.

**Phase 0: try it by hand (no code).**
- Install Cua Driver on a Mac, register it in `mcp-servers.json` (`{"id":"cua","label":"Cua Driver","transport":"stdio","command":"cua-driver","args":["mcp"],"enabled":true}`), grant the permissions, and run one harmless task from a chat.
- Record: real tool names and annotations, how long connect and the first call take, what the permission prompts look like.
- Output: a tool list to base `tierOverrides` on, and confirmation that stdio works from `console start`.

**Phase 1: screenshots reach the model.** Change `renderResult` in `services/mcp/adapter.go` to emit `{"type":"image","data":<base64 string>,"mimeType":...}` for MCP image content (check whether the SDK gives raw bytes or base64). Claude only. Unit tests.

**Phase 2: permissions.** `requireApproval` on the MCP server config (+ REST DTO and settings UI fields), an `AlwaysAsk` check in the executor before `Resolve`, tier overrides for Cua tools, and the subagent fix at `subagent.go:148`. Tests for each mode and for subagents.

**Phase 3: approval UX.** Action summary in `reason`, screenshot preview (fetch route), desktop `PermissionInteractionCard` and Android `PermissionPanel` render it, fix Android `parsePermission`, shorter timeout, Stop button.

**Phase 4: remote viewing.** Document the macOS Screen Sharing setup and an Ubuntu virtual-display + VNC recipe; verify Cua Driver on Linux; a "Open desktop" button that launches `vnc://`. Launch the server as a LaunchAgent / systemd user service.

**Phase 5: polish.** Session grants, audit log, self-lockout guard, in-app viewer, `computer-use` skill, optional per-run toggle.

**Phase 6: other providers** if needed (extend the codex / opencode / antigravity converters to carry images).

## 10. Open questions

- Does Cua Driver offer a non-stdio transport, and does it work well headless on Linux? (Not checked.)
- What exact tools does it expose, and do they carry `readOnlyHint`? (Phase 0.)
- Will the Mac mini be a dedicated machine, or also used day to day? That decides how strict the guard needs to be.
- Should approvals also be possible from mobile with only a screenshot, without VNC?
- How should images in session history be retained (screenshots add up)?
- Is a per-app allow-list worth having in addition to approvals?
- Should `plan-mode` (currently allow-all, enforced only by the prompt) be tightened as part of this?

## 11. Verification matrix

| Check | How |
|---|---|
| stdio server connects and lists tools | `POST /api/mcp/servers/:id/connect`, then `GET /api/mcp/servers/:id` |
| Screenshot reaches Claude | run a chat that asks what is on screen; the answer must describe the screen |
| Always-ask holds in every mode | unit tests for `always-ask`, `accept-edits`, `plan-mode`, `full-access` |
| Subagent cannot bypass | unit test: a subagent calling a flagged tool triggers an approval request |
| Approval shows the action and screenshot | manual on desktop and Android |
| Deny works | denying returns an error result to the model and nothing happens on screen |
| Stop works | abort mid-task; MCP child is gone, no further actions |
| Remote view | connect to the mini over Tailscale with Screen Sharing; see the agent's actions live |
| macOS permissions | fresh login on the mini, server started as a LaunchAgent, first call succeeds |
