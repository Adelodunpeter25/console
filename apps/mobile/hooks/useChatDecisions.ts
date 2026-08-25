import { useCallback } from "react";
import {
  answerQuestion as answerChatQuestion,
  approvePermission as approveChatPermission,
} from "@/stores/useChatStore";
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

  const answer = useCallback(
    async (requestId: string, answer: string | string[]) => {
      if (!selectedSessionId) return;
      try {
        await answerChatQuestion(selectedSessionId, requestId, answer);
      } catch (err) {
        console.error("Failed to answer question:", err);
      }
    },
    [selectedSessionId],
  );

  const approve = useCallback(
    async (requestId: string, allow: boolean) => {
      if (!selectedSessionId) return;
      try {
        await approveChatPermission(selectedSessionId, requestId, allow);
      } catch (err) {
        console.error("Failed to approve permission:", err);
      }
    },
    [selectedSessionId],
  );

  return {
    answer,
    approve,
    isAnswering: false,
    isApproving: false,
  };
}
