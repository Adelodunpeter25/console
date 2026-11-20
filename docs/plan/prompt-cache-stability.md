# Prompt Cache Stability

Status: **in progress** (2026-11-20). Done: G1, G2, G3, C1, X2, A1, D2. Open: G4, G5, C2, A2, A3, D1, D3.

Notes on what shipped:
- G1/G2: setup is frozen per session (rebuilt only when the enabled capability set changes). Mode notice, `$chat` refs, late capability references and background work go in `TurnRequest.Tail`, appended after the cache breakpoint (Claude) or as a final message (other providers). System prompt no longer contains the approval mode.
- C1/X2: main runs use long retention: Claude sends `ttl: "1h"`, Codex and OpenCode send 24h retention. Subagents and helper calls still use their own retention.
- A1: Antigravity identity persists in `antigravity-sessions.json` in the console storage dir.
- D2: Devin's router assignment is reused per conversation until its JWT nears expiry.
- G3: each session keeps a compaction checkpoint (`compaction.Checkpoint`). Once compaction runs, later turns and runs append new messages to that exact compacted prefix instead of recompacting from the full history, so the cache breaks once per compaction. A checkpoint is dropped if history no longer matches (rewind, edit, fork). It is in memory, so a restart recompacts once. Emergency (overflow) compaction is unchanged.
