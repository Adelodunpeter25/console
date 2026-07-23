import { getConsoleApiClient } from "../client.js";
import type { ProjectInfo, FsTreeEntry } from "@console/types";

export const fsService = {
  async getProjects(): Promise<ProjectInfo[]> {
    const res = await getConsoleApiClient().get("/api/projects");
    return res.data.data;
  },

  async addProject(path: string): Promise<ProjectInfo> {
    const res = await getConsoleApiClient().post("/api/projects", { path });
    return res.data.data;
  },

  async getFsBrowse(
    path?: string,
  ): Promise<{ currentPath: string; parentPath: string | null; entries: FsTreeEntry[] }> {
    const res = await getConsoleApiClient().get("/api/fs/browse", { params: { path } });
    return res.data;
  },

  async getFsTree(path?: string): Promise<FsTreeEntry[]> {
    const res = await getConsoleApiClient().get("/api/fs/tree", { params: { path } });
    return res.data;
  },

  async readFile(path: string): Promise<{ content: string; path: string }> {
    const res = await getConsoleApiClient().get("/api/fs/file", { params: { path } });
    return res.data;
  },

  async writeFile(path: string, content: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post("/api/fs/file", { path, content });
    return res.data;
  },

  async deleteFile(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete("/api/fs/file", { params: { path } });
    return res.data;
  },

  async createDir(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post("/api/fs/dir", { path });
    return res.data;
  },

  async deleteDir(path: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete("/api/fs/dir", { params: { path } });
    return res.data;
  },
};
