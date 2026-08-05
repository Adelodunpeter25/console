import type { AgentSessionEvent, ToolResult } from "@console/types";
import type { ChatSessionState, RunActivityState } from "../types/chat";

/** Update the latest run in the session's runs array. */
function updateLatestRun(
  session: ChatSessionState,
  update: (run: RunActivityState) => RunActivityState,
): ChatSessionState {
  if (session.runs.length === 0) return session;
  const runs = [...session.runs];
  runs[runs.length - 1] = update(runs[runs.length - 1]!);
  return { ...session, runs };
}

/** Merge a result into a results array by toolCallId (idempotent). */
function mergeResult(results: ToolResult[], result: ToolResult): ToolResult[] {
  const index = results.findIndex((r) => r.toolCallId === result.toolCallId);
  if (index === -1) return [...results, result];
  const next = [...results];
  next[index] = result;
  return next;
}

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
      return updateLatestRun(session, (run) => ({
        ...run,
        results: mergeResult(run.results, event.result),
      }));
    case "toolExecutionStart": {
      return updateLatestRun(session, (run) => {
        const calls = [...run.calls];
        for (const call of event.calls) {
          if (!calls.some((existing) => existing.id === call.id)) calls.push(call);
        }
        return { ...run, calls };
      });
    }
    case "toolExecutionEnd": {
      // Build the complete results list for this batch.
      const eventResults = [...event.results];
      // Add error results for any active calls that didn't get a result.
      for (const call of session.activeToolCalls) {
        if (!eventResults.some((r) => r.toolCallId === call.id)) {
          eventResults.push({
            toolCallId: call.id,
            toolName: call.name,
            content: "Tool execution ended without a result.",
            isError: true,
          });
        }
      }

      // Merge results into the latest run.
      const updated = updateLatestRun(session, (run) => ({
        ...run,
        results: eventResults.reduce(mergeResult, run.results),
      }));

      // Append the toolResult message to messages for persistence transport.
      // It renders as null in the UI — results are shown via RunActivity.
      return {
        ...updated,
        messages: [...updated.messages, { role: "toolResult", results: eventResults }],
        activeToolCalls: [],
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
    case "sessionEnd":
      // Finalize the latest run: mark as completed if still working, and
      // add error results for any calls that never received a result.
      return updateLatestRun(session, (run) => {
        if (run.status !== "working") return run;
        let results = run.results;
        for (const call of run.calls) {
          if (!results.some((r) => r.toolCallId === call.id)) {
            results = mergeResult(results, {
              toolCallId: call.id,
              toolName: call.name,
              content: "Run ended before this tool call completed.",
              isError: true,
            });
          }
        }
        return {
          ...run,
          status: "completed",
          results,
          elapsedMs: run.startedAt ? Date.now() - run.startedAt : run.elapsedMs,
        };
      });
    case "error":
      if (event.error?.message.toLowerCase().includes("aborted")) {
        return updateLatestRun(session, (run) => ({
          ...run,
          status: run.status === "working" ? "aborted" : run.status,
        }));
      }
      return {
        ...updateLatestRun(session, (run) => ({
          ...run,
          status: run.status === "working" ? "failed" : run.status,
        })),
        messages: [
          ...session.messages,
          {
            role: "assistant",
            content: [
              { type: "text", text: `Error: ${event.error?.message ?? "Unknown agent error"}` },
            ],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    default:
      return session;
  }
}
