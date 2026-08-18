import { randomUUID } from "node:crypto";
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentMessage, AgentTool } from "@console/types";
import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { parseSse } from "../shared/sse-parser.js";
import { CODEX_BASE_URL, CODEX_CLIENT_VERSION, codexResponsesUrl } from "./constants.js";
import { loadCodexCredential, refreshCodexIfNeeded } from "./oauth.js";

function toolResultText(content: unknown): string {
  return typeof content === "string" ? content : JSON.stringify(content);
}

function convertInput(messages: AgentMessage[]): Array<Record<string, unknown>> {
  const input: Array<Record<string, unknown>> = [];
  for (const message of messages) {
    if (message.role === "user") {
      const content: Array<Record<string, unknown>> = [{ type: "input_text", text: message.content }];
      for (const attachment of message.attachments ?? []) {
        content.push({
          type: "input_image",
          image_url: `data:${attachment.mimeType};base64,${attachment.data}`,
          detail: "auto",
        });
      }
      input.push({ role: "user", content });
    } else if (message.role === "assistant") {
      for (const part of message.content) {
        if (part.type === "text" && part.text) {
          input.push({ role: "assistant", content: [{ type: "output_text", text: part.text }] });
        } else if (part.type === "toolCall") {
          input.push({
            type: "function_call",
            call_id: part.call.id,
            name: part.call.name,
            arguments: typeof part.call.arguments === "string" ? part.call.arguments : JSON.stringify(part.call.arguments ?? {}),
          });
        }
      }
    } else {
      for (const result of message.results) {
        input.push({
          type: "function_call_output",
          call_id: result.toolCallId,
          output: toolResultText(result.content),
        });
      }
    }
  }
  return input;
}

function convertTools(tools: AgentTool[]): Array<Record<string, unknown>> {
  return tools.map((tool) => ({
    type: "function",
    name: tool.name,
    description: tool.description,
    parameters: normalizeCodexSchema(
      zodToJsonSchema(tool.inputSchema, { target: "openApi3", $refStrategy: "none" }),
    ),
    strict: false,
  }));
}

/**
 * Codex consumes draft-2020-12 JSON Schema. zod-to-json-schema's OpenAPI
 * output still uses draft-07's boolean exclusiveMinimum/exclusiveMaximum
 * keywords, which Codex rejects with errors such as "True is not of type
 * number". This mirrors the draft upgrade performed by oh-my-pi while
 * preserving the rest of Console's tool schema.
 */
export function normalizeCodexSchema(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeCodexSchema);
  if (!value || typeof value !== "object") return value;

  const schema = value as Record<string, unknown>;
  const normalized: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(schema)) {
    normalized[key] = normalizeCodexSchema(child);
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
  } else if (normalized.exclusiveMaximum === false) {
    delete normalized.exclusiveMaximum;
  }

  return normalized;
}

