import { useCallback } from "react";
import { useChatStore } from "@/stores/useChatStore";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

/**
 * Posts agent decisions (question answers / permission approvals) to the
 * backend and clears the pending UI state once delivered. The server
 * consumes each pending request exactly once, so the panel clears regardless
 * of the outcome.
 */
export function useChatDecisions() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const answerQuestion = useChatStore((state) => state.answerQuestion);
  const approvePermission = useChatStore((state) => state.approvePermission);

  const answer = useCallback(
    async (requestId: string, answer: string | string[]) => {
      if (!selectedSessionId) return;
      try {
        await answerQuestion(selectedSessionId, requestId, answer);
      } catch (err) {
        console.error("Failed to answer question:", err);
      }
    },
    [answerQuestion, selectedSessionId],
  );

  const approve = useCallback(
    async (requestId: string, allow: boolean) => {
      if (!selectedSessionId) return;
      try {
        await approvePermission(selectedSessionId, requestId, allow);
      } catch (err) {
        console.error("Failed to approve permission:", err);
      }
    },
    [approvePermission, selectedSessionId],
  );

  return {
    answer,
    approve,
    isAnswering: false,
    isApproving: false,
  };
}
