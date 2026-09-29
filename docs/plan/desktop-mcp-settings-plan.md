# Desktop MCP Settings & Management UI Plan

## Overview
Add an **MCP Servers** tab to the Settings view in the GPUI desktop app (`apps/desktop`). This allows users to view configured Model Context Protocol (MCP) servers, add new local (`stdio`) or remote (`HTTP/OAuth 2.1`) servers via an Add/Edit modal, authorize OAuth connections with the browser, inspect discovered tools, and manage server lifecycles (connect, reconnect, disconnect, delete).

---

## 1. Settings Navigation & Layout

### 1.1 Settings Tab Entry
- Add an `MCP Servers` item to the Settings sidebar navigation list (alongside General, Models, Keys, etc.).
- Active state renders the MCP management view in the settings content pane.

### 1.2 Main MCP View
- **Header**:
  - Title: `MCP Servers`
  - Subtitle: `Connect local tools and remote MCP integrations to the agent harness.`
  - Action: `+ Add Server` primary button (opens modal).
- **Empty State** (when no servers are configured):
  - Icon + descriptive text ("No MCP servers configured yet").
  - `Add your first MCP server` button.
- **Server List View** (when servers exist):
  - List of configured server cards showing:
    - **Header**: Server ID / Display Name, Transport badge (`stdio` / `http`).
    - **Status Badge**:
      - 🟢 `Connected` (`N tools loaded`)
      - 🟡 `Needs Authorization` (with inline `Authorize` button)
      - 🔵 `Connecting...` (spinner / loading state)
      - 🔴 `Disconnected` / `Error` (with error message tooltip / text)
    - **Target / Detail**: Command line string (for stdio) or endpoint URL (for HTTP).
    - **Expandable Tool List**: Accordion / drawer showing registered tools with descriptions and permissions tier.
    - **Actions Menu / Buttons**:
      - `Authorize` / `Connect` (if disconnected or requires OAuth)
      - `Refresh Tools` / `Reconnect`
      - `Edit` (re-opens modal with existing values)
      - `Disconnect` / `Reset Auth`
      - `Delete` (with confirmation)

---

## 2. Add / Edit Server Modal

A modal dialog opened via `+ Add Server` or clicking `Edit` on an existing card.

### 2.1 Form Fields

1. **Server ID / Name**:
   - Unique identifier slug (e.g. `atlassian`, `filesystem`, `github-mcp`).
   - Auto-slugified from name or user-editable string.

2. **Transport / Server Type (Dropdown)**:
   - Option A: `Local (stdio)`
   - Option B: `Remote HTTP (SSE / Streamable)`

3. **Conditional Fields based on Transport**:
   - **For `Local (stdio)`**:
     - **Command**: executable name or path (e.g. `npx`, `uvx`, `docker`, `/usr/local/bin/my-mcp`).
     - **Arguments**: editable list or newline/space-delimited input (e.g. `-y @modelcontextprotocol/server-filesystem /path/to/dir`).
     - **Environment Variables**: Key-Value table for environment variables injected into the child process.
   - **For `Remote HTTP`**:
     - **Endpoint URL**: `https://...` (e.g. `https://mcp.atlassian.com/v2/mcp`).
     - **Authentication Type (Dropdown)**:
       - `OAuth 2.1` (Interactive browser authorization via PKCE / dynamic client registration).
       - `Static Token / Bearer` (Direct API Key / Bearer token input).
       - `None` (Open unauthenticated endpoints).

4. **Optional Permissions & Tool Overrides**:
   - Default tool permission tier setting.

5. **Modal Actions**:
   - `Cancel`
   - `Save & Connect` (saves configuration and immediately triggers connection / OAuth flow).
   - `Save Only` (saves configuration without connecting).

---

## 3. OAuth 2.1 Flow Experience

1. User saves an HTTP server with `OAuth 2.1` auth.
2. Clicking **Authorize** or **Save & Connect**:
   - Sends `POST /api/mcp/servers/:id/connect` to `server-go`.
   - Backend spins up the local loopback server, performs client registration, creates PKCE challenge, and opens the system default browser.
   - Desktop card updates to `Waiting for browser authentication...` with a pulsing indicator.
   - Fallback button: `Copy Auth URL` (in case default browser didn't open).
3. User completes login in the browser; callback hits loopback port.
4. Backend stores credentials in `~/.console/mcp-credentials.json` (0600) and initializes the MCP session.
5. Desktop UI detects status transition to `Connected` and fetches the list of available tools to display.

---

## 4. API & State Integration (`apps/desktop` <-> `server-go`)

The desktop app will communicate with the existing `server-go` REST endpoints:
- `GET /api/mcp/servers`: Load all configured servers and their connection statuses.
- `POST /api/mcp/servers`: Create a new MCP server configuration.
- `PUT /api/mcp/servers/:id`: Update an existing server configuration.
- `DELETE /api/mcp/servers/:id`: Remove a server and delete its stored credentials.
- `POST /api/mcp/servers/:id/connect`: Trigger connection, tool discovery, and OAuth PKCE browser launch.
- `POST /api/mcp/servers/:id/disconnect`: Disconnect running stdio process or terminate session.
- `GET /api/mcp/servers/:id/tools`: Fetch advertised tools for server inspection.

---

## 5. Implementation Phases

1. **Phase 1: Rust Types & API Client (`apps/desktop/crates/`)**
   - Add MCP server and tool model structs in desktop crates matching backend JSON models.
   - Add client methods for MCP CRUD, connect/disconnect, and tool listing.

2. **Phase 2: Settings Sidebar & MCP Tab View (`apps/desktop/src/settings/`)**
   - Add MCP tab to Settings navigation.
   - Implement MCP server list view, status indicators, and empty state.

3. **Phase 3: Add / Edit Server Modal**
   - Implement modal with transport dropdown (`stdio` vs `http`).
   - Dynamic form controls for command/args/env vs URL/Auth type.
   - Form validation (required fields, valid URLs, non-empty command).

4. **Phase 4: Connection & OAuth UX Lifecycle**
   - Implement interactive connection handling, browser OAuth waiting state, and error handling.
   - Expandable tool preview list for connected servers.

5. **Phase 5: Verification & Polish**
   - Test stdio server creation and discovery (e.g. filesystem server).
   - Test OAuth / HTTP server lifecycle and error flows.