export const codexStreamFn: StreamFn = async function* ({ model, systemPrompt, messages, tools, signal }) {
  const credential = await refreshCodexIfNeeded(await loadCodexCredential());
  const sessionId = randomUUID();
  const body: Record<string, unknown> = {
    model: model.id,
    input: convertInput(messages),
    stream: true,
    store: false,
    ...(systemPrompt.trim() ? { instructions: systemPrompt } : {}),
    ...(tools.length > 0 ? { tools: convertTools(tools) } : {}),
  };
  const response = await fetch(codexResponsesUrl((model as { baseUrl?: string }).baseUrl ?? CODEX_BASE_URL), {
    method: "POST",
    headers: {
      Authorization: `Bearer ${credential.accessToken}`,
      "chatgpt-account-id": credential.accountId,
      "OpenAI-Beta": "responses=experimental",
      originator: "pi",
      version: CODEX_CLIENT_VERSION,
      session_id: sessionId,
      conversation_id: sessionId,
      "x-client-request-id": sessionId,
      Accept: "text/event-stream",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Codex request failed (${response.status} ${response.statusText}): ${await response.text().catch(() => "")}`);
  }

  type FunctionCallState = {
    itemId: string;
    callId: string;
    name: string;
    arguments: string;
    finalized: boolean;
    emittedArguments: boolean;
  };

  const callsByItemId = new Map<string, FunctionCallState>();
  const callsByCallId = new Map<string, FunctionCallState>();
  const pendingDeltas = new Map<string, string>();
  const pendingFinalArguments = new Map<string, { name?: string; arguments: string }>();

  const emitArguments = (state: FunctionCallState, argumentsJson: string) => {
    if (state.emittedArguments || !argumentsJson) return null;
    state.emittedArguments = true;
    return {
      type: "toolCall" as const,
      id: state.callId,
      name: state.name,
      argumentsJson,
    };
  };

  for await (const event of parseSse<Record<string, unknown>>(response)) {
    const type = typeof event.type === "string" ? event.type : "";
    if (type === "error") {
      throw new Error(String((event.error as { message?: unknown } | undefined)?.message ?? "Codex stream error"));
    }
    if (type === "response.output_text.delta" || type === "response.refusal.delta") {
      if (typeof event.delta === "string") yield { type: "text", text: event.delta };
    } else if (type === "response.reasoning_summary_text.delta" || type === "response.reasoning_text.delta") {
      if (typeof event.delta === "string") yield { type: "thinking", text: event.delta };
    } else if (type === "response.output_item.added") {
      const item = event.item as
        | { type?: string; id?: string; call_id?: string; name?: string; arguments?: string }
        | undefined;
      if (item?.type === "function_call" && item.call_id && item.name) {
        const itemId = item.id ?? item.call_id;
        const state: FunctionCallState = {
          itemId,
          callId: item.call_id,
          name: item.name,
          arguments: item.arguments ?? "",
          finalized: false,
          emittedArguments: false,
        };
        const pendingDelta = pendingDeltas.get(itemId);
        if (pendingDelta) {
          state.arguments += pendingDelta;
          pendingDeltas.delete(itemId);
        }
        const pendingFinal = pendingFinalArguments.get(itemId);
        if (pendingFinal) {
          state.name = pendingFinal.name ?? state.name;
          state.arguments = pendingFinal.arguments;
          state.finalized = true;
          pendingFinalArguments.delete(itemId);
        }

        callsByItemId.set(itemId, state);
        callsByCallId.set(state.callId, state);
        // Emit the call immediately for the UI/agent loop, but defer arguments
        // until the finalized event so streamed fragments cannot be duplicated.
        yield { type: "toolCall", id: state.callId, name: state.name, argumentsJson: "" };
        if (state.finalized) {
          const finalized = emitArguments(state, state.arguments);
          if (finalized) yield finalized;
        }
      }
    } else if (type === "response.function_call_arguments.delta") {
      const itemId = typeof event.item_id === "string" ? event.item_id : "";
      const fragment = String(event.delta ?? "");
      const state = callsByItemId.get(itemId);
      if (state) {
        state.arguments += fragment;
      } else if (itemId) {
        pendingDeltas.set(itemId, `${pendingDeltas.get(itemId) ?? ""}${fragment}`);
      }
    } else if (type === "response.function_call_arguments.done") {
      const itemId = typeof event.item_id === "string" ? event.item_id : "";
      const argumentsJson = String(event.arguments ?? "");
      const state = callsByItemId.get(itemId);
      if (state) {
        state.name = typeof event.name === "string" ? event.name : state.name;
        state.arguments = argumentsJson;
        state.finalized = true;
        const finalized = emitArguments(state, state.arguments);
        if (finalized) yield finalized;
      } else if (itemId) {
        pendingFinalArguments.set(itemId, {
          name: typeof event.name === "string" ? event.name : undefined,
          arguments: argumentsJson,
        });
      }
    } else if (type === "response.failed" || type === "response.incomplete") {
      const responseError = event.response as { error?: { message?: string } } | undefined;
      if (responseError?.error?.message) throw new Error(responseError.error.message);
    }
  }

  // The finalized event is normally guaranteed, but flushing here keeps a
  // completed stream with only argument deltas from silently becoming `{}`.
  for (const state of callsByCallId.values()) {
    if (!state.finalized) {
      const assembled = emitArguments(state, state.arguments);
      if (assembled) yield assembled;
    }
  }
};
