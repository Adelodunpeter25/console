import { getConsoleApiClient } from "../client";
import type { GitStatusSummary } from "@console/types";

export const gitService = {
  async getStatus(path: string): Promise<GitStatusSummary | null> {
    const res = await getConsoleApiClient().get("/api/git/status", { params: { path } });
    return res.data?.data ?? null;
  },
};