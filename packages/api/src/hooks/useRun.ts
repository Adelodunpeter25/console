import { useMutation } from "@tanstack/react-query";
import { runService } from "../services/run.service";

export function useAbortRun() {
  return useMutation({
    mutationFn: (sessionId: string) => runService.abortRun(sessionId),
  });
}
