# BatchWrite Harness Fix Plan

## Symptom
Agent sends `batchWrite({ files: [{ path, content }, ...] })` per tool schema, but server rejects with `files.0: Expected object, received string` — validator expects map form. Agent falls back to sequential `writeFile` calls (more round-trips, same bytes).

## Root Cause
* Client schema (`packages/types/src/tool.ts` / tool registry in `apps/server/agent/src/tools/index.ts`) advertises `files: {path, content}[]`.
* Server handler `apps/server/agent/src/tools/batch-write.ts` (and `write-file.ts`) validates with `zod.record(zod.string())` or unwrapped array — shape mismatch. The AI SDK passes the array, harness unwraps one level differently than advertised.

## Files
* `apps/server/agent/src/tools/batch-write.ts` — zod schema + handler.
* `apps/server/agent/src/tools/index.ts` — tool registration / description.
* `packages/types/src/tool.ts` / `apps/server/agent/src/types/tool.ts` — shared Tool type.
* `apps/server/agent/src/service/tool-executor.ts` — validation before execution.
* `apps/server/api/src/services/run-tools.ts` — alternative validation path.

## Fix
1. Pick single canonical shape — recommend array `files: Array<{path:string, content:string}>` (matches current client, easier to extend with `mode`/`encoding`).
2. Update server zod to `z.array(z.object({path: z.string(), content: z.string()}))` and adjust handler loop accordingly.
3. Add backward compat: accept both shapes — if `files` is record, convert `Object.entries(files).map(([path,content])=>[path,content])` before validate, then deprecate map form.
4. Regenerate / verify `packages/types` export aligns.

## Alternative (if map preferred)
* Keep map `files: Record<string,string>` — update client tool description to `files: {"<path>":"<content>"}` and update agent prompt example. Requires one doc update, smaller diff.

## Test
* Unit: `batchWrite` with array shape succeeds, no fallback to `writeFile`.
* Manual: agent creates 2+ files in one turn — single `batchWrite` call, both files on disk, no `files.0` error in server log.
* `bun tests/run-tools.test.ts` or equivalent if exists.

## Rollout
* Single commit, no migration — runtime fix. After deploy, remove fallback individual writes in agent loop if desired.
