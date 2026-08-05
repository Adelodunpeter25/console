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
        runActivity: {
          ...session.runActivity,
          results: session.runActivity.results.some((result) => result.toolCallId === event.result.toolCallId)
            ? session.runActivity.results.map((result) =>
                result.toolCallId === event.result.toolCallId ? event.result : result,
              )
            : [...session.runActivity.results, event.result],
        },
      };
    case "toolExecutionStart": {
      const calls = [...session.runActivity.calls];
      for (const call of event.calls) {
        if (!calls.some((existing) => existing.id === call.id)) calls.push(call);
      }
      return {
        ...session,
        activeToolCalls: event.calls,
        liveToolResults: [],
        runActivity: { ...session.runActivity, calls },
      };
    }
    case "toolExecutionEnd": {
      const results = [...event.results];
      for (const live of session.liveToolResults) {
        if (!results.some((result) => result.toolCallId === live.toolCallId)) {
          results.push(live);
        }
      }
      for (const call of session.activeToolCalls) {
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
        ...session,
        messages: [...session.messages, { role: "toolResult", results }],
        liveToolResults: [],
        activeToolCalls: [],
        runActivity: {
          ...session.runActivity,
          results: results.reduce((current, result) => {
            const existing = current.findIndex((item) => item.toolCallId === result.toolCallId);
            if (existing === -1) return [...current, result];
            const next = [...current];
            next[existing] = result;
            return next;
          }, session.runActivity.results),
        },
      };
    }
    case "askQuestion":
      return { ...session, pendingQuestion: { request: event.request } };
    case "permissionRequest":
      return {
        ...session,
        pendingPermissions: [...session.pendingPermissions, { request: event.request }],
      };
    case "todoUpdate":
      return { ...session, todoItems: event.items };
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
