/**
 * OpenCode Zen StreamFn — OpenAI-compatible /chat/completions with SSE streaming.
 *
 * No API key required (free tier). Emits text, thinking, and toolCall deltas.
 */
import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { parseSse } from "../shared/sse-parser.js";
import { OPENCODE_BASE_URL } from "./constants.js";
import {
  convertOpencodeMessages,
  type OpenAIInputMessage,
} from "./convert-messages.js";
import { convertOpencodeTools } from "./convert-tools.js";

interface OpenCodeChatChunk {
  choices?: Array<{
    delta?: {
      content?: string | null;
      reasoning_content?: string | null;
      tool_calls?: Array<{
        index?: number;
        id?: string;
        function?: { name?: string; arguments?: string };
      }>;
    };
    finish_reason?: string | null;
  }>;
}

const CHAT_COMPLETIONS_URL = `${OPENCODE_BASE_URL}/chat/completions`;

export const opencodeStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const convertedMessages: OpenAIInputMessage[] = convertOpencodeMessages(
    messages,
    systemPrompt,
  );
  const convertedTools = tools.length > 0 ? convertOpencodeTools(tools) : undefined;

  const body: Record<string, unknown> = {
    model: model.id,
    stream: true,
    messages: convertedMessages,
  };
  if (convertedTools && convertedTools.length > 0) {
    body.tools = convertedTools;
  }

  const response = await fetch(CHAT_COMPLETIONS_URL, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify(body),
    signal,
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(
      `OpenCode Zen request failed (${response.status} ${response.statusText}): ${detail}`,
    );
  }

  // Per-stream tool call state. OpenAI streams tool calls in pieces (possibly
  // interleaved): each has a stable index, and arguments arrive across chunks.
  // The agent loop concatenates argumentsJson by toolCall id, matching this.
  const toolCallState = new Map<
    number,
    { id: string; name: string; argumentsJson: string }
  >();
  let syntheticCallIndex = 0;

  for await (const chunk of parseSse<OpenCodeChatChunk>(response)) {
    const choice = chunk.choices?.[0];
    const delta = choice?.delta;
    if (!delta) continue;

    if (delta.reasoning_content) {
      yield { type: "thinking", text: delta.reasoning_content };
    }

    if (delta.content) {
      yield { type: "text", text: delta.content };
    }

    for (const tc of delta.tool_calls ?? []) {
      const index = tc.index ?? syntheticCallIndex;
      let state = toolCallState.get(index);
      if (!state) {
        state = { id: tc.id ?? `call-${syntheticCallIndex}`, name: "", argumentsJson: "" };
        toolCallState.set(index, state);
      }
      if (tc.id) state.id = tc.id;
      if (tc.function?.name) state.name = tc.function.name;
      if (tc.function?.arguments) state.argumentsJson += tc.function.arguments;

      // Emit as soon as we have an id + name; the agent loop accumulates
      // subsequent argument fragments onto the same id.
      if (state.name) {
        yield {
          type: "toolCall",
          id: state.id,
          name: state.name,
          argumentsJson: state.argumentsJson,
        };
        state.argumentsJson = "";
      }
    }
  }
};
