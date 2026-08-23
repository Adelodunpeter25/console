# High-quality `readFile` implementation

This document describes the proposed implementation for the local agent filesystem tools in `apps/server/agent`. It is based on the practical summary inspired by Command Code's approach and is tailored to this repository's agent harness.

## Goal

Return only useful parts of a file to the model, provide clear recovery signals on failure, and avoid silent token waste, accidental overwrites, and deadlocks. This is especially valuable for smaller or open models and for sessions where token cost matters.

The implementation applies to:

- `apps/server/agent/src/tools/read-file.ts`
- `apps/server/agent/src/tools/write-file.ts`
- `apps/server/agent/src/tools/edit-file.ts`
- `apps/server/agent/src/tools/batch-write.ts`
- `apps/server/agent/src/tools/index.ts`
- the `Agent`/session tool-construction path

## Current codebase findings

The current implementation is intentionally simple, but it has the main risks this design addresses:

- `readFile` uses `fs.readFile()` and loads the entire file before slicing lines.
- There are no line, byte, or line-width ceilings.
- Base64 reads are completely unbounded.
- A default read can return the entire file.
- Empty files produce effectively blank output.
- Past-EOF reads return an `isError: true` result and an `Error:` message rather than a recovery instruction.
- Text is decoded without BOM removal or CRLF normalization.
- Missing filenames are not repaired or searched for alternatives.
- Binary, PDF, image, SVG, and notebook files are not special-cased.
- `writeFile`, `editFile`, and `batchWrite` can overwrite files after the agent has only seen a partial read.
- The tools are exported as shared singleton objects through `allTools`.

The most important architectural constraint is therefore:

> The read ledger must be session/agent-scoped, not module-global, because `allTools` currently exposes shared tool instances.

A module-global ledger could leak observations between users or agent sessions and could incorrectly authorize a write based on another session's read.

## 1. Enforce three independent ceilings

Every text read must apply all three limits. No read path may return unbounded content.

Recommended initial values:

| Ceiling | Recommended value | Purpose |
| --- | ---: | --- |
| Maximum lines | 2,000 | Controls normal large source files |
| Maximum bytes | 128 KiB | Controls wide or binary-like content |
| Maximum characters per line | 2,000 | Controls minified JavaScript and long log lines |

The limits are independent. A read stops as soon as any ceiling is reached. Explicit user ranges do not bypass these safety limits.

For base64 output, apply an equivalent byte ceiling to the encoded result and clearly state that the binary content was not fully returned. Prefer returning metadata or directing the model to a suitable binary tool instead of returning large base64 payloads.

### Long-line behavior

Do not silently omit a long line. Truncate the displayed line at the character ceiling and append a marker such as:

```text
[line truncated at 2000 characters]
```

Never split a UTF-8 character while enforcing byte limits.

## 2. Always return actionable feedback

Tool output should never be an empty string or a generic failure. Human-readable text should not begin with `Error:` for expected recovery conditions; models often treat that prefix as a hard failure and retry blindly.

Examples:

```text
File is empty: /path/to/file
```

```text
Offset beyond EOF. The file has 120 lines. Try a smaller startLine.
```

```text
Showing lines 1–2000 of 8400.
Output truncated at line 2000. Resume with startLine=2001.
```

```text
Output truncated by the byte limit. Resume with startLine=1847.
```

```text
This is a PDF. Use pdftotext or a PDF tool to extract its contents.
```

```text
Binary file (MIME: application/octet-stream). Cannot display as text.
```

For machine consumers, return structured metadata in addition to rendered text:

```ts
{
  content: string,
  startLine: number,
  endLine: number,
  totalLines?: number,
  truncated: boolean,
  resumeFrom?: number,
  sizeBytes: number,
  mtimeMs: number,
  kind: "text" | "binary" | "pdf" | "image" | "notebook"
}
```

The existing `AgentTool` interface permits arbitrary tool output, so this metadata can be introduced without changing the core tool executor. The existing `ToolResult` wrapper can continue to carry the rendered content.

## 3. Define offset and range semantics

Use the existing `startLine` and `endLine` parameters for text files:

- line numbers are 1-indexed;
- numbering must match `cat -n`, editor line references, and stack traces;
- `startLine` is inclusive;
- `endLine` is inclusive;
- omitted `startLine` means line 1;
- omitted `endLine` means read until a ceiling is reached;
- a ceiling always wins over the requested range.

If byte offsets are later exposed, make them separate parameters. Do not overload a line offset with byte semantics.

The resume value must be computed from the actual line that was successfully returned. It must not be an estimate based only on bytes.

## 4. Stream and read in chunks

Do not load a multi-hundred-megabyte file into memory to return the first 2,000 lines.

The text reader should:

