# Browser JS kernel — `/browser-use`

Status: design draft. No kernel code yet.

The browser tool is already functional. This is an orchestration layer, not a
replacement for the driver: one JavaScript runtime per chat session that can
navigate, observe, act, extract, loop, and clean up inside one bounded call.
Token savings are not the main argument; the existing `browser` schema is small.
The argument is state and multi-step workflows.

The precedent is the CUA kernel (`internal/services/cua`): one persistent goja
runtime, a small preinstalled API, two tools, and an embedded `SKILL.md`. We
follow that model, with one important difference: the browser kernel is
**synchronous**, because goja has no async/await and no microtask pump.

---

## Decisions

- **Sync API only.** Calls block until the desktop returns. `async`, `await`,
  `.then`, and Promise completions are refused with a clear error, matching
  `internal/services/cua/js.go`.
- **Two new tools:** `browser_run` executes code; `browser_reset` clears the
  kernel and cleans up its tabs. The existing `browser` tool stays available
  as the low-level escape hatch.
- **The JS object is `browser`.** The tool name and the JS namespace are
  different; `browser_run` runs code against `browser`.
- **One runtime per chat session**, bound implicitly. Do not accept a
  caller-supplied session ID; that would let one session reach another.
- **Desktop is the source of truth** for tabs, URLs, titles, and page state.
  The kernel stores variables, locator handles, the owned-tab set, and a trace.
- **Reset closes only tabs the kernel opened.** Never close user tabs.
- **No new browser engine.** Reuse the existing browser actions and driver.
- **The kernel does not automatically fix** cross-origin iframes, trusted
  input, browser profile/geo identity, or CSP-blocked `run_js`. Those are
  driver/native problems.

---

## What “risky action” means

There is no separate `buy()` or `send()` method to gate. A risk is an ordinary
click or type that causes one of these outcomes:

| Category | Examples |
| --- | --- |
| Money | Buy, order, checkout, pay, subscribe, donate, book |
| Communication | Send message/email, post, comment, submit an application |
| Account/security | Log in/out, password/MFA change, permissions, API keys, delete account |
| Destructive | Delete, remove, cancel, discard, unsubscribe, close account |
| Downloads/installs | Download a file, install an app/extension |
| Legal/consent | Accept terms, sign, consent to privacy processing |

**MVP policy:** the kernel is for research, extraction, navigation, search,
filtering, and bounded pagination. It must not complete a purchase, send a
message, change account/security settings, download/install, delete data, or
sign/consent to something. Cookie/privacy consent is report-only unless an
explicit policy says otherwise.

`browser_run` is `TierExec`, so the user approves the script once. That is a
coarse gate: one approval can cover many actions. In the MVP the risk policy is
enforced by the skill and visible in the trace, not by a hard security
boundary. Hard enforcement needs target inspection and per-action approval;
that is Phase 3, not the first slice. The existing `browser` tool remains the
explicit escape hatch for one-off actions.

**Known inconsistency:** the existing `browser` tool is `TierRead` and can
already click anything. This plan does not change that. If the risk policy must
be a boundary rather than a guardrail, the browser tool's tier and approval
model need a separate change.

---

## Architecture

```
agent
  -> browser_run({code})
     -> per-session goja runtime
        -> browser object / page objects
           -> BrowserHandler (existing request/response bridge)
              -> desktop browser actions + page driver
  -> browser_reset({tabId?})
     -> close kernel-owned tabs, drop runtime
```

Target package layout:

```
apps/server-go/internal/services/browseruse/
  manager.go      # per-session runtimes, owned tabs, LRU cap
  js.go           # goja runtime, host bridge, limits, trace
  api.go          # browser/page objects, locator and extract helpers
  tools.go        # browser_run, browser_reset, Loader
  invoke.go       # /browser-use splitting
  skill.go        # embedded SKILL.md
  SKILL.md
```

Server wiring:

- `internal/run/browser_use.go` — per-session activation marker, mirroring
  `computer_use.go`.
- `internal/run/turns.go` — detect `/browser-use`, append the skill, restore
  the kernel tools on later runs, and keep the existing `browser` tool.
- `internal/agent/systemprompt/builder.go` — no default inventory change:
  `browser` stays listed; `browser_run` / `browser_reset` load lazily.
- `internal/run/decisions.go` — unchanged; reuse `BrowserHandlerFor`.

