# Browser tool: real results, tab ids, and element refs

## Goal
Make the agent `browser` tool return data the model can act on, not confirmations.

## Principles
- Every result contains ground truth (page text, JS return value, element list).
- One tool with an `action` enum.
- Tabs are addressed explicitly by id; errors and timeouts carry state.

## Phase 1: result channel
- Wrap agent scripts in a helper that awaits promises and JSON-serializes the value (with a fallback for DOM nodes and circular objects).
- Each script carries a request id; the page posts `{request_id, ok, value | error}` over the existing webview IPC channel.
- The app keeps a map of pending request ids with a ~10s timeout.
- Cap result size (~20-50 KB) and mark truncation.

## Phase 2: real actions
- `run_js`: returns the serialized result.
- `get_content`: no selector returns visible text (or trimmed HTML); with a selector returns matched text/HTML or "no match".

## Phase 3: tabs and errors
- Optional `tabId` on every action; `navigate` returns the id it used or created.
- New `tabs` action lists open tabs with id, URL, and title.
- "No tab found" errors list the open tabs.
- Navigation timeouts return title, URL, and text loaded so far.

## Phase 4: snapshot and refs (done)
- `snapshot` returns numbered elements, e.g. `[ref=e12] button "Contact me"`, using the same role/name rules as the inspector script (`element_script.js`). Capped at 200 elements; an optional `selector` scopes it.
- `click` and `type` take a ref (`type` also takes `text` and optional `submit` to press Enter). Refs live on the page's `window`, so a full page load drops them, and a detached element is caught too; a stale ref returns a fresh snapshot.
- Password fields are never echoed in snapshots.

## Later
- `screenshot` via the webview's native snapshot, returned as an image.
- `back`, `forward`, `reload`, `logs`.
- `wait_for` is done (selector, URL fragment, or text).

## Notes
- The browser is a visible tab in the user's desktop window with their own session, not headless.
