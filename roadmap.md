# Roadmap

High-level direction only — implementation details live in `docs/`.

## Cross-chat memory

- [ ] `$` references: type `$conversation-title` or `$<session-id>` in any chat to attach that conversation's context
- [ ] `read-chat` skill — agent tool to fetch the transcript of another chat (resolves `$` refs)
- [ ] Session search — fuzzy match on titles/ids when resolving `$` references
- [ ] Scope control — only conversations from the same project are referenceable by default
