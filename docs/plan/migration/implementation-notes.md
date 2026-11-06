# NSH migration — implementation notes

Platform findings and workarounds from the migration spikes, against NSH v0.4.0. Each entry says
what we saw, what causes it (checked in NSH's source), and what we do about it. New findings go
on top.

## 1. Web target drops a controlled `Input`'s app-side value updates

**Seen in**: phase 0 spike, web target (`nsh run --target web`). The headless runner
(`nsh test`) is unaffected.

**Symptom**: after `send()`, `draft.set("")` clears the app's state but the browser field keeps
the typed text; the next `onChange` reports the old text plus what was typed since, so the second
send goes out with the stale text prepended.

**Root cause** (verified in NSH source and against the live page with an instrumented field):

- The web host reconciles a controlled input with an edit-count guard: each user edit increments
  `field._count` (`textChanged` in `nsh-platform-web/web/nsh.js`), and an incoming value op is
  applied only when its `count >= field._count` (`op_iv` in the same file). A value based on an
  older count is treated as stale typing and dropped. Sound design — **if** the core's count
  tracks the user's edits.
- On native platforms the engine does that bookkeeping: `nsh-engine/src/lib.rs` calls
  `core.input_changed(id, text, count)` for every `ChangeText` event before dispatching it, so the
  core's `input.count` matches the field and the guard passes for app-side sets.
- On the web target the core is the wasm build, and its event entry
  (`nsh_event` → `dispatch_event` in `nsh-platform-web/src/lib.rs` and
  `nsh-core/src/core/components.rs`) never calls `input_changed` — nothing in the wasm path
  updates `input.value`/`input.count`. The core therefore believes the input still holds the last
  value **the app set**, and:
  - the app's cleared value is emitted with `count: 0`, which the web host drops (`0 < 10`);
  - confirmed live: after typing 10 characters and submitting, `field._count` is 10 and **no
    `iv` op arrives at all** when the app clears — the core saw `input.value == ""` (stale) and
    skipped the emit as a no-op.

**Upstream fix** (one-line shape): where the wasm core dispatches events
(`dispatch_event`/`nsh_event`), call `input_changed(id, text, count)` for `ChangeText` exactly as
`nsh-engine` does for native hosts. Native paths are unaffected (they call it already).

**Our workaround**: none in app code is clean — the value never reaches the DOM. Until the fix
lands (patch the local checkout, or upstream), a chat composer on web should be written so stale
text is harmless: read the prompt from the field's own `onChange` payload (we do), and treat the
composer's contents as "append whatever is typed" rather than assuming a clear. Alternatively
remount the `Input` per send (heavy; not worth it in the spike).

## 2. Percentage max-widths collapse layout on the web target

**Seen in**: phase 0 spike, web target. Headless rendering is unaffected (no layout assertions).

**Symptom**: chat bubbles styled `max-w-[75%]` (Tailwind arbitrary percentage) inside a
`flex flex-row` row rendered as one-character-wide vertical text columns — the bubble's content
box measured to ~0 width. Changing only the caps to pixels (`max-w-[520px]`) fixed every bubble
with no other change.

**Suspected cause** (not root-caused): percentage resolution against the flex row's indefinite
width during the web runtime's measure pass. Native and headless targets resolved the same class
fine.

**Our workaround**: avoid percentage max-widths in TSX for now; use pixel caps (or
`max-w-*`/`basis-*` theme scales). Re-check when we build the real workspace screens; if it
persists it needs an upstream report with a minimal repro.

## 3. `mod+return` shortcuts need `{ when: "always" }` while a composer is focused

By default a keymap binding does not fire while a text input holds focus if the input uses the
keystroke — and a single-line input uses Return (submit). A chat "Cmd+Enter to send" binding must
be `keymap.bind("mod+return", send, { when: "always" })`. Not a bug: documented default, easy to
miss. Our desktop habit from GPUI is the same shape, so this is just a note.

## 4. No `key` prop on mapped views (yet)

`messages().map(...)` with `key={m.id}` warns "unsupported prop `key`"; list diffing is
positional. For a transcript where messages are append-only this is fine; if we ever reorder or
splice lists mid-stream, watch for stateful children (focused inputs, expanded rows) losing
themselves. `List` may take keys — re-check when building the transcript for real.
