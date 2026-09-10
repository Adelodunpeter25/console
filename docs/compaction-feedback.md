# Context Compaction Report

## User Feedback

I do not like the current compaction implementation.

The desktop app is now being used full time, so compaction behavior is no longer an occasional edge case. When an agent compacts, it loses important context. This is especially noticeable when the lost information comes from work completed as recently as the prompt immediately before compaction.

That behavior is not acceptable for normal desktop use. A compaction should preserve the active task state and the most recent work, rather than making the agent forget what it just did.

## Observed Problem

- The agent can lose context from the most recent prompt and the work performed in response to it.
- Recent decisions, file changes, command results, and unresolved details may disappear after compaction.
- The resulting session feels as though the agent has forgotten active work, even though that work happened immediately before compaction.
- This is particularly disruptive in long-running desktop sessions, where the user expects the agent to maintain continuity across many turns.

## Impact

Compaction currently damages task continuity instead of only reducing older, less important history. The desktop experience becomes unreliable because the user may need to repeat information or re-establish the state of work that was just completed.

The priority is therefore preserving recent and active context. Any future compaction behavior must be evaluated against whether the agent can continue the task without losing the immediately preceding work.
