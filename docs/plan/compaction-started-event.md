# Compaction Started Event (deferred — for later discussion)

Status: **not started**. Companion to `docs/plan/context-overflow-compaction-resilience-plan.md`, kept separate on purpose.

## Problem

Compaction work is silent while it runs. The server emits exactly one `compaction` event, only *after* the work finishes (`apps/server/agent/src/service/agent-loop.ts`). Consequences observed:

- LLM/smol summaries take tens of seconds (measured ~42s on a real 876-message session) with zero progress events — both apps look stuck.
- Mid-turn shake and overflow recovery are likewise invisible until done.
- Mobile drops the `compaction` event entirely (`apps/mobile/utils/chat-events.ts` `default:` branch) — not even the after-the-fact notice.
- Desktop shows a post-hoc notice (`state/run.rs` → "Context compacted: …"), cleared on next turn.

## Proposal (not yet agreed)

- Emit a lightweight `compaction-started` event (or extend the existing event with a phase) at each trigger point: pre-turn, mid-turn, overflow-retry. Fields: `trigger`, `tier`, maybe rough `tokensBefore`.
- Optionally route smol summarizer deltas into the visible stream as thinking text so long compactions show progress.
- Mobile: handle the event (spinner/notice) instead of dropping it.
- Desktop: show a "compacting…" state until the existing notice replaces it.

## Acceptance (to define when scheduled)

- A 40s LLM compaction shows continuous progress UI on both clients, never a frozen screen.
- No behavior change for structural path (still instant, single event).
- Abort during a signaled compaction keeps working.
