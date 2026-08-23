import { getConsoleApiClient } from "../client";
import type { FileSearchResponse, SlashCommandInfo } from "@console/types";

export const assistService = {
  async listSlashCommands(sessionId?: string | null): Promise<SlashCommandInfo[]> {
    const path = sessionId
      ? `/api/assist/${encodeURIComponent(sessionId)}/commands`
      : "/api/assist/commands";
    const res = await getConsoleApiClient().get(path);
    return res.data.data ?? res.data;
  },

  async searchFiles(
    sessionId: string | null | undefined,
    query: string,
    root?: string,
  ): Promise<FileSearchResponse> {
    const base = sessionId ? `/api/assist/${encodeURIComponent(sessionId)}/search` : "/api/assist/search";
    const params: Record<string, string> = { q: query };
    if (root) params.root = root;
    const res = await getConsoleApiClient().get(base, { params });
    return res.data.data ?? res.data;
  },
};