1. Open the file after validating the path.
2. Read bounded chunks from the file.
3. Decode safely across chunk boundaries.
4. Normalize BOM and line endings.
5. Track the current line, byte count, and line character count.
6. Stop immediately when any ceiling or requested end line is reached.
7. Detect whether more content remains, including when a boundary falls exactly at the end of a chunk.
8. Capture post-read metadata and invalidate the view if the file changed during the read.

The implementation should avoid `fullText.split("\\n")` for large files. A streaming line reader or an incremental `StringDecoder` is preferable.

## 5. Maintain a session/agent-scoped partial-read ledger

Track a small ledger per canonical file path and agent/session. It should record:

- the canonical path;
- file `mtimeMs` and size at read time;
- a content hash of the observed bytes — **required**, not optional: `mtimeMs` + size can miss same-millisecond rewrites, and `editFile`'s exact-match-uniqueness guarantee depends on it;
- the returned line or byte ranges;
- whether the returned view was partial;
- whether the file was classified as text, binary, PDF, image, or notebook;
- the observation timestamp;

Example shape:

```ts
interface FileObservation {
  path: string;
  mtimeMs: number;
  sizeBytes: number;
  coveredLineRanges: Array<[number, number]>;
  complete: boolean;
  kind: "text" | "binary" | "pdf" | "image" | "notebook";
  observedAt: number;
}
```

The ledger should merge adjacent or overlapping ranges. A file is complete only when the entire text file has been observed, not merely because the requested range ended at the apparent last line.

### Where the ledger belongs in this codebase

`allTools` currently contains shared tool objects. Do not put mutable ledger state directly in `read-file.ts` at module scope.

Prefer a per-agent factory or context:

```ts
const fileContext = createFileToolContext();
const tools = createFileTools(fileContext);
```

The `Agent` instance owns conversation history and is the natural owner of this context. Subagents should receive their own context unless they are intentionally sharing the parent agent's file observations.

### Write protection

Before `writeFile`, `editFile`, or `batchWrite` mutates an existing file:

- check the current file version against the observation;
- reject a full rewrite when only a partial view was observed;
- reject or require a fresh read if the file changed since observation;
- perform the check before creating directories or writing data;
- invalidate the observation after a successful write.

For `editFile`, a range-based edit can be allowed when the affected region was observed, the exact match is unique, and the file version is unchanged. The existing exact-string and occurrence checks remain valuable.

New files with no prior version can be created normally. `batchWrite` must apply the same checks independently to every existing target.

Never-read policy (previously unspecified, must be explicit):

- new files with no prior version -> create normally;
- existing file with no observation -> `editFile` requires a fresh read first; `writeFile` / `batchWrite` fall through to the normal permission prompt instead of a hard block.

## 6. Deduplicate identical reads — deferred

**Deferred — do not implement in v1.**

The stub ("see the previous tool result") breaks after conversation compaction: the ledger lives on the Agent, compaction happens in the agent loop, and nothing tells the ledger its referenced tool result is gone. The "self-expiring" escape hatch had no defined trigger mechanism.

A repeated identical read costs tokens once; a stale stub can trap the model in a retry loop. Revisit only if repeated-read waste is observed in practice, and only with an explicit compaction-aware invalidation signal.

## 7. Repair common filename problems

Before returning a missing-file response:

1. Normalize Unicode, including NFC/NFD differences.
2. Replace common typographic variants such as narrow no-break spaces and curly quotes.
3. Try a small, bounded set of candidate spellings.
4. Offer a `did you mean?` suggestion using substring matching and Levenshtein distance less than or equal to 2.

Suggestions must remain within the allowed working directory and must not turn path repair into an unrestricted directory scan.

Example:

```text
File not found: src/servce/index.ts
Did you mean: src/service/index.ts?
```

## 8. Special-case common formats

### Images

**Deferred to a separate task.** Attaching image content blocks requires multimodal plumbing through `normalizeToolOutput` and the `streamFn` message mapping — not tool-local work. v1 behavior: detect image MIME and return kind + size metadata.

### Notebooks

**Deferred to a separate task.** v1 behavior: `.ipynb` is treated as structured JSON text under the normal ceilings.

### SVG

Treat SVG as text. It is XML and should be subject to the normal text ceilings.

### PDF

Return a clear instruction to use `pdftotext` or a PDF-capable tool. If conversion is available in the execution environment, provide a bounded extracted-text view.

### Binary files

Return MIME type and size only unless the caller explicitly requests a bounded supported representation. Do not decode arbitrary binary data as UTF-8 and send it to the model.

## 9. Safety and hygiene

Block dangerous paths before any I/O, including examples such as:

- `/dev/zero`
- `/dev/urandom`
- `/proc/.../fd/*`
- other special devices and unbounded pseudo-files

Resolved policy: symlinks are followed, but the resolved realpath must remain inside the permitted workspace root. Note macOS `/tmp` is itself a symlink — test fixtures must use realpath-resolved temp directories.

