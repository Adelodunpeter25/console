# CCA 400 Error: "This model does not support assistant message prefill"

## Symptom

A run fails with an HTTP 400 from the Cloud Code Assist (CCA) / Antigravity endpoint:

```
Error: CCA request failed (400 Bad Request): {
  "error": {
    "code": 400,
    "message": "{\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"This model does not support assistant message prefill. The conversation must end with a user message.\",\"request_id\":\"...\"},\"status\":\"INVALID_ARGUMENT\"}"
  }
}
```

Observed on Antigravity Claude models (`claude-sonnet-4-6`, etc.) after the model
spawns 3 subagents and they finish working.

## Root cause

CCA uses Gemini `generateContent` semantics: every request's `contents` array must
end with a `user` (or tool-result) turn. The model rejects any conversation whose
final turn is `role: "model"` ("assistant prefill" = starting/ending a request with
an assistant turn).

In `apps/server/providers/src/shared/convert-messages.ts`:

- `AssistantMessage` → `role: "model"` (text / functionCall parts)
- `ToolResultMessage` → `role: "user"` (functionResponse parts)
- **Empty user messages are skipped** (line 73-75) and empty toolResult arrays are
  skipped (line 116-119), so a trailing turn can be silently dropped.

If history's last message is an assistant reply (e.g. `stopReason: "stop"` text
turn with no trailing tool call), `convertMessages` produces a `contents` array
ending in `model` → 400.

## Why subagents trigger it

1. Parent loop (`apps/server/agent/src/service/agent-loop.ts`) pushes an assistant
   message with `subagent` tool calls, then a `toolResult` message → wire history
   ends with `user`, fine.
2. The `subagent` tool (`apps/server/agent/src/tools/subagent.ts`) collapses the
   whole subagent conversation into ONE text result.
3. The parent's follow-up reply after those results is a pure text turn
   (`stopReason: "stop"`) — no further tool calls, so no trailing `toolResult`.
4. That final assistant message is the last message when the session is resumed →
   request ends with `model` → 400.

Normal tool-use chains end with tool results; a subagent conclusion doesn't, which
is why this surfaces after multi-subagent runs.

## Fix direction (not yet applied)

- In `convert-messages.ts`, after building `contents`, drop trailing
  `role: "model"` entries (or append a synthetic `role: "user"` turn) to satisfy
  the "must end with user message" invariant.
- Note: `convertMessages` is shared by gemini, antigravity, and possibly other
  providers — verify each consumer.
- Secondary contributor: the empty-user-message skip can leave a dangling model
  turn if history ends `[assistant, user(empty)]`.

## Related observations

- In `convert-messages.ts` line ~113, `makeFunctionResponsePart(r.toolCallId, ...)`
  passes the tool-call id as the `name` arg (first positional param) — appears to
  be a copy-paste slip; `functionResponse.name` will equal the tool-call id rather
  than the tool name. Harmless-ish but worth fixing.
