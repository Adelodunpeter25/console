# Roadmap

High-level direction only — implementation details live in `docs/`.

## Cross-chat memory

- [x] `$` references: type `$conversation-title` or `$<session-id>` in any chat to attach that conversation's context
- [x] `read-chat` skill — agent tool to fetch the transcript of another chat (resolves `$` refs)
- [x] Session search — fuzzy match on titles/ids when resolving `$` references
- [x] Scope control — only conversations from the same project are referenceable by default

## Navigation

- [x] Sidebar session search — inline search in the desktop sidebar; matches titles across all projects, including settled chats, through the server (`GET /api/sessions?query=`)
- [x] Command palette chat listing — open any chat from ⌘K, including chats the sidebar has not loaded, by searching titles on the server (`search_palette_chats` → `GET /api/sessions?query=`)
- [ ] Search chat contents — extend sidebar search beyond titles to message text

## Agent capabilities

- [x] Browser use — agent controls a built-in browser on desktop (navigate, click, type, screenshot)
- [x] Computer use — agent controls the computer (keyboard, mouse, screen)
- [x] Plan and Full Access modes — two permission modes, Plan by default; Shift+Tab toggles on desktop, a Plan chip on mobile
- [x] Codemode — the agent writes JavaScript that calls the harness's own tools through a `code` tool backed by a persistent per-session REPL; the harness tool list collapses to the REPL tools (default path)
- [x] Automations — scheduled and triggered agent runs
- [x] Pull requests — read and write pull requests from the agent and the apps (`docs/plan/pull-requests.md`)

## Accounts

- [x] Multi-account support — log into multiple accounts per provider and switch between them; `?active` marks the account in use

## Remote access

- [x] iroh tunnel — encrypted peer-to-peer connection between desktop/Android and the server, the default, with the relay as fallback
