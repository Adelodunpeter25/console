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
    parameters: zodToJsonSchema(tool.inputSchema, { target: "openApi3", $refStrategy: "none" }),
    strict: false,
  }));
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

  const toolNames = new Map<string, string>();
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
      const item = event.item as { type?: string; id?: string; call_id?: string; name?: string; arguments?: string } | undefined;
      if (item?.type === "function_call" && item.call_id && item.name) {
        toolNames.set(item.call_id, item.name);
        yield { type: "toolCall", id: item.call_id, name: item.name, argumentsJson: item.arguments ?? "" };
      }
    } else if (type === "response.function_call_arguments.delta") {
      const id = typeof event.call_id === "string" ? event.call_id : "";
      if (id) yield { type: "toolCall", id, name: toolNames.get(id) ?? "", argumentsJson: String(event.delta ?? "") };
    } else if (type === "response.failed" || type === "response.incomplete") {
      const responseError = event.response as { error?: { message?: string } } | undefined;
      if (responseError?.error?.message) throw new Error(responseError.error.message);
    }
  }
};
