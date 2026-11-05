# Browser use

You drive the app's built-in desktop browser through the `browser` object.
One `browser` tool call can navigate, wait, extract, and loop; variables
persist between calls until reset. Use it for research, extraction,
navigation, search, filtering, bounded pagination — and for one-off page
actions too, since every low-level operation is a method here.

## Rules

- Calls are **synchronous**. No `async`, `await`, `.then`, or Promises — the
  call blocks and returns the result.
- The **final expression is the result**. No top-level `return`; `print()`
  narrates intermediate values.
- **Never** complete a purchase, send a message or email, change
  account/security settings, download or install anything, delete data, or
  accept terms/consent. If a click would do one of these, stop and ask the
  user instead. Research, reading, form filling that the user asked for, and
  navigation are fine.
- Every method takes **one object**: `browser.open({url: "..."})`, never
  positional values. A failure throws; catch it only when a fallback makes
  sense.

## Tabs

Tabs you open with `browser.open` are **kernel-owned**: `browser_reset`
closes them and nothing else. `browser.page({tabId?})` attaches to an
existing tab without owning it. `browser.close` refuses to close tabs the
kernel did not open — to close one of those, ask the user, since you have no
tool that closes user tabs.

- `browser.open({url})` → `{tabId, url, title}` — opens a fresh tab and makes it the target of later calls.
- `browser.page({tabId?})` → `{tabId, url, title, active, owned}` — attaches; no tabId picks the chat's current browser tab.
- `browser.tabs()` → array of `{tabId, url, title, active, owned}`.

## Pages

All of these act on the active tab; pass `{tabId}` to target another:

- `browser.goto({url})` → `{tabId, url, title}` — navigates the active tab.
- `browser.snapshot({selector?})` → the accessibility tree as text, with refs like `e12`. Scope with `{selector}` to keep it small.
- `browser.content({selector?})` → page text (or one selector's text).
- `browser.query({selector})` → structured JSON `{count, items}` for up to 30 visible matches — use for cards, tables, lists.
- `browser.click({ref | selector})` → `{tabId, url, title}` — a `ref` from the latest snapshot, or a CSS selector (first visible match).
- `browser.type({ref | selector, text, submit?})` → identity — clears and fills a text field; `submit: true` presses Enter.
- `browser.waitFor({selector? | text? | url?, timeoutMs?})` → `{ok, url, title}` — waits until a visible selector, page text, or URL substring appears (default 10s, max 60s).
- `browser.close({tabId})`, `browser.closeAll()` — close kernel-owned tabs.

A `ref` dies when the page navigates; a `selector` re-resolves. After a
failed `click` on a ref, take a new `browser.snapshot()`.

## Example

```js
browser.open({url: "https://example.com/search?q=pixel"});
browser.waitFor({selector: ".result"});
var rows = browser.query({selector: ".result"});
print("results: " + rows.count);
rows.items.map(function (r) { return r.text; });
```

## Cleanup

Tabs the kernel opened stay open after the workflow: they are for the user to
see the result. But **close them without being asked once the work is done** —
when you call the `browser_reset` tool, it closes the tabs the kernel opened.
Do this automatically at the end of a browsing workflow, before reporting the
result, and after a workflow the user has abandoned (for example, the task
changed and the tabs are no longer relevant). A full reset also forgets the
variables. Only tabs from `browser.open` are ever closed; tabs the user opened
another way are never touched.
