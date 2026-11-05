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
  positional values. Unknown keys are refused with the accepted list, so a
  typo fails loudly. A failure throws; catch it only when a fallback makes
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
- `browser.scroll({direction: "up" | "down" | "top" | "bottom", amount?})` → `{scrollY, scrollHeight, viewportHeight, atTop, atBottom, tabId}` — scrolls the page; `amount` is pixels (default: most of a viewport).
- `browser.click({ref | selector | text | role, name | handle})` → `{tabId, url, title}`.
- `browser.type({ref | selector | role, name | handle, text, submit?})` → identity — clears and fills a text field; `submit: true` presses Enter.
- `browser.waitFor({selector? | text? | url?, timeoutMs?})` → `{ok, url, title}` — waits until a visible selector, page text, or URL substring appears (default 10s, max 60s).
- `browser.close({tabId})`, `browser.closeAll()` — close kernel-owned tabs.

### Targets

`click` and `type` take exactly **one** target, in one of five forms:

- `{ref: "e12"}` — a ref from the latest `browser.snapshot()`. Refs die when
  the page navigates; after a failed ref click, take a new snapshot.
- `{selector: "#buy button.buy"}` — CSS, first visible match.
- `{text: "Add to cart"}` — first visible element whose accessible name or
  text equals the string (a substring match is the fallback). Prefer exact
  product-page text: a loose `{text}` can match the wrong control.
- `{role: "button", name: "Search"}` — ARIA role (as `browser.snapshot`
  prints it, e.g. `button`, `link`, `textbox`, `combobox`) with an optional
  accessible name.
- `{handle: "h1"}` — a handle from `browser.find` (below).

Never guess a locator from memory: read it from a snapshot or a `query`.

### Handles

```js
var h = browser.find({text: "Add to cart"});   // or {selector} / {role, name}
// h → {handle: "h1", tabId, url, target}
browser.click({handle: h.handle});
```

A handle re-resolves the locator on every use, so it survives page
re-renders that would kill a ref. It is dropped when its tab navigates (the
handle is pinned to the URL it was found on) — `browser.find` again. At most
32 live handles; `browser.close`/`browser_reset` drop them.

### Structured extraction

`browser.extract` pulls fields out of a list in **one** call, scoped so
related-products blocks and other page noise stay out:

```js
browser.extract({
  scope: "#buybox, #corePrice_feature_div",  // alternatives, tried in order
  items: ".offer",                            // rows inside the scope
  fields: {
    title: "h2",                              // string: element text
    price: {selector: ".a-price .a-offscreen", optional: true},
    href:  {selector: "a", attribute: "href"},
    tags:  {selector: ".tag", all: true}      // array of matches
  },
  limit: 30
});
// → {count, items: [{title, price, href, tags}, ...], truncated?}
```

A missing field is `null` — never a fallback to a page-wide selector. Fields
take `{selector, attribute?, all?, optional?, trim?}`; at most 16 fields and
200 items per call. Without `{items}` the scope itself becomes one row; a
scope that matches nothing errors.

### Bounded pagination

`browser.paginate` walks a paginated list: extract page, click next, repeat —
deduplicating by `href` when a field of that name exists, else by full item:

```js
browser.paginate({
  itemSelector: ".result",
  next: {text: "Next"},          // or {selector} / {role, name}
  maxPages: 5,                   // default 5, hard cap 20
  extract: {title: "h2", price: ".price", href: {selector: "a", attribute: "href"}}
});
// → {items, pages, truncated, warnings?}
```

It stops on the page budget, a missing or disabled next control, a page that
never changes after the click, or the caps (500 items / 200 KB) — `truncated`
says the caps bit, `warnings` say why it stopped. Each page costs a couple of
host calls; keep `maxPages` honest.

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
