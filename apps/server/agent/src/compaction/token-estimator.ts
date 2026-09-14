import type { AgentMessage, AgentTool } from "@/agent/src/types/index.js";

/**
 * Estimate token count for a list of messages.
 * Uses ~4 chars per token for text and ~1,000 tokens per inline image attachment.
 */
export function estimateMessageTokens(messages: AgentMessage[]): number {
  let totalChars = 0;
  let imageTokens = 0;

  for (const msg of messages) {
    if (msg.role === "user") {
      totalChars += msg.content ? msg.content.length : 0;
      if (msg.attachments && msg.attachments.length > 0) {
        imageTokens += msg.attachments.length * 1000;
      }
    } else if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "text" || part.type === "thinking") {
          totalChars += part.text ? part.text.length : 0;
        } else if (part.type === "toolCall") {
          totalChars += part.call.name.length;
          if (part.call.arguments) {
            try {
              totalChars += JSON.stringify(part.call.arguments).length;
            } catch {
              totalChars += 50;
            }
          }
        }
      }
    } else if (msg.role === "toolResult") {
      for (const res of msg.results) {
        if (typeof res.content === "string") {
          totalChars += res.content.length;
        } else if (res.content != null) {
          try {
            totalChars += JSON.stringify(res.content).length;
          } catch {
            totalChars += 100;
          }
        }
      }
    }
  }

  return Math.ceil(totalChars / 4) + imageTokens;
}

/** Where a payload token estimate came from. */
export type TokenCountSource = "local" | "provider";

export interface PayloadTokenEstimate {
  tokens: number;
  source: TokenCountSource;
}

export interface EstimatePayloadOptions {
  messages: AgentMessage[];
  /** System prompt / instructions sent with every request. */
  systemPrompt?: string;
  /** Tools whose name, description, and schema ride on every request. */
  tools?: Pick<AgentTool, "name" | "description">[];
  /** Flat safety margin for provider wire overhead. Default: 2,000. */
  wireMarginTokens?: number;
}

/**
 * Conservative wire-payload estimate: messages + system prompt + tool
 * definitions. Prose counts at ~4 chars/token; code, JSON, and tool output
 * tokenize denser (~3 chars/token). Deliberately no tokenizer dependency —
 * provider-native counting endpoints can plug in as `source: "provider"`
 * later; this heuristic stays as the fail-open fallback.
 */
const CHARS_PER_PROSE_TOKEN = 4;
const CHARS_PER_CODE_TOKEN = 3;
const IMAGE_TOKENS_EACH = 1000;
/** Flat per-tool allowance for the JSON input schema (name/description counted exactly). */
const TOOL_SCHEMA_OVERHEAD_TOKENS = 400;
const DEFAULT_WIRE_MARGIN_TOKENS = 2000;

function toolArgsChars(args: unknown): number {
  if (args == null) return 0;
  try {
    return JSON.stringify(args)?.length ?? 50;
  } catch {
    return 50;
  }
}

function toolResultChars(content: unknown): number {
  if (typeof content === "string") return content.length;
  if (content == null) return 0;
  try {
    return JSON.stringify(content)?.length ?? 100;
  } catch {
    return 100;
  }
}

export function estimatePayloadTokens(options: EstimatePayloadOptions): PayloadTokenEstimate {
  const { messages, systemPrompt, tools } = options;
  const wireMarginTokens = options.wireMarginTokens ?? DEFAULT_WIRE_MARGIN_TOKENS;

  let proseChars = systemPrompt?.length ?? 0;
  let codeChars = 0;
  let imageTokens = 0;

  for (const msg of messages) {
    if (msg.role === "user") {
      proseChars += msg.content ? msg.content.length : 0;
      if (msg.attachments && msg.attachments.length > 0) {
        imageTokens += msg.attachments.length * IMAGE_TOKENS_EACH;
      }
    } else if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "text" || part.type === "thinking") {
          proseChars += part.text ? part.text.length : 0;
        } else if (part.type === "toolCall") {
          proseChars += part.call.name.length;
          codeChars += toolArgsChars(part.call.arguments);
        }
      }
    } else if (msg.role === "toolResult") {
      for (const res of msg.results) {
        codeChars += toolResultChars(res.content);
      }
    }
  }

  let toolTokens = 0;
  for (const tool of tools ?? []) {
    toolTokens += Math.ceil((tool.name.length + (tool.description?.length ?? 0)) / CHARS_PER_PROSE_TOKEN);
    toolTokens += TOOL_SCHEMA_OVERHEAD_TOKENS;
  }

  const tokens =
    Math.ceil(proseChars / CHARS_PER_PROSE_TOKEN) +
    Math.ceil(codeChars / CHARS_PER_CODE_TOKEN) +
    imageTokens +
    toolTokens +
    wireMarginTokens;

  return { tokens, source: "local" };
}
