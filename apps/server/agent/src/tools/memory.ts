/**
 * Persistent cross-session memory tool. One `memory.db` per project
 * (`<storage>/projects/<projectId>/memory.db`) plus a shared
 * `memory-global.db` for facts that aren't tied to a specific project.
 * No embeddings — recall is deterministic keyword/tag matching (see
 * `agent/src/memory/search.ts`).
 */
import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { memoryRegistry, recallMemories, type MemoryRegistry, type MemoryScope } from "@/agent/src/memory/index.js";

const inputSchema = z.object({
  op: z
    .enum(["store", "recall", "list", "edit", "delete"])
    .describe("Operation to apply: 'store', 'recall', 'list', 'edit', or 'delete'"),
  content: z
    .string()
    .optional()
    .describe("Memory content (required for 'store'; new content for 'edit')"),
  tags: z
    .array(z.string())
    .optional()
    .describe("Tags for filtering (used with 'store', 'edit', 'recall', or 'list')"),
  scope: z
    .enum(["project", "global"])
    .optional()
    .describe(
      "Defaults to 'project'. 'project' = this project's own memory; 'global' = shared across every project. " +
        "Use 'global' only for facts that aren't tied to this project's code (e.g. user identity/preferences).",
    ),
  query: z.string().optional().describe("Free-text search term (used with 'recall')"),
  id: z.string().optional().describe("Memory id (required for 'edit' and 'delete'; must match the entry's scope)"),
});

type Input = z.infer<typeof inputSchema>;

function errorResult(message: string) {
  return { content: [{ type: "text", text: `Error: ${message}` }], isError: true };
}

function textResult(text: string) {
  return { content: [{ type: "text", text }] };
}

function renderEntries(title: string, entries: { id: string; content: string; tags: string[] }[]) {
  if (entries.length === 0) {
    return textResult(`${title}:\n(No memories found)`);
  }
  const lines = entries.map((entry) => {
    const tagSuffix = entry.tags.length > 0 ? ` [${entry.tags.join(", ")}]` : "";
    return `- ${entry.id}: ${entry.content}${tagSuffix}`;
  });
  return textResult(`${title}:\n${lines.join("\n")}`);
}

export function createMemoryTool(projectId: string | null, registry: MemoryRegistry = memoryRegistry): AgentTool {
  return {
    name: "memory",
    description:
      "Manage persistent memory that survives across sessions. Operations: store, recall, list, edit, delete. " +
      "Use 'store' only when the user explicitly asks you to remember something, or clearly confirms/corrects a " +
      "non-obvious approach — not for routine task details or anything derivable from the code. Use 'recall' " +
      "(free-text query and/or tags) or 'list' (browse by tag) to check memory before assuming. Defaults to " +
      "'project' scope; use 'global' only for facts that aren't tied to this project's code.",
    tier: "write",
    inputSchema,
    execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
      const { op, content, tags, query, id } = args;
      const scope: MemoryScope = args.scope ?? "project";

      let store;
      try {
        store = registry.resolve(scope, projectId);
      } catch (err) {
        return errorResult(err instanceof Error ? err.message : String(err));
      }

      if (op === "store") {
        if (!content) {
          return errorResult("'store' operation requires 'content'.");
        }
        const entry = store.store({ content, tags });
        return textResult(`Stored memory ${entry.id} (scope: ${entry.scope}).`);
      }

      if (op === "recall") {
        if (!query && (!tags || tags.length === 0)) {
          return errorResult("'recall' operation requires 'query' and/or 'tags'.");
        }
        const entries = store.list();
        const matches = recallMemories(entries, { text: query, tags });
        return renderEntries(`Recalled memories (scope: ${scope})`, matches.map((match) => match.entry));
      }

      if (op === "list") {
        const entries = store.list(tags && tags.length > 0 ? { tags } : undefined);
        return renderEntries(`Memories (scope: ${scope})`, entries);
      }

      if (op === "edit") {
        if (!id) {
          return errorResult("'edit' operation requires 'id'.");
        }
        if (!content && !tags) {
          return errorResult("'edit' operation requires 'content' and/or 'tags' to update.");
        }
        const updated = store.update(id, { content, tags });
        if (!updated) {
          return errorResult(`Memory ${id} not found in scope '${scope}'.`);
        }
        return textResult(`Updated memory ${updated.id}.`);
      }

      if (op === "delete") {
        if (!id) {
          return errorResult("'delete' operation requires 'id'.");
        }
        const removed = store.remove(id);
        if (!removed) {
          return errorResult(`Memory ${id} not found in scope '${scope}'.`);
        }
        return textResult(`Deleted memory ${id}.`);
      }

      return errorResult(`Unknown operation.`);
    },
  };
}

/** Default project-less instance for static registration in `allTools`. */
export const memoryTool = createMemoryTool(null);
