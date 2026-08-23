import { useCallback, useEffect, useState } from "react";
import { getConsoleApiClient } from "@console/api";
import type { SlashCommandInfo } from "@console/types";
import { useAppStore } from "@/stores/useAppStore";

/** List slash commands for a session (built-in + discovered). */
export function useSlashCommands(sessionId?: string) {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const [commands, setCommands] = useState<SlashCommandInfo[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchCommands = useCallback(async () => {
    if (!sessionId || !backendUrl) return;
    setLoading(true);
    try {
      const res = await getConsoleApiClient().get(`/api/assist/${sessionId}/commands`);
      setCommands((res.data?.data as SlashCommandInfo[]) ?? []);
    } catch (err) {
      console.error("Failed to load slash commands:", err);
    } finally {
      setLoading(false);
    }
  }, [backendUrl, sessionId]);

  useEffect(() => {
    fetchCommands();
  }, [fetchCommands]);

  return { commands, loading, refetch: fetchCommands };
}
