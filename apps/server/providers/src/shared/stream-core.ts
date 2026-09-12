/**
 * Core streaming logic shared by Gemini CLI and Antigravity providers.
 *
 * Responsible for:
 *  1. POSTing the CloudCodeAssistRequest to the CCA endpoint
 *  2. Parsing the SSE response stream
 *  3. Mapping CCA response parts to LLMDelta events
 *  4. Skipping thinking/reasoning parts (thought === true)
 *  5. Capturing final cumulative usage and yielding it as a single `usage`
 *     delta after the stream completes
 *  6. Surfacing in-band stream errors
 */
import type { LLMDelta } from "@/agent/src/service/agent-loop.js";
import type { CacheRetention, TurnUsage } from "@console/types";
import type {
  CcaResponsePart,
  CcaUsageMetadata,
  CloudCodeAssistChunk,
  CloudCodeAssistRequest,
} from "@/providers/src/types/index.js";
import { parseSse } from "./sse-parser.js";

export interface StreamCoreOptions {
  endpoint: string;
  accessToken: string;
  extraHeaders: Record<string, string>;
  body: CloudCodeAssistRequest;
  signal: AbortSignal | undefined;
  /**
   * Provider-neutral cache retention hint. Currently used only to influence
   * `cacheStatus` reporting when the endpoint returns no cache metadata —
   * never to fabricate a cache hit. Providers that cannot honor cache
   * controls should pass "none".
   */
  cacheRetention?: CacheRetention;
}

/** Returns true when a part should be skipped (reasoning/thinking content) */
function isThinkingPart(part: CcaResponsePart): boolean {
  return part.thought === true;
}

/** Returns true when a part has visible text output */
function hasText(part: CcaResponsePart): part is CcaResponsePart & { text: string } {
  return typeof part.text === "string" && part.text.length > 0;
}

/** Returns true when a part contains a function call from the model */
function hasFunctionCall(part: CcaResponsePart): boolean {
  return part.functionCall !== undefined;
}

/**
 * Normalize a CCA `usageMetadata` block into the agent-level `TurnUsage`.
 *
 * Missing metadata produces `cacheStatus: "unknown"`, never a forced miss.
 * We only report a "miss" when the endpoint explicitly tells us
 * `cachedContentTokenCount === 0` alongside a `promptTokenCount` — and even
 * then "miss" is reported as a separate signal (the prompt was billed, no
 * cache served it) rather than being inferred from the absence of fields.
 */
function normalizeUsage(
  metadata: CcaUsageMetadata | undefined,
  retention: CacheRetention | undefined,
): TurnUsage | undefined {
  if (!metadata) return undefined;

  const prompt = metadata.promptTokenCount;
  const cached = metadata.cachedContentTokenCount;
  const output = metadata.candidatesTokenCount ?? 0;
  const thoughts = metadata.thoughtsTokenCount;
  const total = metadata.totalTokenCount ?? (prompt ?? 0) + output + (thoughts ?? 0);

  // If the endpoint reports neither prompt nor total, we have nothing
  // meaningful to surface — treat as "unknown" rather than zeros.
  if (prompt === undefined && metadata.totalTokenCount === undefined) {
    if (retention === "none") {
      return {
        input: 0,
        cacheRead: 0,
        cacheWrite: 0,
        output,
        ...(thoughts !== undefined ? { reasoningTokens: thoughts } : {}),
        totalTokens: output + (thoughts ?? 0),
        cacheStatus: "unsupported",
      };
    }
    return undefined;
  }

  // Compute uncached input only when both prompt and cached are reported.
  // Otherwise we cannot separate cached from uncached tokens, so leave the
  // cache fields at zero and mark cacheStatus as unknown.
  const hasCacheBreakdown = prompt !== undefined && cached !== undefined;
  const cacheRead = hasCacheBreakdown ? cached! : 0;
  const input = hasCacheBreakdown ? prompt! - cached! : prompt ?? 0;

  let cacheStatus: TurnUsage["cacheStatus"];
  if (!hasCacheBreakdown) {
    cacheStatus = retention === "none" ? "unsupported" : "unknown";
  } else if (cached! > 0) {
    cacheStatus = "hit";
  } else if (cached === 0) {
    cacheStatus = "miss";
  } else {
    cacheStatus = "unknown";
  }

  return {
    input,
    cacheRead,
    cacheWrite: 0,
    output,
    ...(thoughts !== undefined ? { reasoningTokens: thoughts } : {}),
    totalTokens: total,
    cacheStatus,
  };
}

