/**
 * Core streaming logic shared by Gemini CLI and Antigravity providers.
 *
 * Responsible for:
 *  1. POSTing the CloudCodeAssistRequest to the CCA endpoint
 *  2. Parsing the SSE response stream
 *  3. Mapping CCA response parts to LLMDelta events
 *  4. Skipping thinking/reasoning parts (thought === true)
 *  5. Surfacing in-band stream errors
 */
import type { LLMDelta } from "../../../agent/src/service/agent-loop.js";
import type {
  CcaResponsePart,
  CloudCodeAssistChunk,
  CloudCodeAssistRequest,
} from "../types/index.js";
import { parseSse } from "./sse-parser.js";

export interface StreamCoreOptions {
  endpoint: string;
  accessToken: string;
  extraHeaders: Record<string, string>;
  body: CloudCodeAssistRequest;
  signal: AbortSignal | undefined;
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
 * Calls the CCA endpoint and yields LLMDelta for each streaming event.
 * Throws on HTTP errors or in-band stream errors.
 */
export async function* streamCore(options: StreamCoreOptions): AsyncGenerator<LLMDelta> {
  const { endpoint, accessToken, extraHeaders, body, signal } = options;

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

  for await (const chunk of parseSse<CloudCodeAssistChunk>(response)) {
    // In-band error delivered as final SSE event
    if (chunk.error !== undefined) {
      const err = chunk.error;
      throw new Error(
        `CCA stream error (${err.code ?? "?"} ${err.status ?? ""}): ${err.message ?? "unknown"}`,
      );
    }

    const candidates = chunk.response?.candidates;
    if (candidates === undefined || candidates.length === 0) continue;

    const candidate = candidates[0];
    if (candidate === undefined || candidate.content === undefined) continue;

    for (const part of candidate.content.parts) {
      if (isThinkingPart(part)) continue;

      if (hasText(part)) {
        const delta: LLMDelta = { type: "text", text: part.text };
        yield delta;
      } else if (hasFunctionCall(part) && part.functionCall !== undefined) {
        const fc = part.functionCall;
        const delta: LLMDelta = {
          type: "toolCall",
          id: fc.id ?? `call-${syntheticCallIndex++}`,
          name: fc.name,
          argumentsJson: JSON.stringify(fc.args),
        };
        yield delta;
      }
    }
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