The manager owns one `*JSRuntime` per session. It is capped (16, like CUA) and
evicted least-recently-used. Eviction drops variables only; the owned-tab set
stays in the manager so a later reset can still clean up. It does not close
tabs on eviction.

---

## Session and tab lifecycle

- `browser.open(url)` creates a **new** tab and marks it kernel-owned. It must
  not silently reuse an existing user tab on the same URL.
- `browser.page(tabId?)` attaches to an existing tab; with no id it uses the
  active tab. Attaching does not make the tab kernel-owned.
- `browser.tabs()` returns `{id, url, title, active, owned}` objects.
- `browser.close(tabId)` closes only a kernel-owned tab. Closing a user tab is
  an error; use the low-level `browser` tool for that.
- `browser.closeAll()` closes all kernel-owned tabs and keeps the runtime and
  variables. Page handles for closed tabs become invalid.
- `browser_reset()` (tool) drops the runtime, closes every kernel-owned tab,
  and returns `{cleared, closedTabs, warnings}`.
- `browser_reset({tabId})` is **targeted cleanup**: close that one owned tab,
  remove it from the owned set, and leave unrelated variables intact. Page
  handles for the closed tab fail on the next call. If the tab is not owned,
  return an error and close nothing.
- On session deletion, close kernel-owned tabs best-effort. On an ordinary run
  end, do not close them: the chat session persists across messages and the
  user may want to inspect the result.

`browser.open` needs a `new_tab` flag on the navigate request. Without it the
desktop's existing navigate path may reuse a tab on the same URL, which would
make ownership wrong and could close a user tab on reset.

Phase 1 wire additions:

- `new_tab` on `BrowserActionRequest` (proto, Go wire struct, desktop request,
  desktop navigate branch) for `browser.open`.
- An internal `tabs_json` action so `browser.tabs()` gets objects instead of
  parsing the low-level tool's display string. The low-level `tabs` output
  stays unchanged.
- Selector fallback in the driver's click/type/select path; the request
  already has `selector`, so no new field is required.

Phase 2 adds locator and extract payloads, either as JSON fields on the
existing request or as typed proto messages if they are exposed to the public
browser tool.

---

## JavaScript API

Calls are synchronous. The final expression is the result; there is no
top-level `return` and no Promise wrapper. `print()` narrates intermediate
values, exactly like CUA.

```js
var page = browser.open("https://example.com");

page.waitFor({ text: "Products" });

var products = page.query(".product-card");
products;
```

```js
var page = browser.open("https://example.com");
page.goto("https://example.com/search?q=pixel");

page.type("#q", "Pixel 8a", { submit: true });
page.waitFor({ selector: ".s-main-slot" });

var rows = page.query(".s-result-item");
print("results: " + rows.count);
rows;
```

### `browser`

- `browser.open(url)` -> page
- `browser.page(tabId?)` -> page
- `browser.tabs()` -> array
- `browser.close(tabId)` -> `{closed}`
- `browser.closeAll()` -> `{closed}`

### `page`

Phase 1, using existing actions:

- `page.goto(url)` -> `{url, title}`
- `page.url()` / `page.title()`
- `page.snapshot({ scope? })` -> snapshot text
- `page.content(selector?)` -> text
- `page.query(selector)` -> parsed JSON from the `query` action
- `page.click(target)` -> `{ok, url, title}`
- `page.type(target, text, { submit? })` -> `{ok, url, title}`
- `page.select(target, value)` -> `{ok}`
- `page.waitFor(condition, { timeoutMs? })` -> `{ok, url, title}`
- `page.refresh()` -> re-read URL/title

Phase 2:

- `page.find(locator)` -> opaque handle (CSS/text/role/name/ref)
- `page.extract(schema)` -> structured data
- `page.paginate(options)` -> bounded multi-page extraction
- `page.scroll(options)`

Phase 3, advanced:

- `page.dismissOverlays(policy)`
- `page.evaluate(script)` — disabled in MVP; can bypass locator and risk checks
- `page.screenshot(options)`

### Targets

A target is one of:

```js
"#search"                                  // CSS, Phase 1
{ ref: "e12" }                             // snapshot ref, Phase 1
{ css: ".result-card" }                    // explicit CSS
{ text: "Add to cart" }                    // Phase 2
{ role: "button", name: "Search" }         // Phase 2
{ handle: <opaque page.find() result> }    // Phase 2
```

Phase 1 adds selector fallback to the driver's click/type/select path. The
request already carries `selector`; the driver currently ignores it for these
ops.

### Wait conditions

