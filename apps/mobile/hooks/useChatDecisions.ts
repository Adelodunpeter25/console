import { useCallback } from "react";
import { useAnswerQuestion, useApprovePermission } from "@console/api";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";

/**
 * Posts agent decisions (question answers / permission approvals) to the
 * backend and clears the pending UI state once delivered. The server
 * consumes each pending request exactly once, so the panel clears regardless
 * of the outcome.
 */
export function useChatDecisions() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const clearPending = useChatStore((state) => state.clearPending);
  const answerQuestion = useAnswerQuestion();
  const approvePermission = useApprovePermission();

  const answer = useCallback(
    async (requestId: string, answer: string | string[]) => {
      if (!selectedSessionId) return;
      try {
        await answerQuestion.mutateAsync({
          sessionId: selectedSessionId,
          payload: { requestId, answer },
        });
      } catch (err) {
        console.error("Failed to answer question:", err);
      } finally {
        clearPending();
      }
    },
    [answerQuestion, clearPending, selectedSessionId],
  );

  const approve = useCallback(
    async (requestId: string, allow: boolean) => {
      if (!selectedSessionId) return;
      try {
        await approvePermission.mutateAsync({
          sessionId: selectedSessionId,
          payload: { requestId, allow },
        });
      } catch (err) {
        console.error("Failed to approve permission:", err);
      } finally {
        clearPending();
      }
    },
    [approvePermission, clearPending, selectedSessionId],
  );

  return {
    answer,
    approve,
    isAnswering: answerQuestion.isPending,
    isApproving: approvePermission.isPending,
  };
}
