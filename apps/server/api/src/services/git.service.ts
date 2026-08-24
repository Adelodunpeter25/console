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

      const files: Array<{ path: string; status: GitFileStatus; staged: boolean }> = [];

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

        files.push({
          path: absolutePath,
          status,
          staged,
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
