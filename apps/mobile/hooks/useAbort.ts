import { useCallback } from "react";
import { useAbortRun } from "@console/api";
import { useAppStore } from "../stores/useAppStore";

/** Wraps the shared abort-run mutation for the mobile stop button. */
export function useAbort() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const abortRun = useAbortRun();

  const abort = useCallback(async () => {
    if (!selectedSessionId) return;
    try {
      await abortRun.mutateAsync(selectedSessionId);
    } catch (err) {
      console.error("Abort run error:", err);
    }
  }, [abortRun, selectedSessionId]);

  return {
    abort,
    isAborting: abortRun.isPending,
  };
}