```js
page.waitFor({ selector: ".result" });           // visible, not merely attached
page.waitFor({ text: "Results" });
page.waitFor({ url: "/search" });                // substring for MVP
page.waitFor({ gone: ".loading" });              // Phase 2
page.waitFor({ predicate: fn });                 // not MVP: goja cannot see the DOM
```

`predicate` is deliberately excluded. A JS predicate runs in goja and has no
live DOM. Supporting it means sending code to the page (CSP, trust) or running
it against extracted data. Do not pretend the two are equivalent.

### Refs and locators

- Snapshot refs are per document and never recycled. They are **not** valid
  after navigation.
- A raw ref that no longer resolves must fail with the existing stale-ref hint.
  Do not silently remap `e12` to a new element by role/name; that can click the
  wrong product.
- Locators are re-resolvable. `page.click({ text: "Next" })` may resolve again
  after a DOM mutation; `page.click({ ref: "e12" })` may not.
- Automatic retry is allowed only when the failure happened **before** dispatch.
  A click whose result was lost after dispatch has unknown completion; report
  that and inspect fresh state instead of replaying it.
- `page.type` keeps the driver's clear-and-fill behavior, so retries do not
  append duplicate text.

### Locator handles

`page.find()` returns an opaque handle, not a DOM node. It carries the locator,
optional scope, tab id, and document generation. The host resolves it on each
use. Handles do not cross sessions and do not survive `browser_reset()`.

---

## Extraction and pagination

Phase 1 uses the existing `query` action, which returns compact JSON for up to
30 visible matches with `text`, `href`, `src`, `value`, `checked`, `disabled`,
`role`, and `tag`. That covers repeated cards and tables without a new driver
op.

Phase 2 adds a scoped extract op. A kernel alone does not fix the Amazon
related-price problem; the scope does.

```js
var rows = page.extract({
  scope: "#buybox, #corePrice_feature_div",
  items: ".offer",
  fields: {
    title: "h2",
    price: { selector: ".a-price .a-offscreen", optional: true },
    href: { selector: "a", attribute: "href" },
    tags: { selector: ".tag", all: true }
  },
  limit: 30
});
```

- One host call per page, not one call per field.
- Field spec: string = text; object supports `selector`, `attribute`, `all`,
  `optional`, and `trim`.
- Missing optional fields are `null`, never a fallback to a page-wide selector.
- Items and bytes are capped. No giant snapshot in the result.
- `scope` is explicit; without it the search is page-wide and more likely to
  pick unrelated products.

`page.paginate` is a bounded helper, not magic:

```js
var all = page.paginate({
  itemSelector: ".result",
  next: { text: "Next" },
  maxPages: 5,
  extract: {
    title: "h2",
    price: ".price"
  }
});
```

It stops on `maxPages`, a missing/disabled next control, an unchanged URL, a
repeated page signature, or the item/byte cap. It dedupes by a key field
(`href` when present, otherwise the full item), and returns
`{items, pages, truncated, warnings}`.

---

## Result and trace

`browser_run` returns one JSON object as text:

```json
{
  "result": {},
  "sessionId": "...",
  "activeTab": "...",
  "finalUrl": "...",
  "title": "...",
  "trace": [
    {"action": "click", "target": "Search", "status": "success", "ms": 42}
  ],
  "warnings": [],
  "errors": []
}
```

- URL/title come from the driver's live page identity, not the committed tab
  URL; that is the same stale-URL fix the browser tool now uses.
- Trace records action, short target description, status, duration, retries,
  overlay decisions, and risk refusals. Cap at 100 entries.
- Do not put page content or full snapshots in the trace.
- The rendered result text is capped (64 KB). If it is truncated, say so; the
  JS variables still hold the full value for the next call.
- Host failures throw catchable JS errors. Uncaught errors become a tool error
  with the JS stack and the trace so far.

---

## Limits and cancellation

Mirror CUA where it fits:

- code: 200 KB
- execution: 5 minutes default
- browser actions per evaluation: 100
- kernel-owned tabs: 5
- snapshot text: 24 KB (driver already caps results)
- rendered result: 64 KB
- trace: 100 entries
- pagination: 5 pages default, 20 hard cap

Cancellation propagates the evaluation context into `BrowserHandler`. A
cancelled/interrupted action may already have taken effect; report unknown
completion and inspect fresh state before retrying, never replay blindly.

---

## `/browser-use`

Mirror `/computer-use`:

