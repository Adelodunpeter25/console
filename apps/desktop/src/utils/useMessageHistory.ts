import React from "react";
import type { AgentMessage } from "@console/types";
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
  // until the next user message.
  let currentRun: RunActivityState | null = null;
  let runIndex = 0;

  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;

    if (msg.role === "user") {
      // Finalize the previous run.
      if (currentRun) {
        finalizeReconstructedRun(currentRun, messages, i - 1);
        runs.push(currentRun);
      }
      // Start a new run.
      currentRun = {
        runId: `reconstructed-${runIndex++}`,
        startedAt: msg.createdAt ?? null,
        elapsedMs: 0,
        calls: [],
        results: [],
        status: "completed",
      };
    } else if (currentRun) {
      if (msg.role === "assistant") {
        const msgCalls = msg.content
          .filter((c): c is Extract<typeof c, { type: "toolCall" }> => c.type === "toolCall")
          .map((c) => c.call);
        currentRun.calls.push(...msgCalls);
      } else if (msg.role === "toolResult") {
        currentRun.results.push(...msg.results);
      }
    }
  }

  // Finalize the last run.
  if (currentRun) {
    finalizeReconstructedRun(currentRun, messages, messages.length - 1);
    runs.push(currentRun);
  }

  return runs;
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
