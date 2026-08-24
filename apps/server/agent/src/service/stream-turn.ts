import { extractThinkingFromText } from "./thinking.js";
import { parseToolCallArguments } from "@/agent/src/utils/model-turn.js";
import type { AgentSessionEvent, AssistantMessage, ToolCall } from "@/agent/src/types/index.js";
import type { StreamFn, StreamParams } from "./types.js";

/**
 * Stream one turn from the LLM, collecting all text, thinking, and tool-call deltas.
 */
export async function streamOneTurn(
  params: StreamParams,
  streamFn: StreamFn,
  turnId: string,
  emit: (event: AgentSessionEvent) => void,
): Promise<AssistantMessage> {
  emit({ type: "modelStreamStart", turnId });

  let textAccumulator = "";
  const textParts: Array<{ text: string; thoughtSignature?: string }> = [];
  let thinkingAccumulator = "";
  // Ordered segments in arrival order: each entry is either a thinking block,
  // a tool call id, or a text part index. This preserves the model's real
  // timeline (think → tool → think → tool) instead of grouping all thinking
  // ahead of every tool call.
  const segments: Array<
    | { kind: "thinking"; text: string }
    | { kind: "toolCall"; id: string }
    | { kind: "text"; partIndex: number }
  > = [];
  const toolCallMap = new Map<
    string,
    {
      id: string;
      name: string;
      argumentsJson: string;
      thoughtSignature?: string;
    }
  >();
  const toolCallOrder: string[] = [];

  const stream = streamFn(params);
  for await (const delta of stream) {
    if (params.signal?.aborted) break;

    if (delta.type === "text") {
      textAccumulator += delta.text;
      emit({ type: "modelStreamPart", part: { text: delta.text } });
      if (delta.thoughtSignature) {
        textParts.push({ text: textAccumulator, thoughtSignature: delta.thoughtSignature });
        textAccumulator = "";
        segments.push({ kind: "text", partIndex: textParts.length - 1 });
      }
    } else if (delta.type === "thinking") {
      thinkingAccumulator += delta.text;
      emit({ type: "modelStreamPart", part: { thinking: delta.text } });
    } else if (delta.type === "toolCall") {
      const existing = toolCallMap.get(delta.id);
      if (existing) {
        existing.argumentsJson += delta.argumentsJson;
        if (delta.thoughtSignature) {
          existing.thoughtSignature = delta.thoughtSignature;
        }
      } else {
        // A tool call's first delta: flush thinking that streamed before it so
        // the segment list reflects true order (think before this tool).
        if (thinkingAccumulator) {
          segments.push({ kind: "thinking", text: thinkingAccumulator });
          thinkingAccumulator = "";
        }
        toolCallMap.set(delta.id, {
          id: delta.id,
          name: delta.name,
          argumentsJson: delta.argumentsJson,
          thoughtSignature: delta.thoughtSignature,
        });
        toolCallOrder.push(delta.id);
        segments.push({ kind: "toolCall", id: delta.id });
        emit({
          type: "modelStreamPart",
          part: { toolCall: { id: delta.id, name: delta.name } },
        });
      }
    }
  }

  // Flush trailing thinking.
  if (thinkingAccumulator) {
    segments.push({ kind: "thinking", text: thinkingAccumulator });
    thinkingAccumulator = "";
  }
  // Flush any remaining text that never received a thought signature.
  if (textAccumulator) {
    textParts.push({ text: textAccumulator });
    textAccumulator = "";
    segments.push({ kind: "text", partIndex: textParts.length - 1 });
  }

  const toolCalls: ToolCall[] = toolCallOrder.map((id) => {
    const tc = toolCallMap.get(id)!;
    const args = parseToolCallArguments(tc.argumentsJson);
    return {
      id: tc.id,
      name: tc.name,
      arguments: args,
      ...(tc.thoughtSignature ? { thoughtSignature: tc.thoughtSignature } : {}),
    };
  });

  // Assemble content in segment order.
  const content: AssistantMessage["content"] = [];
  const pushTextPart = (part: { text: string; thoughtSignature?: string }) => {
    const extracted = extractThinkingFromText(part.text);
    content.push(...extracted.thinkingParts);
    if (extracted.textParts.length > 0) {
      const last = extracted.textParts.length - 1;
      content.push(
        ...extracted.textParts.map((textPart, index) =>
          index === last && part.thoughtSignature
            ? { ...textPart, thoughtSignature: part.thoughtSignature }
            : textPart,
        ),
      );
    } else if (part.thoughtSignature) {
      content.push({ type: "text", text: "", thoughtSignature: part.thoughtSignature });
    }
  };
  for (const seg of segments) {
    if (seg.kind === "thinking") {
      content.push({ type: "thinking", text: seg.text });
    } else if (seg.kind === "toolCall") {
      const tc = toolCalls.find((t) => t.id === seg.id);
      if (tc) content.push({ type: "toolCall", call: tc });
    } else {
      pushTextPart(textParts[seg.partIndex]!);
    }
  }

  if (!params.signal?.aborted && content.length === 0) {
    throw new Error("The model returned an empty response.");
  }

  const stopReason: AssistantMessage["stopReason"] = toolCalls.length > 0 ? "toolUse" : "stop";

  return {
    role: "assistant",
    id: turnId,
    content,
    stopReason,
  };
}
