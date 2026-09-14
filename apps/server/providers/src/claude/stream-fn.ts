/**
 * Claude StreamFn — Anthropic Messages API (`POST /v1/messages`, SSE)
 * authenticated with a Claude Pro/Max subscription OAuth token.
 *
 * Wire fingerprint (Authorization bearer, `oauth-2025-04-20` beta,
 * `claude-cli` User-Agent) mirrors oh-my-pi's
 * `providers/anthropic.ts` OAuth branch.
 */
import type { CacheRetention, CacheStatus, TurnUsage } from "@console/types";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import {
  CLAUDE_BASE_URL,
  CLAUDE_MAX_OUTPUT_TOKENS,
  CLAUDE_OAUTH_BETAS,
  CLAUDE_SDK_VERSION,
  CLAUDE_USER_AGENT,
  claudeMessagesUrl,
} from "./constants.js";
import { convertClaudeMessages, convertClaudeTools } from "./convert.js";
import { loadClaudeCredential, refreshClaudeIfNeeded } from "./oauth.js";

function mapStainlessOs(platform: string): string {
  switch (platform.toLowerCase()) {
    case "darwin":
      return "MacOS";
    case "windows":
    case "win32":
      return "Windows";
    case "linux":
      return "Linux";
    case "freebsd":
      return "FreeBSD";
    default:
      return `Other:${platform.toLowerCase()}`;
  }
}

function mapStainlessArch(arch: string): string {
  switch (arch.toLowerCase()) {
    case "amd64":
    case "x64":
      return "x64";
    case "arm64":
    case "aarch64":
      return "arm64";
    case "386":
    case "x86":
    case "ia32":
      return "x86";
    default:
      return `other:${arch.toLowerCase()}`;
  }
}

function buildHeaders(accessToken: string): Record<string, string> {
  return {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    Accept: "text/event-stream",
    "anthropic-version": "2023-06-01",
    "anthropic-beta": CLAUDE_OAUTH_BETAS.join(","),
    "anthropic-dangerous-direct-browser-access": "true",
    "User-Agent": CLAUDE_USER_AGENT,
    "X-Stainless-Arch": mapStainlessArch(process.arch),
    "X-Stainless-Lang": "js",
    "X-Stainless-OS": mapStainlessOs(process.platform),
    "X-Stainless-Package-Version": CLAUDE_SDK_VERSION,
    "X-Stainless-Retry-Count": "0",
    "X-Stainless-Runtime": "node",
    "X-Stainless-Runtime-Version": "v26.3.0",
    "X-Stainless-Timeout": "600",
    "x-app": "cli",
  };
}

/**
 * Parses Anthropic's SSE stream into `{ event, data }` pairs. Unlike the
 * shared CCA parser, the event name (`message_start`, `content_block_delta`,
 * ...) is significant so `event:` lines are tracked.
 */
export async function* parseAnthropicSse(response: Response): AsyncGenerator<{ event: string; data: unknown }> {
  if (!response.body) {
    throw new Error("No response body from Claude endpoint");
  }
  const decoder = new TextDecoder();
  let buffer = "";
  let currentEvent = "";

  const handleLine = function* (line: string): Generator<{ event: string; data: unknown }> {
    const trimmed = line.trim();
    if (trimmed.startsWith("event:")) {
      currentEvent = trimmed.slice(6).trim();
    } else if (trimmed.startsWith("data:")) {
      const jsonText = trimmed.slice(5).trim();
      currentEvent = currentEvent || "message";
      if (jsonText === "[DONE]" || jsonText === "") {
        currentEvent = "";
        return;
      }
      try {
        yield { event: currentEvent, data: JSON.parse(jsonText) as unknown };
      } catch {
        // malformed SSE chunk — skip
      }
      currentEvent = "";
    }
  };

  for await (const chunk of response.body as unknown as AsyncIterable<Uint8Array>) {
    buffer += decoder.decode(chunk, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      yield* handleLine(line);
    }
  }
  if (buffer.trim() !== "") {
    yield* handleLine(buffer);
  }
}

interface ClaudeUsageWire {
  input_tokens?: number;
  output_tokens?: number;
  cache_creation_input_tokens?: number;
  cache_read_input_tokens?: number;
}

/**
 * Normalize Anthropic usage blocks into the agent-level TurnUsage.
 * Follows the same contract as the Codex provider: missing usage →
 * cacheStatus "unknown", never a forced miss; `cacheRetention: "none"`
 * with no cache fields → "unsupported".
 */
export function normalizeClaudeUsage(
  input: ClaudeUsageWire | undefined,
  output: ClaudeUsageWire | undefined,
  retention: CacheRetention | undefined,
): TurnUsage | undefined {
  if (!input && !output) return undefined;
  const prompt = input?.input_tokens;
  const cached = input?.cache_read_input_tokens;
  const created = input?.cache_creation_input_tokens ?? 0;
  const out = output?.output_tokens ?? 0;
  if (prompt === undefined) return undefined;

  const hasCacheBreakdown = cached !== undefined;
  const cacheRead = hasCacheBreakdown ? cached! : 0;

  let cacheStatus: CacheStatus;
  if (hasCacheBreakdown) {
    cacheStatus = cached! > 0 ? "hit" : cached === 0 ? "miss" : "unknown";
  } else if (retention === "none") {
    cacheStatus = "unsupported";
  } else {
    cacheStatus = "unknown";
  }

  return {
    input: hasCacheBreakdown ? Math.max(0, prompt - cached!) : prompt,
    cacheRead,
    cacheWrite: created,
    output: out,
    totalTokens: prompt + created + out,
    cacheStatus,
  };
}