/**
 * Calls the CCA endpoint and yields LLMDelta for each streaming event.
 * Throws on HTTP errors or in-band stream errors.
 */
export async function* streamCore(options: StreamCoreOptions): AsyncGenerator<LLMDelta> {
  const { endpoint, accessToken, extraHeaders, body, signal, cacheRetention } = options;

  // Stable ID counter for function calls that arrive without an explicit id.
  // Some CCA responses omit fc.id, and generating a fresh randomUUID per delta
  // would split a single logical tool call into duplicates. We assign a
  // sequential synthetic id so all deltas for the same call accumulate.
  let syntheticCallIndex = 0;

  const response = await fetch(endpoint, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...extraHeaders,
    },
    body: JSON.stringify(body),
    signal,
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`CCA request failed (${response.status} ${response.statusText}): ${detail}`);
  }

  // Track the latest usageMetadata across all chunks. CCA sends cumulative
  // counts in usageMetadata, so the last one wins. If the endpoint never
  // emits usageMetadata we still emit a delta with cacheStatus: "unknown"
  // so callers can see we tried to observe.
  let lastUsage: CcaUsageMetadata | undefined;

  try {
    for await (const chunk of parseSse<CloudCodeAssistChunk>(response)) {
      // In-band error delivered as final SSE event
      if (chunk.error !== undefined) {
        const err = chunk.error;
        throw new Error(
          `CCA stream error (${err.code ?? "?"} ${err.status ?? ""}): ${err.message ?? "unknown"}`,
        );
      }

      const usage = chunk.response?.usageMetadata;
      if (usage !== undefined) lastUsage = usage;

      const candidates = chunk.response?.candidates;
      if (candidates === undefined || candidates.length === 0) continue;

      const candidate = candidates[0];
      if (candidate === undefined || candidate.content === undefined) continue;

      for (const part of candidate.content.parts) {
        if (isThinkingPart(part)) continue;

        if (hasFunctionCall(part) && part.functionCall !== undefined) {
          const fc = part.functionCall;
          const delta: LLMDelta = {
            type: "toolCall",
            id: fc.id ?? `call-${syntheticCallIndex++}`,
            name: fc.name,
            argumentsJson: JSON.stringify(fc.args),
            ...(part.thoughtSignature ? { thoughtSignature: part.thoughtSignature } : {}),
          };
          yield delta;
        } else if (hasText(part) || part.thoughtSignature) {
          const delta: LLMDelta = {
            type: "text",
            text: part.text ?? "",
            ...(part.thoughtSignature ? { thoughtSignature: part.thoughtSignature } : {}),
          };
          yield delta;
        }
      }
    }
  } finally {
    // Emit the final usage delta after the stream completes (whether or not
    // we observed a usage block). Providers that report cumulative usage per
    // chunk will overwrite earlier values; the final cumulative record is
    // authoritative.
    const usage = normalizeUsage(lastUsage, cacheRetention) ?? {
      input: 0,
      cacheRead: 0,
      cacheWrite: 0,
      output: 0,
      totalTokens: 0,
      cacheStatus: cacheRetention === "none" ? "unsupported" as const : "unknown" as const,
    };
    yield { type: "usage", usage };
  }
}

/**
 * Builds the full CCA streaming endpoint URL.
 *
 * Mirrors the Gemini CLI / Antigravity clients: the projectId is NOT in the
 * URL — it rides in the request body (`body.project`). The endpoint is the
 * global `streamGenerateContent` SSE route, qualified only by `alt=sse` so the
 * response is a server-sent events stream rather than a JSON blob.
 *
 * Pattern: {baseUrl}/v1internal:streamGenerateContent?alt=sse
 */
export function buildEndpointUrl(baseUrl: string): string {
  const base = baseUrl.replace(/\/+$/, "");
  return `${base}/v1internal:streamGenerateContent?alt=sse`;
}
