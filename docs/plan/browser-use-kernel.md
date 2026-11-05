# Browser JS kernel — what landed and what is next

The kernel exists: `internal/services/browseruse/`, always loaded (no
`/browser-use` needed). The low-level `browser` tool is gone — the kernel
tool **is** `browser`, one small schema plus a persistent `browser` object
per chat session, built on the same desktop bridge (`BrowserHandler`) the old
tool used. `browser_reset` cleans up.

What landed, briefly — the code and the `/browser-use` skill are the source
of truth:

- Console-kernel shape: flat `browser.*` methods, one object argument each,
  plain results, unknown members named in the error. Sync only (goja has no
  microtask pump); Promise completions are refused.
- Methods: `open`, `page`, `tabs`, `goto`, `snapshot`, `content`, `query`,
  `click`, `type`, `waitFor`, `close`, `closeAll`. Targets: snapshot refs and
  CSS selectors (driver resolves the first visible match without a snapshot).
- Ownership: `browser.open` navigates with a `new_tab` wire flag, so the tab
  is unambiguously the kernel's; reset closes only owned tabs (targeted
  `{tabId}` reset closes one and keeps variables; full reset drops the
  runtime). Owned-tab cap 5, action budget 100 per call, trace capped at 100
  riding every result.
- Read-tier by decision: browsing is observation, so plan mode can browse;
  page clicks are gated by the skill's risk rules (never purchase, message,
  account change, download, delete, consent), not the approval table.

## Left over from the first slice

- **Close owned tabs on session delete** (best-effort). Today they survive a
  deleted session until the server restarts. Needs a hook from the session
  service into `browseruse.Manager`; the handler broadcast only reaches a
  connected desktop, so the hook must accept best-effort failure.
- **Strict method arguments.** The console kernel rejects unknown keys with
  the accepted list (`methodArgs` in its runtime); the browser methods
  silently ignore them. Porting the console kernel's strictness here would
  catch `{projectId: ...}`-style mistakes.

## Phase 2 — locators, extract, pagination

The Amazon related-price problem is the driver: `query` is page-wide, so a
price selector leaks related products into the result. Phase 2 adds scope and
richer targets.

- **Locator targets** beyond refs and CSS: `{text: "Add to cart"}`,
  `{role: "button", name: "Search"}`. Driver-side find: re-resolvable, unlike
  snapshot refs, which die at navigation. A raw ref that no longer resolves
  still fails with the stale-ref hint — never silently remap by role/name;
  that can click the wrong product.
- **Scoped extract** — one host call per page, not one per field:

  ```js
  browser.extract({
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

  Field spec: string = text; object supports `selector`, `attribute`, `all`,
  `optional`, `trim`. Missing optional fields are `null`, never a fallback
  to a page-wide selector. Items and bytes capped; `scope` explicit.
- **Bounded pagination**:

  ```js
  browser.paginate({
    itemSelector: ".result",
    next: { text: "Next" },
    maxPages: 5,
    extract: { title: "h2", price: ".price" }
  });
  ```

  Stops on `maxPages` (5 default, 20 hard cap), a missing/disabled next
  control, an unchanged URL, a repeated page signature, or the item/byte cap.
  Dedupes by `href` when present, else the full item. Returns
  `{items, pages, truncated, warnings}`.
- **Locator handles** (`browser.find`) if one-off scripts need re-resolvable
  element references: opaque, per session, generation-checked, dropped on
  reset.

## Phase 3 — safety and advanced surface

- Preflight target inspection for the risk categories (money, communication,
  account/security, destructive, downloads, legal/consent): resolve the
  target first, and refuse with `ConfirmationRequired` before dispatch. Until
  risk is tied to a server approval, it stays a guardrail in the skill — the
  tool is read-tier, so the approval table cannot be the boundary here.
- Overlay policy: report-only by default; explicit cookie accept/reject only
  with an explicit instruction; never auto-accept consent.
- `page.evaluate` — advanced, can bypass locator and risk checks; decide
  whether it belongs at all before building it.
- Screenshot helpers (the wire already carries `imageBase64`).

## Non-goals (unchanged)

Async/await or a Node-like runtime; a DOM engine or second browser; fixing
cross-origin iframes, trusted input, geo/profile identity, or CSP; closing
tabs the kernel did not open.

## Tests

Kernel tests live in `internal/services/browseruse/kernel_test.go` (fake
desktop). Phase 2 adds driver contract tests in
`apps/desktop/crates/console-ui/tests/browser/`; acceptance of anything
visual is manual on the real desktop app.
