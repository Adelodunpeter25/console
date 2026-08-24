# Roadmap

High-level direction only — implementation details live in `docs/`.

## Cross-chat memory

- [ ] `$` references: type `$conversation-title` or `$<session-id>` in any chat to attach that conversation's context
- [ ] `read-chat` skill — agent tool to fetch the transcript of another chat (resolves `$` refs)
- [ ] Session search — fuzzy match on titles/ids when resolving `$` references
- [ ] Scope control — only conversations from the same project are referenceable by default

## Agent capabilities

- [ ] Browser use — agent controls a built-in browser on desktop (navigate, click, type, screenshot)
- [ ] Computer use — agent controls the computer (keyboard, mouse, screen)

## Accounts

- [ ] Multi-account support — log into multiple accounts per provider and switch between them; `?active` marks the account in use

## Developer experience

- [ ] Server path aliases — `@/…` imports like mobile already has; eliminate fragile `../../` chains
