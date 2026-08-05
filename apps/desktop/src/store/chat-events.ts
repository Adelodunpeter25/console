import type { AgentSessionEvent } from "@console/types";
import type { ChatSessionState } from "../types/chat";

export function applyChatEvent(
  session: ChatSessionState,
  event: AgentSessionEvent,
): ChatSessionState {
  switch (event.type) {
    case "modelStreamPart": {
      const text = event.part?.text;
      const thinking = event.part?.thinking;
      if (!text && !thinking) return session;
      return {
        ...session,
        streamingText: text ? session.streamingText + text : session.streamingText,
        streamingThinking: thinking
          ? session.streamingThinking + thinking
          : session.streamingThinking,
      };
    }
    case "modelStreamEnd":
      if (event.turn) {
        return {
          ...session,
          messages: [...session.messages, event.turn],
          streamingText: "",
          streamingThinking: "",
        };
      }
      if (!session.streamingText && !session.streamingThinking) return session;
      return {
        ...session,
        messages: [
          ...session.messages,
          {
            role: "assistant",
            content: [
              ...(session.streamingThinking
                ? [{ type: "thinking" as const, text: session.streamingThinking }]
                : []),
              ...(session.streamingText
                ? [{ type: "text" as const, text: session.streamingText }]
                : []),
            ],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    case "toolExecutionResult":
      return {
        ...session,
        liveToolResults: [...session.liveToolResults, event.result],
      };
    case "toolExecutionEnd": {
      const results = [...event.results];
      for (const live of session.liveToolResults) {
        if (!results.some((result) => result.toolCallId === live.toolCallId)) {
          results.push(live);
        }
      }
      return {
        ...session,
        messages: [...session.messages, { role: "toolResult", results }],
        liveToolResults: [],
      };
    }
    case "askQuestion":
      return { ...session, pendingQuestion: { request: event.request } };
    case "permissionRequest":
      return {
        ...session,
        pendingPermissions: [...session.pendingPermissions, { request: event.request }],
      };
    case "error":
      if (event.error?.message.toLowerCase().includes("aborted")) {
        return { ...session, streamingText: "", streamingThinking: "" };
      }
      return {
        ...session,
        messages: [
          ...session.messages,
          {
            role: "assistant",
            content: [{ type: "text", text: `Error: ${event.error?.message ?? "Unknown agent error"}` }],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    default:
      return session;
  }
}
