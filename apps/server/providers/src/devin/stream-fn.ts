/**
 * Devin (Codeium Cascade) Connect-streaming provider.
 *
 * Emits Console's `LLMDelta` events directly, so the agent loop doesn't
 * need to know about Devin's start/text_delta/text_end lifecycle.
 *
 * Streaming protocol:
 * - POST `/exa.api_server_pb.ApiServerService/GetChatMessage`
 * - Request body: 1 flag byte (0x01=gzip) + 4 BE length + gzipped protobuf
 * - Response: stream of framed Connect messages with the same shape;
 *   flag 0x01 = gzip payload, flag 0x02 = end-of-stream JSON trailer
 * - Errors usually arrive as trailers on HTTP 200, not as HTTP status
 */
import { gzipSync, gunzipSync } from "node:zlib";
import * as crypto from "node:crypto";
import {
  ChatMessagePromptSchema,
  ChatMessageRequestType,
  ChatToolChoiceSchema,
  CompletionConfigurationSchema,
  ConversationalPlannerMode,
  GetChatMessageRequestSchema,
  GetChatMessageResponseSchema,
  MetadataSchema,
  PromptCacheOptionsSchema,
  CacheControlType,
  ChatToolDefinitionSchema,
} from "./proto/devin-proto.js";
import { create, fromBinary, toBinary } from "./proto/protobuf.js";
import { buildChatMessagePrompts, buildDevinTools } from "./transform-messages.js";
import { fetchDevinAuthMetadata } from "./auth.js";
import { devinCliMetadata, DEVIN_STREAMING_BASE_URL } from "./metadata.js";
import type { LLMDelta, StreamFn } from "@/agent/src/service/types.js";

const CHAT_MESSAGE_PATH = "/exa.api_server_pb.ApiServerService/GetChatMessage";

const CONNECT_COMPRESSED_FLAG = 0x01;
const CONNECT_END_STREAM_FLAG = 0x02;
/** Hard cap on a single Connect frame payload (4-byte BE length is otherwise
 *  attacker-controlled up to 2^32 - 1). */
const MAX_CONNECT_FRAME_PAYLOAD = 16 * 1024 * 1024;

const DEVIN_DEFAULT_STOP_PATTERNS = [
  "<|user|>",
  "<|bot|>",
  "<|context_request|>",
  "<|endoftext|>",
  "<|end_of_turn|>",
];

const DEFAULT_MAX_TOKENS = 64_000;
const DEFAULT_TEMPERATURE = 0.4;

/** Connect trailer shape: `{ "error": { "code": "...", "message": "...", ... } }`. */
interface TrailerError {
  code: string;
  message: string;
  detail?: string;
  raw: string;
  formatted: string;
}

function readConnectTrailerError(json: string): TrailerError | null {
  if (!json) return null;
  let parsed: { error?: { code?: unknown; message?: unknown; detail?: unknown } };
  try {
    parsed = JSON.parse(json) as typeof parsed;
  } catch {
    return null;
  }
  const e = parsed.error;
  if (!e || typeof e.code !== "string" || typeof e.message !== "string") return null;
  const detail = typeof e.detail === "string" ? e.detail : undefined;
  return {
    code: e.code,
    message: e.message,
    detail,
    raw: json,
    formatted: detail ? `${e.code}: ${e.message} (${detail})` : `${e.code}: ${e.message}`,
  };
}

