# Phase 0 — spike: one real Console screen in TSX

**Goal**: prove the NSH workflow on Console's actual UI before any migration commitment. The
notes demo showed the mechanics; a chat transcript is the real test — long scrolling lists of
rich text, streaming updates, markdown, code blocks.

## Build

- A scratch app (not in `apps/`): `nsh init console-spike --template mixed` — a Rust crate that
  exports a fake "session" module (a `#[module]` that streams a scripted reply token by token,
  like `run/service.go` does) and the TSX screens that render it.
- One screen: the chat transcript — messages, markdown rendering (`TextView`), a code block
  (`Editor` read-only or `TextView`'s code rendering), streaming text arriving from Rust via
  callbacks, and an input with `mod+return` to send (`keymap`).

## Validate

- Streaming: Rust → TSX callbacks update one message without re-rendering the list.
- Scroll: `Scroll`/`List` behavior with hundreds of messages; `List` virtualization if needed.
- Styling: rebuild a Console-like look with Tailwind classes + theme tokens only.
- Tests: transcript renders, `mod+return` submits, streaming text lands — headless via `nsh test`.

## Done when

- The screen runs on web and headless with tests passing.
- We have a felt answer on the two open risks: TSX expressiveness for Console's UI, and
  NSH-on-QuickJS performance with a realistic transcript.
- A written go/no-go in this file.
