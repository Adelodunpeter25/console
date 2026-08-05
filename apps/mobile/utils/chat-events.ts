import type { AgentSessionEvent } from "@console/types";
import type { ChatSnapshot } from "../types";

/**
 * Pure reducer applying an agent session event to the current chat snapshot.
 * Mirrors the desktop chat-events reducer and adds mobile handling for
 * permission requests and ask-tool questions so runs never hang waiting.
 */
export function applyChatEvent(snapshot: ChatSnapshot, event: AgentSessionEvent): ChatSnapshot {
  switch (event.type) {
    case "modelStreamPart": {
      const text = event.part?.text;
      const thinking = event.part?.thinking;
      if (!text && !thinking) return snapshot;
      return {
        ...snapshot,
        streamingText: text ? snapshot.streamingText + text : snapshot.streamingText,
        streamingThinking: thinking
          ? snapshot.streamingThinking + thinking
          : snapshot.streamingThinking,
      };
    }

    case "modelStreamEnd": {
      if (event.turn) {
        return {
          ...snapshot,
          messages: [...snapshot.messages, event.turn],
          streamingText: "",
          streamingThinking: "",
        };
      }
      if (!snapshot.streamingText && !snapshot.streamingThinking) return snapshot;
      return {
        ...snapshot,
        messages: [
          ...snapshot.messages,
          {
            role: "assistant",
            content: [
              ...(snapshot.streamingThinking
                ? [{ type: "thinking" as const, text: snapshot.streamingThinking }]
                : []),
              ...(snapshot.streamingText
                ? [{ type: "text" as const, text: snapshot.streamingText }]
                : []),
            ],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    }

    case "toolExecutionStart":
      return {
        ...snapshot,
        activeToolCalls: event.calls,
        liveToolResults: [],
      };

    case "toolExecutionResult":
      return {
        ...snapshot,
        liveToolResults: [...snapshot.liveToolResults, event.result],
      };

    case "toolExecutionEnd": {
      const results = [...event.results];
      for (const live of snapshot.liveToolResults) {
        if (!results.some((result) => result.toolCallId === live.toolCallId)) {
          results.push(live);
        }
      }
      for (const call of snapshot.activeToolCalls) {
        if (!results.some((result) => result.toolCallId === call.id)) {
          results.push({
            toolCallId: call.id,
            toolName: call.name,
            content: "Tool execution ended without a result.",
            isError: true,
          });
        }
      }
      return {
        ...snapshot,
        messages: [...snapshot.messages, { role: "toolResult", results }],
        liveToolResults: [],
        activeToolCalls: [],
      };
    }

    case "permissionRequest":
      return {
        ...snapshot,
        pendingPermission: { request: event.request },
      };

    case "askQuestion":
      return {
        ...snapshot,
        pendingQuestion: { request: event.request },
      };

    case "error": {
      if (event.error?.message.toLowerCase().includes("aborted")) {
        return { ...snapshot, streamingText: "", streamingThinking: "" };
      }
      return {
        ...snapshot,
        messages: [
          ...snapshot.messages,
          {
            role: "assistant",
            content: [{ type: "text", text: `Error: ${event.error?.message ?? "Unknown agent error"}` }],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    }

    default:
      return snapshot;
  }
}
