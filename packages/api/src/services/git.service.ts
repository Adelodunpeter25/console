import { getConsoleApiClient } from "../client";
import type { GitStatusSummary } from "@console/types";

export const gitService = {
  async getDiff(repoPath: string, filePath?: string): Promise<string | null> {
    const res = await getConsoleApiClient().get("/api/git/diff", { params: { repoPath, ...(filePath ? { path: filePath } : {}) } });
    return res.data?.data?.diff ?? null;
  },
  async getStatus(path: string): Promise<GitStatusSummary | null> {
    const res = await getConsoleApiClient().get("/api/git/status", { params: { path } });
    return res.data?.data ?? null;
  },
};