function buildRequestBody(
  model: { id: string },
  systemPrompt: string,
  messages: ReturnType<typeof convertClaudeMessages>,
  tools: ReturnType<typeof convertClaudeTools>,
  retention: CacheRetention | undefined,
): Record<string, unknown> {
  const trimmedSystem = systemPrompt.trim();
  return {
    model: model.id,
    max_tokens: CLAUDE_MAX_OUTPUT_TOKENS,
    ...(trimmedSystem
      ? {
          system:
            retention === "none"
              ? trimmedSystem
              : [{ type: "text", text: trimmedSystem, cache_control: { type: "ephemeral" } }],
        }
      : {}),
    messages,
    ...(tools.length > 0 ? { tools } : {}),
    stream: true,
  };
}

type ToolCallState = {
  index: number;
  id: string;
  name: string;
  arguments: string;
  finalized: boolean;
  emittedStart: boolean;
};

export const claudeStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
  cacheRetention,
}) {
  const credential = await refreshClaudeIfNeeded(await loadClaudeCredential());
  const convertedMessages = convertClaudeMessages(messages);
  const convertedTools = convertClaudeTools(tools);
  const body = buildRequestBody(
    model,
    systemPrompt,
    convertedMessages,
    convertedTools,
    cacheRetention,
  );

  const baseUrl = (model as { baseUrl?: string }).baseUrl ?? CLAUDE_BASE_URL;
  const response = await fetch(claudeMessagesUrl(baseUrl), {
    method: "POST",
    headers: buildHeaders(credential.accessToken),
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Claude request failed (${response.status} ${response.statusText}): ${await response.text().catch(() => "")}`);
  }

  const callsByIndex = new Map<number, ToolCallState>();
  let inputUsage: ClaudeUsageWire | undefined;
  let outputUsage: ClaudeUsageWire | undefined;

  const emitStart = (state: ToolCallState) => {
    if (state.emittedStart) return null;
    state.emittedStart = true;
    return { type: "toolCall" as const, id: state.id, name: state.name, argumentsJson: "" };
  };

  const emitFinal = (state: ToolCallState) => {
    if (state.finalized) return null;
    state.finalized = true;
    return {
      type: "toolCall" as const,
      id: state.id,
      name: state.name,
      argumentsJson: state.arguments || "{}",
    };
  };

  try {
    for await (const { event, data } of parseAnthropicSse(response)) {
      const payload = (data ?? {}) as Record<string, unknown>;
      if (event === "error") {
        const err = payload.error as { message?: unknown } | undefined;
        throw new Error(String(err?.message ?? "Claude stream error"));
      } else if (event === "message_start") {
        const message = payload.message as { usage?: ClaudeUsageWire } | undefined;
        if (message?.usage) inputUsage = message.usage;
      } else if (event === "content_block_start") {
        const index = typeof payload.index === "number" ? payload.index : -1;
        const block = payload.content_block as { type?: string; id?: string; name?: string } | undefined;
        if (index >= 0 && block?.type === "tool_use" && block.id && block.name) {
          const state: ToolCallState = {
            index,
            id: block.id,
            name: block.name,
            arguments: "",
            finalized: false,
            emittedStart: false,
          };
          callsByIndex.set(index, state);
          const start = emitStart(state);
          if (start) yield start;
        }
      } else if (event === "content_block_delta") {
        const index = typeof payload.index === "number" ? payload.index : -1;
        const delta = payload.delta as { type?: string; text?: string; thinking?: string; partial_json?: string } | undefined;
        if (delta?.type === "text_delta" && typeof delta.text === "string") {
          yield { type: "text", text: delta.text };
        } else if (delta?.type === "thinking_delta" && typeof delta.thinking === "string") {
          yield { type: "thinking", text: delta.thinking };
        } else if (
          delta?.type === "input_json_delta" &&
          typeof delta.partial_json === "string" &&
          index >= 0
        ) {
          const state = callsByIndex.get(index);
          if (state) state.arguments += delta.partial_json;
        }
      } else if (event === "content_block_stop") {
        const index = typeof payload.index === "number" ? payload.index : -1;
        const state = index >= 0 ? callsByIndex.get(index) : undefined;
        if (state) {
          const start = emitStart(state);
          if (start) yield start;
          const final = emitFinal(state);
          if (final) yield final;
        }
      } else if (event === "message_delta") {
        const usage = payload.usage as ClaudeUsageWire | undefined;
        if (usage) outputUsage = usage;
      }
    }
  } finally {
    // The stop event normally finalizes each tool call, but flushing here
    // keeps a truncated stream from silently dropping accumulated arguments.
    for (const state of callsByIndex.values()) {
      if (!state.finalized) {
        const start = emitStart(state);
        if (start) yield start;
        const final = emitFinal(state);
        if (final) yield final;
      }
    }

    const usage = normalizeClaudeUsage(inputUsage, outputUsage, cacheRetention) ?? {
      input: 0,
      cacheRead: 0,
      cacheWrite: 0,
      output: 0,
      totalTokens: 0,
      cacheStatus: cacheRetention === "none" ? ("unsupported" as const) : ("unknown" as const),
    };
    yield { type: "usage", usage };
  }
};
