import axios from "axios";
import { getConsoleApiClient } from "../client";
import type { ProjectInfo, FsTreeEntry, FileSearchResult } from "@console/types";

export const fsService = {
  async getProjects(): Promise<ProjectInfo[]> {
    const res = await getConsoleApiClient().get("/api/projects");
    return res.data.data;
  },

  async addProject(path: string): Promise<ProjectInfo> {
    const res = await getConsoleApiClient().post("/api/projects", { path });
    return res.data.data;
  },

  async deleteProject(projectId: string): Promise<{ id: string; deleted: boolean }> {
    const res = await getConsoleApiClient().delete(`/api/projects/${encodeURIComponent(projectId)}`);
    return res.data.data;
  },

  async getFsBrowse(
    path?: string,
  ): Promise<{ currentPath: string; parentPath: string | null; entries: FsTreeEntry[] }> {
    const res = await getConsoleApiClient().get("/api/fs/browse", { params: { path } });
    return res.data.data ?? res.data;
  },

  async getFsTree(path?: string): Promise<FsTreeEntry[]> {
    const res = await getConsoleApiClient().get("/api/fs/tree", { params: { path } });
    return res.data.data ?? res.data;
  },

  async getFsEntries(path: string, depth = 1): Promise<FsTreeEntry[]> {
    const res = await getConsoleApiClient().get("/api/fs/entries", { params: { path, depth } });
    return res.data.data ?? res.data;
  },

  async searchFiles(root: string, query: string, limit = 20): Promise<FileSearchResult[]> {
    const res = await getConsoleApiClient().get("/api/fs/search", { params: { root, q: query, limit } });
    return res.data.data ?? [];
  },

  async readFile(path: string): Promise<{ content: string; path: string }> {
    try {
      const res = await getConsoleApiClient().get("/api/fs/file", { params: { path } });
      return res.data.data ?? res.data;
    } catch (err) {
      // Surface the server's structured preview-gate rejection (413/415) so UIs
      // show the real reason instead of "Request failed with status code 413".
      const data = axios.isAxiosError(err) ? err.response?.data : undefined;
      if (data && typeof data.error === "string") {
        const wrapped = new Error(data.error) as Error & { code?: string };
        wrapped.code = typeof data.code === "string" ? data.code : undefined;
        throw wrapped;
      }
      throw err;
    }
  },

  async writeFile(path: string, content: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post("/api/fs/file", { path, content });
    return res.data.data ?? res.data;
  },

  async deleteFile(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete("/api/fs/file", { params: { path } });
    return res.data.data ?? res.data;
  },

  async createDir(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post("/api/fs/dir", { path });
    return res.data.data ?? res.data;
  },

  async deleteDir(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete("/api/fs/dir", { params: { path } });
    return res.data.data ?? res.data;
  },
};
