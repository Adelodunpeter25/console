import { useCallback } from "react";
import { useChatStore } from "@/stores/useChatStore";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

/** Wraps the chat-store abort for the mobile stop button. */
export function useAbort() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const abort = useChatStore((state) => state.abort);

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
  };
}

