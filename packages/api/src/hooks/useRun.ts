import { useMutation } from "@tanstack/react-query";
import { runService } from "../services/run.service";
import type { AnswerQuestionDto, ApproveToolPermissionDto } from "@console/types";

export function useAbortRun() {
  return useMutation({
    mutationFn: (sessionId: string) => runService.abortRun(sessionId),
  });
}

export function useAnswerQuestion() {
  return useMutation({
    mutationFn: ({ sessionId, payload }: { sessionId: string; payload: AnswerQuestionDto }) =>
      runService.answerQuestion(sessionId, payload),
  });
}

export function useApprovePermission() {
  return useMutation({
    mutationFn: ({
      sessionId,
      payload,
    }: {
      sessionId: string;
      payload: ApproveToolPermissionDto;
    }) => runService.approvePermission(sessionId, payload),
  });
}