Normalize text by:

- stripping a UTF-8 BOM;
- converting CRLF and lone CR line endings to LF;
- preserving line numbering;
- never splitting a UTF-8 character during truncation.

**No input aliases in v1.** Add an alias only when a provider is observed actually emitting it; alias preprocessing weakens the strict Zod validation boundary.

Type coercion must be strict. Reject values such as `"2abc"` rather than silently converting them to `2`. Zod schemas should remain the final validation boundary.

## 10. Interaction with the agent harness

`executeTool` already performs Zod validation, permission resolution, cancellation, and output normalization. The read/write safety behavior belongs inside the filesystem tool context rather than in `executeTool`, because it depends on file-specific observations and must also protect direct tool use in tests.

The tool executor should continue to return actionable tool content. Expected read conditions such as truncation, empty files, PDFs, and binary files should generally not be marked as execution failures. Actual permission, I/O, or policy failures can continue to use `isError: true`.

Integration point: tools are bound per run in `run.service.ts` via `boundTools` (which already swaps `ask` / `askMany` / `todo` onto shared instances). The file-tool factory is wired there; the resulting context must be keyed by session (like `todoLists`) so the ledger survives across runs within a session, and subagents get their own context unless intentionally sharing the parent's observations.

## 11. Recommended implementation order

**Task 1 — contained in `read-file.ts` plus small shared path utils; no architectural change:**

1. Streaming reads with safe cross-chunk decoding.
2. The three independent ceilings.
3. Exact truncation messages with computed resume lines.
4. Empty-file, past-EOF, BOM/CRLF normalization, dangerous-path blocking.
5. Binary/PDF/SVG classification messages (no attachments).
6. Structured result metadata alongside rendered text.

**Task 2 — ledger + guards; blocked on the never-read policy above; requires the `allTools` singleton → factory refactor:**

1. Per-session observation ledger, wired in `run.service.ts`.
2. Write guards in `writeFile`, `editFile`, `batchWrite` (never-read policy + version/hash checks).
3. Filename repair with bounded suggestions.

**Deferred:** deduplication (§6), image/notebook attachments (§8), input aliases (§9).

## 12. Focused tests

Extend `apps/server/tests/tools.test.ts`, or add a focused filesystem-tool test file, with these cases:

- empty file returns `File is empty`;
- `startLine` past EOF returns the total line count and recovery advice;
- `startLine > endLine` is rejected clearly;
- the line ceiling returns the exact next `startLine`;
- the byte ceiling returns the exact next `startLine`;
- a long line is visibly truncated without invalid UTF-8;
- a multi-byte UTF-8 character straddling the byte-ceiling chunk boundary is not split and the resume point names the correct next line;
- CRLF input is normalized and line numbers remain correct;
- a UTF-8 BOM is removed from displayed content;
- a changed file invalidates the stored observation (hash/mtime mismatch);
- a partial read prevents full-file `writeFile`;
- a partial read prevents unsafe `batchWrite` replacement;
- a covered, unchanged exact region can be edited safely;
- an unchanged file with an ambiguous edit is rejected;
- `/dev/zero` and similar special files are blocked;
- a symlink resolving outside the workspace root is rejected;
- missing filenames produce bounded `did you mean?` suggestions;
- PDFs and binary files return actionable classification messages;
- base64 output is bounded;
- concurrent sessions do not share ledger observations.

Per the repository working rules, run only the relevant focused test file, for example:

```sh
cd apps/server && npx tsx tests/tools.test.ts
```

Do not run `run-all-tests.ts` or the full suite unless explicitly requested.

## Minimal acceptance checklist

- [ ] Three independent ceilings apply to every read.
- [ ] No read returns unbounded content.
- [ ] Every truncation includes an exact resume location.
- [ ] Empty files and EOF reads return useful non-generic messages.
- [ ] The partial-read ledger records file version and covered ranges.
- [ ] The ledger is session/agent-scoped, never module-global.
- [ ] Writes are rejected when the agent has only seen an unsafe partial view.
- [ ] File changes invalidate observations.
- [ ] Identical unchanged reads are deduplicated with self-expiring state.
- [ ] Filename normalization and bounded suggestions are implemented.
- [ ] Reads are streamed and stop at the first ceiling.
- [ ] SVG, PDF, and binary files have defined v1 behavior; images/notebooks classified with metadata (attachments deferred).
- [ ] Dangerous paths are blocked before I/O.
- [ ] BOM and line endings are normalized.
- [ ] UTF-8 characters are not split during truncation.
- [ ] Line numbers are 1-indexed and match `cat -n`.
- [ ] Numeric coercion is strict.
- [ ] Focused tests cover all safety and recovery behavior.

These changes should make the filesystem tools more predictable, reduce token waste and retry loops, and prevent destructive writes based on incomplete or stale file context.
