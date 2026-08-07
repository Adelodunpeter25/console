import React from "react";
import type { AgentMessage, ToolResult } from "@console/types";
import type { RunActivityState } from "../types/chat.js";

interface UseMessageHistoryOptions {
  history: string[];
  value: string;
  onChange: (value: string) => void;
}

/** Provides shell-style Up/Down navigation through previous chat prompts. */
export function useMessageHistory({ history, value, onChange }: UseMessageHistoryOptions) {
  const indexRef = React.useRef<number | null>(null);
  const draftRef = React.useRef("");

  const reset = React.useCallback(() => {
    indexRef.current = null;
  }, []);

  const navigate = React.useCallback(
    (direction: -1 | 1, event: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (history.length === 0) return;
      const cursor = event.currentTarget.selectionStart;
      if (direction < 0 && cursor !== 0) return;
      if (direction > 0 && cursor !== value.length) return;

      event.preventDefault();
      if (direction < 0) {
        if (indexRef.current === null) draftRef.current = value;
        const next =
          indexRef.current === null ? history.length - 1 : Math.max(0, indexRef.current - 1);
        indexRef.current = next;
        onChange(history[next] ?? "");
        return;
      }

      if (indexRef.current === null) return;
      const next = indexRef.current + 1;
      if (next >= history.length) {
        indexRef.current = null;
        onChange(draftRef.current);
      } else {
        indexRef.current = next;
        onChange(history[next] ?? "");
      }
    },
    [history, onChange, value],
  );

  return { navigate, reset };
}

export function reconstructRuns(messages: AgentMessage[]): RunActivityState[] {
  const runs: RunActivityState[] = [];

  // Walk the message list and split into runs at each user message.
  // Each run contains all assistant turns, tool calls, and tool results
  // until the next user message, rebuilt as an ordered event timeline.
  let currentRun: RunActivityState | null = null;
  let runIndex = 0;

  // Track results from toolResult messages and match them to tool call events
  // by toolCallId. Results may arrive in a separate message after the assistant
  // turn, so we collect them and apply them after building the events.
  const pendingResults: Map<string, ToolResult> = new Map();

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
function applyPendingResults(run: RunActivityState, results: Map<string, ToolResult>): void {
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
