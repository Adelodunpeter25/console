import { useMutation } from "@tanstack/react-query";
import { runService } from "../services/run.service.js";

export function useAbortRun() {
  return useMutation({
    mutationFn: (sessionId: string) => runService.abortRun(sessionId),
  });
}
