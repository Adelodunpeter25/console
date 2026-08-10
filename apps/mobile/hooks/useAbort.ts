import { useCallback } from "react";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";

/** Wraps the chat-store abort for the mobile stop button. */
export function useAbort() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const abort = useChatStore((state) => state.abort);
  const running = useChatStore(
    useCallback(
      (state) => (selectedSessionId ? state.sessions[selectedSessionId]?.running : false),
      [selectedSessionId],
    ),
  );

  const handleAbort = useCallback(async () => {
    if (!selectedSessionId) return;
    try {
      await abort(selectedSessionId);
    } catch (err) {
      console.error("Abort run error:", err);
    }
  }, [abort, selectedSessionId]);

  return {
    abort: handleAbort,
    isAborting: Boolean(running),
  };
}
