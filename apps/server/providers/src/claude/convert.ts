/**
 * Converts AgentMessage[] / AgentTool[] to the Anthropic Messages wire
 * format (`POST /v1/messages`).
 *
 * Mapping:
 *   UserMessage       → role: "user", content blocks (text + images)
 *   AssistantMessage  → role: "assistant", content blocks (text + tool_use)
 *   ToolResultMessage → role: "user", content blocks (tool_result)
 */
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentMessage, AgentTool, CacheRetention } from "@console/types";

export interface ClaudeMessage {
  role: "user" | "assistant";
  content: Array<Record<string, unknown>>;
}

export interface ClaudeTool {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
  cache_control?: { type: "ephemeral" };
}

/** Placeholder for empty tool results — the Messages API rejects empty content. */
export const EMPTY_TOOL_RESULT_TEXT = "Tool failed with no output.";

function toolResultText(content: unknown): string {
  const text = typeof content === "string" ? content : JSON.stringify(content);
  return text.trim() === "" ? EMPTY_TOOL_RESULT_TEXT : text;
}

function parseToolInput(input: unknown): Record<string, unknown> {
  if (input && typeof input === "object" && !Array.isArray(input)) {
    return input as Record<string, unknown>;
  }
  if (typeof input === "string" && input.trim() !== "") {
    try {
      const parsed: unknown = JSON.parse(input);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>;
      }
    } catch {
      // Fall through to empty input below.
    }
  }
  return {};
}

function normalizeImageMime(mimeType: string): "image/jpeg" | "image/png" | "image/gif" | "image/webp" | undefined {
  const normalized = mimeType.trim().toLowerCase();
  if (normalized === "image/jpg") return "image/jpeg";
  if (
    normalized === "image/jpeg" ||
    normalized === "image/png" ||
    normalized === "image/gif" ||
    normalized === "image/webp"
  ) {
    return normalized;
  }
  return undefined;
}

export function convertClaudeMessages(messages: AgentMessage[], cacheRetention?: CacheRetention): ClaudeMessage[] {
  const turns: ClaudeMessage[] = [];

  for (const msg of messages) {
    if (msg.role === "user") {
      const text = msg.content ?? "";
      const attachments = msg.attachments ?? [];
      if (text.trim() === "" && attachments.length === 0) continue;
      const content: Array<Record<string, unknown>> = [];
      if (text.trim() !== "") {
        content.push({ type: "text", text });
      } else {
        content.push({ type: "text", text: "(see attached images)" });
      }
      for (const att of attachments) {
        const mediaType = normalizeImageMime(att.mimeType);
        if (!mediaType) {
          content.push({ type: "text", text: `[unsupported image: ${att.mimeType}]` });
          continue;
        }
        content.push({
          type: "image",
          source: { type: "base64", media_type: mediaType, data: att.data },
        });
      }
      turns.push({ role: "user", content });
      continue;
    }

    if (msg.role === "assistant") {
      const content: Array<Record<string, unknown>> = [];
      for (const part of msg.content) {
        if (part.type === "text" && part.text) {
          content.push({ type: "text", text: part.text });
        } else if (part.type === "thinking" && part.text) {
          // Prior thinking is replayed unsigned, which the signing endpoint
          // rejects — demote it to text (mirrors oh-my-pi's demotion path)
          // instead of dropping the reasoning context entirely.
          content.push({ type: "text", text: part.text });
        } else if (part.type === "toolCall") {
          content.push({
            type: "tool_use",
            id: part.call.id,
            name: part.call.name,
            input: parseToolInput(part.call.arguments),
          });
        }
      }
      if (content.length > 0) {
        turns.push({ role: "assistant", content });
      }
      continue;
    }

    if (msg.role === "toolResult") {
      const content: Array<Record<string, unknown>> = msg.results.map((r) => ({
        type: "tool_result",
        tool_use_id: r.toolCallId,
        content: toolResultText(r.content),
        ...(r.isError ? { is_error: true } : {}),
      }));
      if (content.length > 0) {
        // The Anthropic wire keys results by tool_use_id only.
        turns.push({ role: "user", content });
      }
    }
  }

  // Merge adjacent same-role turns (Anthropic requires strict alternation).
  const merged: ClaudeMessage[] = [];
  for (const turn of turns) {
    const last = merged[merged.length - 1];
    if (last && last.role === turn.role) {
      last.content.push(...turn.content);
    } else {
      merged.push({ role: turn.role, content: [...turn.content] });
    }
  }

  // Drop leading assistant turns; never send an empty conversation.
  while (merged.length > 0 && merged[0]!.role !== "user") {
    merged.shift();
  }
  if (merged.length === 0) {
    merged.push({ role: "user", content: [{ type: "text", text: "(continue)" }] });
  }

  // Trailing prompt-cache breakpoint over the conversation prefix, so the
  // stable history caches across turns. Skipped when the caller opts out.
  if (cacheRetention !== "none") {
    const lastMessage = merged[merged.length - 1]!;
    const lastBlock = lastMessage.content[lastMessage.content.length - 1];
    if (lastBlock) {
      lastBlock.cache_control = { type: "ephemeral" };
    }
  }

  return merged;
}

/**
 * Anthropic consumes draft-2020-12 JSON Schema. zod-to-json-schema's OpenAPI
 * output still uses draft-07's boolean exclusiveMinimum/exclusiveMaximum
 * keywords, which strict providers reject — same upgrade Codex applies.
 */
export function normalizeClaudeSchema(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeClaudeSchema);
  if (!value || typeof value !== "object") return value;

  const schema = value as Record<string, unknown>;
  const normalized: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(schema)) {
    normalized[key] = normalizeClaudeSchema(child);
  }

  if (normalized.exclusiveMinimum === true) {
    if (typeof normalized.minimum === "number") normalized.exclusiveMinimum = normalized.minimum;
    else delete normalized.exclusiveMinimum;
  } else if (normalized.exclusiveMinimum === false) {
    delete normalized.exclusiveMinimum;
  }
  if (normalized.exclusiveMaximum === true) {
    if (typeof normalized.maximum === "number") normalized.exclusiveMaximum = normalized.maximum;
    else delete normalized.exclusiveMaximum;
  }

  return normalized;
}

export function convertClaudeTools(tools: AgentTool[], cacheRetention?: CacheRetention): ClaudeTool[] {
  return tools.map((tool, index) => {
    const rawSchema = zodToJsonSchema(tool.inputSchema, {
      target: "openApi3",
      $refStrategy: "none",
    }) as Record<string, unknown>;
    const input_schema =
      rawSchema?.type === "object"
        ? (normalizeClaudeSchema(rawSchema) as Record<string, unknown>)
        : { type: "object" };
    return {
      name: tool.name,
      ...(tool.description ? { description: tool.description } : {}),
      input_schema,
      // Cache the (static) tool definitions with a breakpoint on the last
      // tool. Skipped when the caller opts out of caching.
      ...(cacheRetention !== "none" && index === tools.length - 1
        ? { cache_control: { type: "ephemeral" } }
        : {}),
    };
  });
}
