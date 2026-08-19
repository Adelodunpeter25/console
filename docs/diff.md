# File-diff rendering for the mobile app

This document specifies how the mobile app should compute and render file-edit diffs in the chat transcript. The backend is **not** involved — it already persists the tool-call arguments (`oldContent` / `newContent` for `editFile`, `files[].content` for `batchWrite` / `writeFile`) and the tool result. All diff logic is client-side.

## Why

Today the mobile app renders every tool call the same way through `ToolActivityRow` (`apps/mobile/components/chat/message-bubbles.tsx`):

- Collapsed: status icon + tool name + `DONE` / `FAILED` badge
- Expanded: a single monospaced text block of the raw `detail` string

For `editFile` the detail is the backend's result summary (e.g. `Edited: /path\n  Replaced 3 line(s) with 5 line(s) (+2 lines)`). The actual `oldContent` / `newContent` strings are never shown — the arguments are dropped by `ToolResultItem` because it only reads `result.content`, not `call.arguments`.

So the user sees "Edit File · DONE" with no way to inspect what changed. This is the gap the diff view fills.

## Backend data already available

The `ToolCall` type (`packages/types/src/tool.ts`) carries the full arguments:

```ts
interface ToolCall {
  id: string;
  name: string;
  arguments: unknown; // already contains oldContent/newContent for editFile
  thoughtSignature?: string;
}
```

The `editFile` tool schema (`apps/server/agent/src/tools/edit-file.ts`):

```ts
{
  path: string;
  cwd?: string;
  oldContent: string;
  newContent: string;
}
```

The `writeFile` / `batchWrite` tools carry full file content (not a diff), so they need a different treatment — see below.

## What to build

### 1. A diff utility module

Create `apps/mobile/utils/diff.ts`. This module takes `oldContent` and `newContent` (both strings) and produces a line-level diff — a list of hunks where each line is tagged as `added`, `removed`, or `context`.

Recommended approach: use a lightweight line-diff algorithm. The `diff` npm package (`diffLines`) is the standard choice and works in React Native. Alternatively, implement a minimal LCS-based line diff inline if a new dependency is unwanted.

The output type:

```ts
type DiffLine =
  | { type: "added"; text: string; oldLineNo?: number; newLineNo: number }
  | { type: "removed"; text: string; oldLineNo: number; newLineNo?: number }
  | { type: "context"; text: string; oldLineNo: number; newLineNo: number };

interface DiffResult {
  lines: DiffLine[];
  addedCount: number;
  removedCount: number;
}
```

### 2. A `DiffView` component

Create `apps/mobile/components/chat/diff-view.tsx`. This renders a `DiffResult`:

- Each line gets a left gutter character (`+` for added, `-` for removed, space for context).
- Added lines: green text / green-tinted background (`theme.colors.status.ready` / `readyBg`).
- Removed lines: red text / red-tinted background (`theme.colors.status.attention` / `attentionBg`).
- Context lines: default text color, slightly muted.
- Line numbers shown in a narrow left column (old number for removed/context, new number for added/context) — optional but helps navigation.
- Monospaced font (`font-mono`), small text size (`text-[12px]`), matching the existing `ToolActivityRow` detail styling.
- Collapsible if long — show first N lines with a "show full diff" toggle, since mobile screens are narrow and diffs can be hundreds of lines.

A summary badge at the top: `+{addedCount} -{removedCount}` in green/red.

### 3. Wire it into the tool-call rendering

The current rendering path:

```
MessageBubble → AssistantBubble → ToolActivityRow (name only)
MessageBubble → ToolResultItem → ToolActivityRow (result.content only)
LiveToolResults → ToolResultItem → ToolActivityRow (result.content only)
```

The problem: `ToolActivityRow` and `ToolResultItem` receive only the tool name and the result text. They never see `call.arguments`, so they can't compute a diff.

Changes needed:

1. **Thread `arguments` through.** The `AssistantBubble` already extracts `toolCalls` from the message content, but it maps them to `{ name: string }` only — it drops `arguments`. Change the mapping to preserve `arguments`:

   ```ts
   const toolCalls = (content ?? [])
     .filter((c) => c.type === "toolCall" && c.call)
     .map((c) => c.call as { name: string; arguments?: unknown });
   ```

2. **Match tool calls to results by ID.** Currently `ToolResultItem` renders results independently and `ToolActivityRow` renders calls independently. To show a diff you need both the `arguments` (from the call) and the `isError` status (from the result) in one place. Match them by `toolCallId` before rendering, the same way the GPUI desktop app does in `attach_results_by_id`.

3. **Special-case edit tools.** Add a branch: if `call.name === "editFile"` and `arguments.oldContent` / `arguments.newContent` are present, render `DiffView` inside the expanded section instead of the raw `detail` text. Other tool names keep the current rendering.

4. **Collapsed summary.** On the collapsed `ToolActivityRow` header, show the `+N -M` badge next to `DONE` / `FAILED` for edit calls. Compute it from the diff once (cache in a `useMemo` keyed by `call.id`).

### 4. writeFile / batchWrite

These tools write entire files, not patches. Two options:

- **Option A (simple):** Treat the entire `content` as "added" lines. Show `+{lineCount} -0` and render all lines green. This is what Aider does for new files.
- **Option B (better, needs old file content):** Show a real diff against the previous file content. But the client doesn't have the old content — the backend reads it server-side and doesn't send it back. Implementing this would require either (a) the backend returning the old content in the result, or (b) the client tracking file content across reads. Both add complexity. **Recommend Option A for now** and revisit if the backend changes to return old content.

For `batchWrite`, render one `DiffView` per file in the batch.

### 5. Error cases

When `editFile` fails (no match, multiple matches), the result `isError` is true and the result text explains the failure. In this case, show the error text as-is (current behavior) and skip the diff — there's nothing to diff because no edit happened, and `oldContent` / `newContent` are still in the arguments but the file wasn't changed.

## Files to touch

| File | Change |
|------|--------|
| `apps/mobile/utils/diff.ts` | **New.** Line-diff computation. |
| `apps/mobile/components/chat/diff-view.tsx` | **New.** `DiffView` component. |
| `apps/mobile/components/chat/message-bubbles.tsx` | Thread `arguments` into `ToolActivityRow`; add edit-tool branch to render `DiffView`; add `+N -M` badge on collapsed row. |
| `apps/mobile/components/chat/live-tool-results.tsx` | If results are matched to calls here, thread arguments through too. |
| `apps/mobile/components/chat/index.ts` | Export `DiffView`. |

No backend changes. No new types — `ToolCall.arguments` is already `unknown` and just needs to be passed through and parsed.

## Dependency choice

- **`diff` package** (MIT, ~30KB): provides `diffLines(oldStr, newStr)`. Works in React Native. This is the path of least resistance.
- **Inline implementation**: a ~80-line LCS line diff. No dependency, full control, but more code to maintain and test.

Recommend the `diff` package unless there's a constraint on adding dependencies. It's battle-tested and the API maps directly to the `DiffResult` shape above.

## Edge cases

- **Large diffs:** Cap rendered lines (e.g. 200) with a "diff truncated" notice. Mobile memory and screen real estate are limited.
- **Multi-line `oldContent` / `newContent`:** The diff handles this naturally — `diffLines` splits on newlines.
- **Identical old/new (no-op edit):** Show `+0 -0` and a muted "no changes" label.
- **Binary content in arguments:** Unlikely for `editFile` (it's string-based), but guard against non-string `arguments` with a type check before diffing.
- **Streaming:** While a tool call is still running (no result yet), don't render a diff. Show the current "Running…" state. The diff renders once the result arrives.
