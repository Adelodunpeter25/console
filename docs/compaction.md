# Advanced Context Compaction Architecture & Roadmap

This document outlines the upgrade path for the `console` context compaction engine, inspired by the state-of-the-art implementation in `oh-my-pi`.

---

## 1. Executive Summary & Goals

Currently, `console` features a basic structural compaction pass (`compactHistory`) that trims older turns when total estimated tokens exceed 85% of the model's context window. 

While functional, production multi-turn agent sessions encounter several critical failure modes:
1. **Context Overflows Mid-Turn**: A bulky tool execution (e.g. `grep` returning 500 lines or reading multiple large files) pushes the conversation over the limit before a turn finishes.
2. **Provider Rejections (400 context_length_exceeded)**: If token estimation is slightly off, the provider fails without automatic recovery.
3. **Incomplete Output Truncation**: When the context limit cuts off an assistant response (`stopReason: "length"`), the session gets stuck.
4. **Loss of Verbatim History in Summaries**: Pure LLM summarization discards exact line references, error traces, and parameters.
5. **Static Context Windows**: Context limits need to be dynamically discovered from local provider configs and model catalogs (e.g. OpenCode, Ollama, Antigravity, OpenAI Codex).

The goal of this upgrade is to evolve `console` into an **advanced, resilient context maintenance system**.

---

## 2. Core Architectural Pillars

```
                     ┌─────────────────────────────────────────┐
                     │            Session Lifecycle            │
                     │  (Pre-Turn, Mid-Turn, Post-Turn, Error) │
                     └────────────────────┬────────────────────┘
                                          │
                   Threshold / Overflow / Incomplete Trigger
                                          │
                                          ▼
                     ┌─────────────────────────────────────────┐
                     │          Compaction Pipeline            │
                     │             (Method Order)              │
                     └────────────────────┬────────────────────┘
                                          │
          ┌───────────────────────────────┼───────────────────────────────┐
          │                               │                               │
          ▼                               ▼                               ▼
    Tier 1: Shake                 Tier 2: Snapcompact             Tier 3: Summarize / Handoff
 (Mechanical Elision)           (Visual Image Archival)            (LLM / Structural Checkpoint)
  - Zero-latency                 - Dense bitmap PNG frames          - Fast auxiliary model
  - Replaces bloated tools       - 8x13 pixel font                  - Cumulative file tracker
  - Evicts dead outputs          - Vision-token efficient           - Strict role alternation
```

---

## 3. Detailed Components & Implementation Spec

### 3.1. Multi-Stage Pipeline (`methodOrder`)

Compaction should execute through a prioritized sequence rather than jumping straight to expensive summarization:

```ts
export type CompactionMethod = "shake" | "snapcompact" | "summarize" | "handoff";

export interface CompactionConfig {
  enabled: boolean;
  maxThresholdRatio: number; // default: 0.85
  keepRecentTokens: number;  // default: 20_000
  methodOrder: CompactionMethod[]; // ["shake", "snapcompact", "summarize"]
  shake: ShakeConfig;
  snapcompact?: SnapcompactConfig;
}
```

#### A. Tier 1: Shake (Mechanical Elision)
- **Concept**: Inspect older turns for giant tool results (e.g. `read_file`, `grep_search`, `run_command` stdout).
- **Execution**:
  - Replace raw output exceeding character threshold (e.g. > 4,000 chars) with a lightweight pointer/stub:
    `[Output omitted: 14,230 characters from grep_search. Available upon re-querying path: src/index.ts]`
  - Superseded read operations (reading file `A.ts` at turn 2, then editing `A.ts` at turn 5) prune the older read result entirely.
- **Benefit**: Reclaims 30–60% of context instantly without calling an LLM or losing conversation flow.

