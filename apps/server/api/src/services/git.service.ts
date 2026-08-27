import path from "node:path";
import { execShell as execAsync } from "@/api/src/utils/exec.js";
import type {
  GitBranchInfo,
  GitBranchesResponse,
  GitFileStatus,
  GitStatusSummary,
} from "@console/types";

function errorText(error: unknown): string {
  if (error instanceof Error) {
    const stderr = "stderr" in error && typeof error.stderr === "string" ? error.stderr : "";
    return `${error.message}\n${stderr}`.toLowerCase();
  }
  return String(error).toLowerCase();
}

function isNotGitRepositoryError(error: unknown): boolean {
  const message = errorText(error);
  return (
    message.includes("not a git repository") ||
    message.includes("not a repository") ||
    message.includes("no git repository")
  );
}

export class GitService {
  private parseNumstat(output: string, map: Map<string, { additions: number; deletions: number }>): void {
    for (const line of output.split("\n")) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const parts = trimmed.split("\t");
      if (parts.length >= 3) {
        const adds = parts[0] === "-" ? 0 : Number.parseInt(parts[0], 10) || 0;
        const dels = parts[1] === "-" ? 0 : Number.parseInt(parts[1], 10) || 0;
        const file = parts[2];
        const existing = map.get(file) || { additions: 0, deletions: 0 };
        map.set(file, {
          additions: existing.additions + adds,
          deletions: existing.deletions + dels,
        });
      }
    }
  }

  private async getNumstatMap(repoPath: string): Promise<Map<string, { additions: number; deletions: number }>> {
    const map = new Map<string, { additions: number; deletions: number }>();
    try {
      const unstaged = await execAsync("git diff --numstat", { cwd: repoPath });
      this.parseNumstat(unstaged.stdout, map);
    } catch {
      // Ignored
    }
    try {
      const staged = await execAsync("git diff --cached --numstat", { cwd: repoPath });
      this.parseNumstat(staged.stdout, map);
    } catch {
      // Ignored
    }
    return map;
  }

  /**
   * Run git status --porcelain=v1 in the repository directory and return structured status.
   */
  async getGitStatus(repoPath: string): Promise<GitStatusSummary> {
    try {
      // Get current branch name
      const branchRes = await execAsync("git rev-parse --abbrev-ref HEAD", { cwd: repoPath });
      const branch = branchRes.stdout.trim() || "main";

      // Get porcelain git status
      const statusRes = await execAsync("git status --porcelain=v1 -u", { cwd: repoPath });
      const lines = statusRes.stdout.split("\n").filter((line) => line.trim().length > 0);

      const numstatMap = await this.getNumstatMap(repoPath);
      const files: import("@console/types").GitFileEntry[] = [];

      for (const line of lines) {
        const indexStatus = line[0];
        const workTreeStatus = line[1];
        const rawFilePath = line.slice(3).trim();

        // Handle renamed files "fileA -> fileB"
        const filePath = rawFilePath.includes("->")
          ? rawFilePath.split("->")[1]!.trim()
          : rawFilePath;

        const absolutePath = path.resolve(repoPath, filePath);

        let status: GitFileStatus = "?";
        let staged = false;

        if (indexStatus === "?" && workTreeStatus === "?") {
          status = "?";
        } else if (indexStatus === "A" || workTreeStatus === "A") {
          status = "A";
          staged = indexStatus === "A";
        } else if (indexStatus === "M" || workTreeStatus === "M") {
          status = "M";
          staged = indexStatus === "M";
        } else if (indexStatus === "D" || workTreeStatus === "D") {
          status = "D";
          staged = indexStatus === "D";
        } else if (indexStatus === "R" || workTreeStatus === "R") {
          status = "R";
        }

        const stats = numstatMap.get(filePath) || numstatMap.get(rawFilePath);

        files.push({
          path: absolutePath,
          status,
          staged,
          additions: stats?.additions ?? (status === "?" ? 0 : 0),
          deletions: stats?.deletions ?? 0,
        });
      }

      return {
        branch,
        clean: files.length === 0,
        files,
      };
    } catch {
      // Not a git repository or git command failed
      return {
        branch: "",
        clean: true,
        files: [],
      };
    }
  }

  /**
   * Get unified diff for the repository or a specific file.
   */
  async getDiff(repoPath: string, filePath?: string): Promise<string> {
    try {
      const target = filePath ? ` -- "${filePath}"` : "";
      const diffRes = await execAsync(`git diff HEAD${target}`, { cwd: repoPath });
      if (diffRes.stdout.trim()) {
        return diffRes.stdout;
      }
      const unstagedRes = await execAsync(`git diff${target}`, { cwd: repoPath });
      return unstagedRes.stdout;
    } catch {
      try {
        const target = filePath ? ` -- "${filePath}"` : "";
        const fallbackRes = await execAsync(`git diff${target}`, { cwd: repoPath });
        return fallbackRes.stdout;
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        return `Failed to compute git diff: ${msg}`;
      }
    }
  }

  /**
   * List all local branches, marking the checked-out one.
   *
   * A folder without Git is a valid project state. It returns an explicit
   * `isGitRepository: false` response instead of being indistinguishable from
   * an empty branch list or a failed request.
   */
  async listBranches(repoPath: string): Promise<GitBranchesResponse> {
    try {
      const repositoryCheck = await execAsync("git rev-parse --is-inside-work-tree", {
        cwd: repoPath,
      });
      if (repositoryCheck.stdout.trim() !== "true") {
        return { branches: [], isGitRepository: false };
      }

      const { stdout } = await execAsync('git branch --format="%(refname:short)"', {
        cwd: repoPath,
      });
      const branches = stdout
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean);
      let current = "";
      try {
        const currentRes = await execAsync("git rev-parse --abbrev-ref HEAD", {
          cwd: repoPath,
        });
        current = currentRes.stdout.trim();
      } catch {
        // Empty repository or detached HEAD: no current local branch.
      }
      return {
        branches: branches.map((name) => ({ name, current: name === current })),
        isGitRepository: true,
      };
    } catch (error) {
      if (isNotGitRepositoryError(error)) {
        return { branches: [], isGitRepository: false };
      }
      throw error;
    }
  }

  /**
   * Check out an existing local branch.
   */
  async checkoutBranch(repoPath: string, branch: string): Promise<void> {
    await execAsync(`git switch "${branch}"`, { cwd: repoPath });
  }
}
