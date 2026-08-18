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
- optionally a content hash for stronger version detection;
- the returned line or byte ranges;
- whether the returned view was partial;
- whether the file was classified as text, binary, PDF, image, or notebook;
- the timestamp and request key for deduplication.

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

## 6. Deduplicate identical reads

For an unchanged file, key duplicate reads by:

- canonical path;
- `startLine` and `endLine`;
- encoding;
- relevant ceiling configuration;
- file `mtimeMs` and size.

On an identical repeat, return:

```text
Content unchanged since the last identical read; see the previous tool result.
```

The cache must be self-expiring and should consume the deduplication record after returning the stub. This avoids stale references after conversation compaction. If the previous result may no longer be visible, the tool should allow a fresh full read rather than trapping the model in repeated stubs.

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

Attach a compressed representation when the provider supports image content. If downscaled, state the scale factor. Use a quality ladder and enforce an output-size ceiling.

### Notebooks

For `.ipynb` files, render cells in a readable form, identify code/markdown/output cells, cap very large outputs, and attach plots as images when supported.

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

Resolve and validate paths consistently for all filesystem tools. Decide explicitly whether symlinks are allowed. If they are allowed, verify the resolved target remains inside the permitted workspace where applicable.

Normalize text by:

- stripping a UTF-8 BOM;
- converting CRLF and lone CR line endings to LF;
- preserving line numbering;
- never splitting a UTF-8 character during truncation.

Input aliases should be accepted where provider compatibility requires them:

- `filePath` → `file_path`/canonical internal path
- `target_file` → `file_path`
- `absolutePath` → `file_path`

Type coercion must be strict. Reject values such as `"2abc"` rather than silently converting them to `2`. Zod schemas should remain the final validation boundary.

## 10. Interaction with the agent harness

`executeTool` already performs Zod validation, permission resolution, cancellation, and output normalization. The read/write safety behavior belongs inside the filesystem tool context rather than in `executeTool`, because it depends on file-specific observations and must also protect direct tool use in tests.

The tool executor should continue to return actionable tool content. Expected read conditions such as truncation, empty files, PDFs, and binary files should generally not be marked as execution failures. Actual permission, I/O, or policy failures can continue to use `isError: true`.

The `Agent` currently constructs the tools used for a run and dynamically replaces the `subagent` tool. This is an appropriate integration point for constructing or attaching the file context. Ensure the context lifetime matches the intended session lifetime, not only one individual turn.

## 11. Recommended implementation order

Implement the smallest safe vertical slice first:

1. Add streaming reads.
2. Add the three ceilings.
3. Add exact truncation messages and resume lines.
4. Add empty-file, EOF, BOM, CRLF, and dangerous-path handling.
5. Add the per-agent observation ledger.
6. Guard `writeFile`, `editFile`, and `batchWrite`.
7. Add deduplication.
8. Add filename repair.
9. Add format-specific handling for binary files, PDFs, images, and notebooks.
10. Add structured result metadata if provider conversions can preserve it.

## 12. Focused tests

Extend `apps/server/tests/tools.test.ts`, or add a focused filesystem-tool test file, with these cases:

- empty file returns `File is empty`;
- `startLine` past EOF returns the total line count and recovery advice;
- `startLine > endLine` is rejected clearly;
- the line ceiling returns the exact next `startLine`;
- the byte ceiling returns the exact next `startLine`;
- a long line is visibly truncated without invalid UTF-8;
- CRLF input is normalized and line numbers remain correct;
- a UTF-8 BOM is removed from displayed content;
- unchanged identical reads return the deduplication stub;
- a changed file invalidates the observation and deduplication record;
- a partial read prevents full-file `writeFile`;
- a partial read prevents unsafe `batchWrite` replacement;
- a covered, unchanged exact region can be edited safely;
- an unchanged file with an ambiguous edit is rejected;
- `/dev/zero` and similar special files are blocked;
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
- [ ] Images, notebooks, SVG, PDFs, and binary files have defined behavior.
- [ ] Dangerous paths are blocked before I/O.
- [ ] BOM and line endings are normalized.
- [ ] UTF-8 characters are not split during truncation.
- [ ] Line numbers are 1-indexed and match `cat -n`.
- [ ] Common parameter aliases are supported where needed.
- [ ] Numeric coercion is strict.
- [ ] Focused tests cover all safety and recovery behavior.

These changes should make the filesystem tools more predictable, reduce token waste and retry loops, and prevent destructive writes based on incomplete or stale file context.