#### B. Tier 2: Snapcompact (Visual Archival)
- **Concept**: For vision-capable models (Claude 3.5/3.7, GPT-4o, Gemini 1.5/2.0), serialize discarded text turns onto tightly packed bitmap PNG images using high-density bitmap fonts.
- **Execution**:
  - Convert older conversation turns into PNG frames.
  - LLMs with vision process high-resolution images for a fraction of the token cost of raw text (e.g., Gemini fixed token budget per image, Claude patch pricing).
  - Preserves verbatim code diffs, logs, and error messages visually.
- **Fallback**: Skips automatically if the active model does not support image inputs.

#### C. Tier 3: Summarization & Handoff
- **Concept**: If `shake` and `snapcompact` do not reclaim enough tokens or are unavailable, invoke structured summarization.
- **Execution**:
  - Use fast/cheap auxiliary model or optimized structural summarizer.
  - Retain the cumulative `<files>` prefix tree (Read, Written, Edited).
  - Guarantee provider-compliant role alternation (`[User summary, Assistant ack, ...recentMessages]`).

---

### 3.2. Lifecycle Triggers & Self-Healing

Compaction must not only run *after* a turn, but also *mid-turn* and on *error*:

| Trigger | Description | Action |
| :--- | :--- | :--- |
| **Post-Turn Maintenance** | After assistant response finishes, if `estimatedTokens >= contextWindow * 0.85`. | Runs pipeline; prepares compacted context for next user turn. |
| **Mid-Turn Maintenance** | During an ongoing tool execution loop, before issuing the next model request. | Checks token usage between tool results. If over limit, shakes previous tool outputs immediately to avoid HTTP 400. |
| **Overflow Recovery** | Catches `context_length_exceeded`, `prompt_too_long`, or HTTP 400/413. | Pops the failing prompt, applies aggressive shake + compaction, and retries the turn automatically. |
| **Incomplete Recovery** | Catches `stopReason === "length"`. | Trims the partial assistant turn, compacts context, and requests model continuation. |

---

### 3.3. Dynamic Model Context Window Discovery

Instead of hardcoded context limits, context windows must be resolved dynamically:

```ts
export interface ModelContextMetadata {
  contextWindow: number;
  maxOutputTokens: number;
  supportsVision: boolean;
}
```

1. **Catalog Resolution**:
   - Query runtime model providers (e.g. OpenCode, Ollama `/api/show`, Anthropic, OpenAI, Antigravity API).
   - Read context limits directly from runtime provider metadata if reported.
2. **Default Fallback Matrix**:
   - Modern frontier models default to 128k–1M tokens.
   - Local models (Ollama/llama.cpp) resolve from the server's configured `num_ctx`.

---

### 3.4. Session Storage & Non-Destructive In-Memory Projection

To align with the design where compaction does **not** corrupt user-facing chat logs:
- SQLite retains full conversation history for user review and transcript export.
- Compaction operates as an **ephemeral in-memory projection** prepared by the server agent runtime immediately before dispatching to the LLM.
- Checkpoints are saved as internal entries (`CompactionEntry`) referencing `firstKeptMessageId`.

---

## 4. Phased Implementation Roadmap

### Phase 1: Mechanical Tool Output Elision (Shake)
- [ ] Implement `pruneToolOutputs` and `shakeConversation` in `apps/server/agent/src/compaction/shake.ts`.
- [ ] Add tool result truncation and superseded-read eviction.
- [ ] Wire shake into `shouldCompact()` and `compactHistory()`.

### Phase 2: Mid-Turn & Recovery Lifecycle
- [ ] Add pre-request context threshold check inside the agent tool execution loop (`agent.ts`).
- [ ] Add automatic catch-and-retry handler for provider context overflow errors (`context_length_exceeded`).
- [ ] Handle `stopReason === "length"` automatic continuation.

### Phase 3: Dynamic Model Context Resolution
- [ ] Create `resolveModelLimits(modelId, provider)` utility with auto-detection for OpenCode, Ollama, and cloud providers.
- [ ] Wire dynamic limits into `Agent` initialization.

### Phase 4: Snapcompact / Visual Archival (Optional Extension)
- [ ] Evaluate TypeScript/Node canvas or native image generation for visual history archiving on vision models.