- `internal/services/browseruse/invoke.go` with a narrow `SplitInvocation`
  (`/browser-useful` must not trigger).
- `internal/services/browseruse/SKILL.md` embedded by `skill.go`.
- `run/turns.go` marks the session, appends the skill to setup, and restores
  `browser_run` / `browser_reset` on later runs.
- Keep the low-level `browser` tool in the default inventory (the existing
  `browser-wkwebview-driver.md` Option A) so existing sessions do not break.
- The skill documents the sync API, no top-level `return`, target forms, ref
  staleness, extraction scope, pagination limits, risk rules, and reset
  semantics.

The skill should say plainly: use `browser` for one-off actions and
`browser_run` for multi-step workflows; do not use the kernel to complete
purchases, messages, account changes, downloads, or destructive actions.

---

## Implementation phases

### Phase 1 — Kernel skeleton over existing actions

- `browseruse` manager, per-session goja runtime, sync host bridge, limits,
  trace, cancellation.
- `browser_run` / `browser_reset`, `/browser-use` invocation, skill text.
- `browser.open`, `page`, `tabs`, `close`, `closeAll`; `new_tab` on navigate.
- Refs + CSS selector fallback for click/type/select.
- Existing `snapshot`, `content`, `query`, `wait_for`; parse `query` JSON.
- Reset ownership and targeted reset.
- Unit tests with a fake `BrowserHandler`.

This is the smallest slice that proves persistence, orchestration, cleanup, and
limits without changing the page model.

### Phase 2 — Locators, extract, pagination

- Driver `find`/locator support for text and role/name.
- Scoped `extract` op with fields, attributes, optional values, arrays.
- `page.find`, `page.extract`, `page.paginate`, `page.scroll`.
- Locator handles with generation checks; pre-dispatch retry only.
- Driver contract tests plus kernel fake-handler tests.

### Phase 3 — Safety and advanced surface

- Preflight target inspection for risk categories; `ConfirmationRequired`
  instead of dispatch. Still a guardrail unless tied to server approval.
- Per-action approval callback for risky operations, if we decide the kernel
  should support them at all.
- Overlay policy (`report` default, explicit cookie accept/reject; never
  auto-accept consent).
- `page.evaluate` behind an explicit advanced path.
- Screenshot and other visual helpers if they earn their place.

The low-level `browser` tool remains available throughout.

---

## Tests and acceptance

Server unit tests, no native browser:

- state persists across `browser_run` calls in one session and is isolated
  between sessions
- sync-only: `await`/Promise code fails with a clear message
- reset closes only owned tabs; targeted reset closes one; user tabs untouched
- `closeAll` keeps the runtime; full reset drops variables
- `new_tab` ownership is correct even when the same URL is already open
- action/timeout/page/byte limits, cancellation, unknown completion
- trace shape and cap; live URL/title in the result
- risk refusal (when Phase 3 lands) executes no click

Desktop driver tests if the driver changes:

- selector click/type resolves the first visible match
- stale ref after navigation returns the stale-ref hint
- scoped extract does not leak page-wide matches

Acceptance is manual and owned by the user, same as the driver plan: run the
desktop app, invoke `/browser-use`, browse/search/extract, call
`browser_reset`, and confirm only kernel-opened tabs closed. Automated tests
are the regression net, not the acceptance signal.

Specific commands during implementation:

```
cd apps/server-go && go test ./tests/services/ -run 'TestBrowserKernel' -v
cd apps/server-go && go test ./tests/tools/ -run 'TestBrowserTool' -v
cd apps/desktop && cargo test -p console-ui --test browser_driver_script_test
```

---

## Non-goals

- Replacing the low-level browser tool.
- Async/await or a general Node-like runtime.
- A DOM engine or a second browser.
- Fixing cross-origin iframes, trusted input, geo/profile identity, or CSP.
- Letting the kernel close or manage user tabs.
- Hard purchase/message/account safety in the first slice.

---

## Open points

1. Should `browser_reset({tabId})` be targeted cleanup (recommended here) or a
   full variable reset that closes only one tab? This plan chooses targeted
   cleanup.
2. Should kernel-owned tabs close when the chat session is deleted? Recommended
   yes, best-effort; never on an ordinary run end.
3. Do we eventually raise the low-level `browser` tool above `TierRead`, or add
   per-action approvals? The kernel guardrail is not meaningful while the
   escape hatch is unguarded.
4. Does `page.evaluate` belong at all, given it can bypass locator and risk
   checks? Keep it Phase 3 and advanced.
