import type { AgentMessage } from "@console/types";
import type { RunActivityState } from "@/types/chat";

/**
 * Rebuild run timelines from persisted messages.
 *
 * Walks the message list and splits it into runs at each user message. Each
 * run contains all assistant turns, tool calls, and tool results until the
 * next user message, rebuilt as an ordered event timeline. Results from
 * toolResult messages are matched to tool call events by toolCallId.
 */
export function reconstructRuns(messages: AgentMessage[]): RunActivityState[] {
  const runs: RunActivityState[] = [];

  let currentRun: RunActivityState | null = null;
  let runIndex = 0;

  // Track results from toolResult messages and match them to tool call events
  // by toolCallId. Results may arrive in a separate message after the assistant
  // turn, so we collect them and apply them after building the events.
  const pendingResults = new Map<string, { toolCallId: string; toolName?: string; content: unknown; isError?: boolean }>();

  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;

    if (msg.role === "user") {
      // Finalize the previous run.
      if (currentRun) {
        finalizeReconstructedRun(currentRun, messages, i - 1);
        applyPendingResults(currentRun, pendingResults);
        runs.push(currentRun);
        pendingResults.clear();
      }
      // Start a new run.
      currentRun = {
        runId: `reconstructed-${runIndex++}`,
        startedAt: msg.createdAt ?? null,
        elapsedMs: 0,
        events: [],
        status: "completed",
      };
    } else if (currentRun) {
      if (msg.role === "assistant") {
        const hasToolCalls = msg.content.some((c) => c.type === "toolCall");
        if (!hasToolCalls) continue; // final response — not part of the timeline

        // Build events in content order: thinking → thinking events, text →
        // text events, toolCall → toolCall events.
        for (const part of msg.content) {
          if (part.type === "thinking" && part.text.trim()) {
            currentRun.events.push({
              type: "thinking",
              id: `reconstructed-thinking-${runIndex}-${currentRun.events.length}`,
              text: part.text,
            });
          } else if (part.type === "text" && part.text.trim()) {
            currentRun.events.push({
              type: "text",
              id: `reconstructed-text-${runIndex}-${currentRun.events.length}`,
              text: part.text,
            });
          } else if (part.type === "toolCall") {
            currentRun.events.push({
              type: "toolCall",
              id: part.call.id,
              call: part.call,
            });
          }
        }
      } else if (msg.role === "toolResult") {
        // Collect results — they'll be matched to tool call events after the
        // run is fully built.
        for (const result of msg.results) {
          pendingResults.set(result.toolCallId, result);
        }
      }
    }
  }

  // Finalize the last run.
  if (currentRun) {
    finalizeReconstructedRun(currentRun, messages, messages.length - 1);
    applyPendingResults(currentRun, pendingResults);
    runs.push(currentRun);
  }

  return runs;
}

/** Match collected results to tool call events by toolCallId. */
function applyPendingResults(
  run: RunActivityState,
  results: Map<string, { toolCallId: string; toolName?: string; content: unknown; isError?: boolean }>,
): void {
  run.events = run.events.map((event) =>
    event.type === "toolCall" && results.has(event.call.id)
      ? { ...event, result: results.get(event.call.id) }
      : event,
  );
}

/** Compute elapsed time from the run's startedAt to the last message. */
function finalizeReconstructedRun(
  run: RunActivityState,
  messages: AgentMessage[],
  lastIndex: number,
): void {
  const lastMessage = messages[lastIndex];
  if (run.startedAt && lastMessage?.createdAt && lastMessage.createdAt > run.startedAt) {
    run.elapsedMs = lastMessage.createdAt - run.startedAt;
  }
}
