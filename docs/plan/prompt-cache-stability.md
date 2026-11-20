# Prompt Cache Stability

Status: **in progress** (2026-11-20). Done: G1, G2, C1, X2, A1, D2. Open: G3, G4, G5, C2, A2, A3, D1, D3.

Notes on what shipped:
- G1/G2: setup is frozen per session (rebuilt only when the enabled capability set changes). Mode notice, `$chat` refs, late capability references and background work go in `TurnRequest.Tail`, appended after the cache breakpoint (Claude) or as a final message (other providers). System prompt no longer contains the approval mode.
- C1/X2: main runs use long retention: Claude sends `ttl: "1h"`, Codex and OpenCode send 24h retention. Subagents and helper calls still use their own retention.
- A1: Antigravity identity persists in `antigravity-sessions.json` in the console storage dir.
- D2: Devin's router assignment is reused per conversation until its JWT nears expiry.
- G3 still open and likely the next biggest win: compaction is recomputed from full history every turn once past the threshold (stored history is never rewritten), so the compacted prefix can shift each turn. Needs a stored compaction boundary reused until the next threshold crossing.

## Goal

Raise the prompt-cache hit rate for every provider (Claude, Antigravity, Devin,
Codex, OpenCode) so each turn pays for only the newest exchange. The daemon log
showed 90-96% overall, with the misses concentrated in a few places.

## Findings (from ~/.console/logs/daemon.log and the code)

- Claude: some runs read ~695 tokens and write 64k-193k within 1-2 minutes of the
  previous run. Tools and system prompt stayed cached; everything from the setup
  message onward was rewritten.
- `loop.WithSetup` puts the per-session setup text at the head of the conversation on
  every request. Several things edit it between runs: background-work results,
  `<referenced_chats>`, newly enabled capability references, the per-mode built-in
  skills list, and capability-load failures (appended to the user message).
- Plan/full-access changes the approval-mode and critical sections inside the system
  prompt (prompt-cache key includes mode), so a mode switch rewrites the whole prefix.
- Claude sends a 5-minute `ephemeral` marker only. Antigravity session identity is
  in memory only. Devin marks only the system prompt and may re-route the model
  per request. Antigravity cold starts line up with idle gaps and restarts, not
  with steer/abort (the code path leaves the prefix untouched).
- Reference implementation: pi (packages/ai, packages/coding-agent) appends changes
  as new blocks at the end, keeps top-level request fields fixed, warms the cache
  before expiry, and uses the 1h TTL.

## Principles

1. The head of the request (tools, system prompt, setup, early history) must be
   byte-identical between turns of a session. Only appends are allowed.
2. Anything that changes per turn or per mode rides in a message at the tail.
3. A full rewrite is only acceptable at compaction.

## Global fixes

### G1. Freeze setup per session; send changing context at the tail
- Build setup once per session (kept with the prompt cache) and reuse it unchanged.
- Move `<background-work>`, `<referenced_chats>`, capability references enabled
  after the first request, and load failures out of setup into a trailing
  context message that is sent but never persisted.
- Capability references for capabilities already on at session start stay in the
  frozen setup.

### G2. Mode-neutral system prompt
- Drop approval mode from the system prompt and from the prompt-cache key.
- Announce the active mode (and mode changes) in the trailing context message.
- Built-in skills list becomes mode-independent; mode gating is stated at the tail.

### G3. Compaction is the only planned rewrite
- Store the compaction result and reuse it on later turns; do not recompute the
  compacted prefix every turn once past the threshold.

### G4. Deterministic tool order
- Tool definitions keep one stable order for the whole session; groups loaded
  mid-run append only.

### G5. Cache warmer
- Replay the last request with a one-token output cap shortly before the cache
  entry expires, only when expected saving exceeds cost, and stop after idle.
  Modeled on pi's `cache-warming.ts`.

## Provider-specific fixes

### Claude
- C1. Send `ttl: "1h"` on the cache_control blocks (main conversation only).
- C2. Keep `thinking` and `output_config.effort` fixed for a session where
  the API allows; otherwise accept one miss per level change.
- C3. Keep the three breakpoints (last tool, system, newest message).

### Antigravity
- A1. Persist session id / trajectory id / step counter so restarts keep identity.
- A2. Check whether the endpoint accepts a `cachedContent` reference for very large
  contexts.
- A3. Confirm per-request fields (requestId, last_step_index) stay out of cached content.

### Devin
- D1. Check whether `ChatMessagePrompt` supports a cache marker; if so, mark the last message.
- D2. Pin the router-assigned model for the session so the cache is not split per model.
- D3. Keep ExecutionId out of anything cache-keyed.

### Codex / OpenCode
- X1. Already send a conversation-scoped `prompt_cache_key`; they only need G1-G4.
- X2. Send 24h retention on OpenCode by default.

## Order of work

1. G1, G2 (largest effect, all providers)
2. G3, G4
3. Claude C1, then Antigravity A1, Devin D2
4. G5 cache warmer
5. Other provider items, then add logging to confirm the numbers

## Verification

- Existing tests per area, run one file at a time (loop, run, providers).
- New tests assert that two consecutive requests share a byte-identical prefix.
- After rollout, compare daemon.log hit rates per provider with the baseline above
  (antigravity ~90-96%, claude ~91%, devin/codex/opencode unmeasured).