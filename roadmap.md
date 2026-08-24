# Roadmap

High-level direction only — implementation details live in `docs/`.

## Cross-chat memory

- [ ] `$` references: type `$conversation-title` or `$<session-id>` in any chat to attach that conversation's context
- [ ] `read-chat` skill — agent tool to fetch the transcript of another chat (resolves `$` refs)
- [ ] Session search — fuzzy match on titles/ids when resolving `$` references
- [ ] Scope control — only conversations from the same project are referenceable by default

## Security & remote access

- [ ] Auth token required on every API request (daemon executes bash — never ship unauthenticated)
- [ ] Documented tunnel recipes: Tailscale / Cloudflare Tunnel for "connect from anywhere"
- [ ] `console service install` — launchd/systemd auto-start + crash recovery
- [ ] LAN auto-discovery (mDNS) so clients find the daemon without typing IPs

## Agent quality

- [ ] Task 2 of `docs/read-file-tool-implementation.md`: per-session read ledger, write/edit guards against stale or partial views, filename repair ("did you mean?")
- [ ] Deferred readFile features: identical-read dedup (compaction-aware), image & notebook attachments
- [ ] Plan-mode state machine completion (see `docs/plan-mode-state-machine.md`)

## Platform & distribution

- [x] Bun runtime migration (`docs/bun-migration.md`)
- [x] Single multi-call binary + rolling release + installer
- [ ] Mobile parity gaps (`docs/mobile-parity.md`)
- [ ] Typed client/server API contracts (evaluate Elysia + Eden)
- [ ] Linux ARM64 server builds alongside x64/macOS

## Under consideration

- Windows daemon support (depends on Bun.Terminal platform coverage)
- Shared team sessions / multi-client live sync on one daemon
