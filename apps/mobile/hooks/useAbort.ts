import { useCallback } from "react";
import { abort as abortChat } from "@/stores/useChatStore";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

/** Wraps the chat-store abort for the mobile stop button. */
export function useAbort() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  
  const handleAbort = useCallback(async () => {
    if (!selectedSessionId) return;
    try {
      await abortChat(selectedSessionId);
    } catch (err) {
      console.error("Abort run error:", err);
    }
  }, [selectedSessionId]);

  return {
    abort: handleAbort,
  };
}

