// The live-run wiring: maps one `AgentSessionEvent` (the camelCase JSON the
// native bridge forwards) onto a transcript state, the way the desktop's
// `run.rs` `process_agent_event` does. Pure functions over the event JSON,
// so the pane and its tests share one mapping.
//
// Events the pane renders today: turn lifecycle, model stream parts and the
// final turn, tool calls and results. The desktop's remaining events
// (permissions, questions, browser actions, subagents, background work,
// queueing) land with their interaction cards — see the migration plan.

import type { AssistantContentPart } from "./message_assistant";
import type { TranscriptState } from "./transcript_state";
import type { ToolCall, ToolResult } from "./toolcalls/model";

/// The event the native bridge forwards: `AgentSessionEvent`'s serde form
/// (tagged `type`, camelCase fields; proto payloads in pbjson's camelCase,
/// byte fields base64).
export type ChatRunEvent = { type: string } & Record<string, unknown>;

/// pbjson byte field → text. Returns null when the field is absent or not
/// valid UTF-8 JSON, which the renderer already tolerates.
function decodeBytes(value: unknown): string | null {
  if (typeof value !== "string") return null;
  try {
    return atob(value);
  } catch {
    return null;
  }
}

function mapToolCall(raw: Record<string, unknown>): ToolCall {
  const args = decodeBytes(raw["arguments"]);
  let parsed: unknown = null;
  if (args != null) {
    try {
      parsed = JSON.parse(args);
    } catch {
      parsed = args;
    }
  }
  return {
    id: String(raw["id"] ?? ""),
    name: String(raw["name"] ?? ""),
    arguments: parsed,
    label: (raw["label"] as string | undefined) ?? null,
    kindFlags: (raw["kindFlags"] as number | undefined) ?? null,
  };
}

function mapToolResult(raw: Record<string, unknown>): ToolResult {
  return {
    toolCallId: String(raw["toolCallId"] ?? ""),
    content: decodeBytes(raw["content"]),
    isError: (raw["isError"] as boolean | undefined) ?? null,
  };
}

const streamPart = (part: Record<string, unknown> | undefined) => {
  if (part == null) return null;
  if (typeof part["text"] === "string") return { kind: "text" as const, text: part["text"] };
  if (typeof part["thinking"] === "string") return { kind: "thinking" as const, text: part["thinking"] };
  if (typeof part["toolCall"] === "object" && part["toolCall"] != null) {
    return { kind: "toolCall" as const, call: mapToolCall(part["toolCall"] as Record<string, unknown>) };
  }
  return null;
};

/// Applies one event. Returns whether the run stream has ended (`sessionEnd`
/// or a terminal error), so the caller knows its `run_prompt` promise is the
/// only remaining lifecycle step. `interactive` receives the two requests
/// that pause the run for the user (permissions, questions) — the pane owns
/// their display and answers, the way the desktop hangs pending requests off
/// the session rather than the transcript.
export function applyRunEvent(
  state: TranscriptState,
  event: ChatRunEvent,
  interactive?: {
    onPermission: (request: Record<string, unknown> | null) => void;
    onQuestion: (request: Record<string, unknown> | null) => void;
  },
): boolean {
  const inter = interactive;
  switch (event["type"]) {
    case "turnStart":
      state.beginStreaming();
      return false;
    case "modelStreamPart": {
      const part = streamPart(event["part"] as Record<string, unknown> | undefined);
      if (part?.kind === "text") state.appendAssistantText(part.text);
      else if (part?.kind === "thinking") state.appendAssistantThinking(part.text);
      else if (part?.kind === "toolCall") state.upsertAssistantToolCall(part.call);
      return false;
    }
    case "modelStreamEnd": {
      // The final turn replaces the streamed shadow with the persisted form.
      const turn = event["turn"] as Record<string, unknown> | undefined;
      if (turn != null) {
        state.finalizeAssistantMessage({
          id: typeof turn["id"] === "string" ? turn["id"] : undefined,
          content: Array.isArray(turn["content"])
            ? (turn["content"] as AssistantContentPartJson[]).map(mapContentPart)
            : ([] as AssistantContentPart[]),
          stopReason: typeof turn["stopReason"] === "string" ? turn["stopReason"] : undefined,
          createdAt: typeof turn["createdAt"] === "number" ? turn["createdAt"] : undefined,
          tokPerSec: typeof turn["tokPerSec"] === "number" ? turn["tokPerSec"] : undefined,
        });
      }
      state.finishStreaming();
      return false;
    }
    case "toolExecutionStart":
      state.upsertAssistantToolCalls(
        ((event["calls"] as unknown[]) ?? []).map((c) => mapToolCall(c as Record<string, unknown>)),
      );
      return false;
    case "toolExecutionResult":
      state.appendToolResults([mapToolResult(event["result"] as Record<string, unknown>)]);
      return false;
    case "toolExecutionEnd":
      state.appendToolResults(
        ((event["results"] as unknown[]) ?? []).map((r) => mapToolResult(r as Record<string, unknown>)),
      );
      return false;
    case "permissionRequest":
      inter?.onPermission(
        (event["request"] as Record<string, unknown> | undefined) ?? null,
      );
      return false;
    case "askQuestion":
      inter?.onQuestion(
        (event["request"] as Record<string, unknown> | undefined) ?? null,
      );
      return false;
    case "sessionEnd":
      state.finishStreaming();
      return true;
    case "error":
      state.finishStreaming();
      return true;
    default:
      // Unrendered events (permissions, subagents, …) leave the stream state
      // alone; the run continues.
      return false;
  }
}

// pbjson's AssistantContentPart: a oneof under `part`.
type AssistantContentPartJson = { part?: Record<string, unknown> };

function mapContentPart(raw: AssistantContentPartJson): AssistantContentPart {
  const part = raw["part"] as Record<string, unknown> | undefined;
  if (part == null) return { type: "text", text: "" };
  if (typeof part["text"] === "object" && part["text"] != null) {
    return {
      type: "text",
      text: String((part["text"] as Record<string, unknown>)["text"] ?? ""),
    };
  }
  if (typeof part["thinking"] === "object" && part["thinking"] != null) {
    return { type: "thinking", text: String((part["thinking"] as Record<string, unknown>)["text"] ?? "") };
  }
  if (typeof part["toolCall"] === "object" && part["toolCall"] != null) {
    return { type: "toolCall", call: mapToolCall(part["toolCall"] as Record<string, unknown>) };
  }
  if (typeof part["image"] === "object" && part["image"] != null) {
    const image = part["image"] as Record<string, unknown>;
    return { type: "image", data: String(image["data"] ?? ""), mimeType: String(image["mimeType"] ?? "") };
  }
  return { type: "text", text: "" };
}
