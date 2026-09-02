import { useEffect, useMemo, useRef, useState } from "react";
import { useValue } from "@legendapp/state/react";
import type { BottomSheetModal } from "@gorhom/bottom-sheet";
import { sessionService } from "@console/api";
import type { SubagentInfo } from "@console/types";
import { chat$, setSubagents, getChatSession } from "@/stores/useChatStore";

export function useSessionSubagents(sessionId: string | null | undefined) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);
  const [selectedSubagentId, setSelectedSubagentId] = useState<string | null>(null);

  // Read reactive subagents for the active session from Legend State store
  const subagents = useValue(() => {
    if (!sessionId) return [] as SubagentInfo[];
    return chat$.sessions[sessionId]?.subagents.get() ?? ([] as SubagentInfo[]);
  });

  // Initial hydration from backend on session mount/change
  useEffect(() => {
    if (!sessionId) return;
    let cancelled = false;

    sessionService
      .getSubagents(sessionId)
      .then((serverSubagents) => {
        if (cancelled) return;
        if (Array.isArray(serverSubagents)) {
          const current = getChatSession(sessionId);
          // Only hydrate if we don't already have live items or if current is empty
          if (!current.subagents || current.subagents.length === 0) {
            setSubagents(sessionId, serverSubagents);
          }
        }
      })
      .catch(() => {
        // Silently ignore network errors during initial hydration
      });

    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  const totalCount = subagents.length;

  const runningSubagents = useMemo(() => {
    return subagents.filter((s) => s.status === "running");
  }, [subagents]);

  const hasSubagents = subagents.length > 0;
  const hasRunningSubagents = runningSubagents.length > 0;

  const latestSubagent = useMemo(() => {
    if (subagents.length === 0) return undefined;
    return subagents[subagents.length - 1];
  }, [subagents]);

  const openSheet = (subagentId?: string) => {
    if (subagentId) {
      setSelectedSubagentId(subagentId);
    }
    bottomSheetRef.current?.present();
  };

  const closeSheet = () => {
    bottomSheetRef.current?.dismiss();
  };

  return {
    subagents,
    totalCount,
    runningSubagents,
    hasSubagents,
    hasRunningSubagents,
    latestSubagent,
    selectedSubagentId,
    setSelectedSubagentId,
    bottomSheetRef,
    openSheet,
    closeSheet,
  };
}
