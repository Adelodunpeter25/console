# Roadmap

High-level direction only — implementation details live in `docs/`.

## Cross-chat memory

- [ ] `$` references: type `$conversation-title` or `$<session-id>` in any chat to attach that conversation's context
- [ ] `read-chat` skill — agent tool to fetch the transcript of another chat (resolves `$` refs)
- [ ] Session search — fuzzy match on titles/ids when resolving `$` references
- [ ] Scope control — only conversations from the same project are referenceable by default

## Agent capabilities

- [x] Browser use — agent controls a built-in browser on desktop (navigate, click, type, screenshot)
- [x] Computer use — agent controls the computer (keyboard, mouse, screen)
- [ ] Codemode — the agent writes JavaScript that calls the harness's own tools, so one turn can run hundreds of them; dry-run and verification built in (`docs/plan/codemode.md`)

## Accounts

- [ ] Multi-account support — log into multiple accounts per provider and switch between them; `?active` marks the account in use