export const devinStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const apiKey = process.env.DEVIN_OAUTH_TOKEN;
  if (!apiKey) throw new Error("Devin stream: DEVIN_OAUTH_TOKEN is not set.");

  const auth = await fetchDevinAuthMetadata(apiKey, fetch, signal);
  const chatBaseUrl = auth.baseUrl ?? DEVIN_STREAMING_BASE_URL;
  const cascadeId = crypto.randomUUID();

  const chatMessagePrompts = buildChatMessagePrompts(messages, cascadeId);
  const devinTools = buildDevinTools(tools);

  const request = create(GetChatMessageRequestSchema, {
    metadata: create(MetadataSchema, devinCliMetadata(apiKey, auth.userJwt)),
    prompt: systemPrompt,
    chatMessagePrompts,
    chatModelUid: model.id,
    requestType: ChatMessageRequestType.CASCADE,
    plannerMode: ConversationalPlannerMode.DEFAULT,
    toolChoice: create(ChatToolChoiceSchema, { choice: { case: "optionName", value: "auto" } }),
    systemPromptCacheOptions: create(PromptCacheOptionsSchema, { type: CacheControlType.EPHEMERAL }),
    cascadeId,
    executionId: crypto.randomUUID(),
    configuration: create(CompletionConfigurationSchema, {
      numCompletions: 1n,
      maxTokens: BigInt(DEFAULT_MAX_TOKENS),
      maxNewlines: 200n,
      temperature: DEFAULT_TEMPERATURE,
      firstTemperature: DEFAULT_TEMPERATURE,
      topK: 50n,
      topP: 1,
      stopPatterns: DEVIN_DEFAULT_STOP_PATTERNS,
      fimEotProbThreshold: 1,
    }),
    tools: devinTools.map((tool) =>
      create(ChatToolDefinitionSchema, {
        name: tool.name,
        description: tool.description,
        jsonSchemaString: tool.jsonSchemaString,
        strict: tool.strict,
      }),
    ),
  });

  const reqBytes = toBinary(GetChatMessageRequestSchema, request);
  const gz = gzipSync(reqBytes);
  const frame = new Uint8Array(5 + gz.length);
  frame[0] = CONNECT_COMPRESSED_FLAG;
  new DataView(frame.buffer).setUint32(1, gz.length, false);
  frame.set(gz, 5);

  const response = await fetch(chatBaseUrl + CHAT_MESSAGE_PATH, {
    method: "POST",
    headers: {
      "content-type": "application/connect+proto",
      "connect-protocol-version": "1",
      "connect-content-encoding": "gzip",
      "accept-encoding": "identity",
      "user-agent": "connect-go/1.18.1 (go1.26.3)",
      "connect-accept-encoding": "gzip",
    },
    body: frame as unknown as BodyInit,
    signal,
  });

  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`Devin API error ${response.status} ${response.statusText}: ${text}`);
  }
  if (!response.body) throw new Error("Devin API error: response body is empty.");

  const reader = response.body.getReader();
  let pending: Buffer = Buffer.alloc(0);

  // Track per-tool-call arg accumulation. Tool calls stream with deltas that
  // are sometimes absolute (whole prefix-so-far) and sometimes incremental.
  const toolPartialJson = new Map<string, string>();
  let activeToolCallId: string | undefined;

  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (value && value.length > 0) {
        pending = pending.length === 0
          ? Buffer.from(value.buffer, value.byteOffset, value.byteLength)
          : Buffer.concat([pending, value]);
      }

      while (pending.length >= 5) {
        const flag = pending[0]!;
        const len = pending.readUInt32BE(1);
        if (len > MAX_CONNECT_FRAME_PAYLOAD) {
          throw new Error(`Devin Connect frame length ${len} exceeds ${MAX_CONNECT_FRAME_PAYLOAD}-byte cap`);
        }
        if (pending.length < 5 + len) break;
        const payload = pending.subarray(5, 5 + len);
        pending = pending.subarray(5 + len);

        if (flag & CONNECT_END_STREAM_FLAG) {
          const trailerBytes = flag & CONNECT_COMPRESSED_FLAG ? gunzipSync(payload) : payload;
          const trailerError = readConnectTrailerError(trailerBytes.toString("utf8").trim());
          if (trailerError) {
            throw new Error(`Devin stream rejected: ${trailerError.formatted}`);
          }
          continue;
        }

        const raw = flag & CONNECT_COMPRESSED_FLAG ? gunzipSync(payload) : payload;
        const msg = fromBinary(GetChatMessageResponseSchema, raw);

        if (msg.deltaThinking) {
          yield { type: "thinking", text: msg.deltaThinking } satisfies LLMDelta;
        }
        if (msg.deltaText) {
          yield { type: "text", text: msg.deltaText } satisfies LLMDelta;
        }
        if (msg.deltaToolCalls.length > 0) {
          for (const tc of msg.deltaToolCalls) {
            const toolCallId = tc.id || activeToolCallId;
            if (!toolCallId) continue;
            // First delta for a tool call may carry the name.
            if (tc.name && !toolPartialJson.has(toolCallId)) {
              yield {
                type: "toolCall",
                id: toolCallId,
                name: tc.name,
                argumentsJson: "",
              } satisfies LLMDelta;
            }
            activeToolCallId = toolCallId;
            if (!tc.argumentsJson) continue;
            const previousJson = toolPartialJson.get(toolCallId) ?? "";
            // Some edges send the whole prefix-so-far; others send an incremental
            // fragment. The server's contract is "prefix", so prefer the longer.
            const accumulated = tc.argumentsJson.startsWith(previousJson)
              ? tc.argumentsJson
              : previousJson + tc.argumentsJson;
            const delta = accumulated.slice(previousJson.length);
            toolPartialJson.set(toolCallId, accumulated);
            if (delta.length > 0) {
              yield {
                type: "toolCall",
                id: toolCallId,
                name: tc.name ?? "",
                argumentsJson: delta,
              } satisfies LLMDelta;
            }
          }
        }
      }

      if (done) break;
    }
  } finally {
    try {
      reader.releaseLock();
    } catch {
      // Already released.
    }
  }
};
