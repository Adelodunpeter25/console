// Centralized git service: branches, checkout, diff, status.

import { Console } from "console-nsh/native";
import type { GitBranchInfo } from "../common/workspace_footer";

async function listBranches(
  baseUrl: string,
  repoPath: string,
): Promise<GitBranchInfo[]> {
  try {
    const raw = await Console.gitBranches(baseUrl, repoPath);
    const parsed = JSON.parse(raw) as {
      branches?: Array<{ name: string; current?: boolean }>;
    };
    return (parsed.branches ?? []).map((b) => ({
      name: b.name,
      current: b.current ?? false,
    }));
  } catch {
    return [];
  }
}

async function getStatus(baseUrl: string, repoPath: string): Promise<string> {
  return Console.gitStatus(baseUrl, repoPath);
}

async function getDiff(
  baseUrl: string,
  repoPath: string,
  filePath?: string | null,
): Promise<string> {
  return Console.gitDiff(baseUrl, repoPath, filePath);
}

export const GitService = {
  listBranches,
  getStatus,
  getDiff,
};
