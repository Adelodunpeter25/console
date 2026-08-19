/**
 * Filesystem & File Browser Service.
 * Implements business logic for browsing system directories, reading/writing files, creating/deleting folders.
 */
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { listDirTool, readFileTool, writeFileTool } from "../../../agent/src/tools/index.js";
import type { FsTreeEntry, GitFileStatus } from "@console/types";
import { GitService } from "./git.service.js";

export class FsService {
  /**
   * Browse a system directory for the mobile/desktop file picker UI.
   * Defaults to user's home directory ($HOME) when no path is provided.
   */
  async browseDirectory(
    targetPath?: string,
  ): Promise<{ currentPath: string; parentPath: string | null; entries: FsTreeEntry[] }> {
    const home = os.homedir();
    const resolvedPath = targetPath ? path.resolve(targetPath) : home;
    const parentPath =
      path.dirname(resolvedPath) !== resolvedPath ? path.dirname(resolvedPath) : null;

    const dirEntries = await fs.readdir(resolvedPath, { withFileTypes: true });
    const entries: FsTreeEntry[] = [];

    const gitStatusMap = new Map<string, GitFileStatus>();
    try {
      const gitService = new GitService();
      const gitStatusSummary = await gitService.getGitStatus(resolvedPath);
      for (const f of gitStatusSummary.files) {
        gitStatusMap.set(f.path, f.status);
      }
    } catch {
      // Ignored if target directory is not a git repo or has no git
    }

    for (const entry of dirEntries) {
      // Ignore hidden files and directories (starting with '.') by default
      if (entry.name.startsWith(".")) {
        continue;
      }

      const entryPath = path.join(resolvedPath, entry.name);
      let size: number | undefined;

      if (!entry.isDirectory()) {
        try {
          const stat = await fs.stat(entryPath);
          size = stat.size;
        } catch {
          // Ignored if stat fails
        }
      }

      entries.push({
        name: entry.name,
        path: entryPath,
        isDir: entry.isDirectory(),
        size,
        gitStatus: gitStatusMap.get(entryPath),
      });
    }

    // Sort directories first, then files alphabetically
    entries.sort((a, b) => {
      if (a.isDir && !b.isDir) return -1;
      if (!a.isDir && b.isDir) return 1;
      return a.name.localeCompare(b.name);
    });

    return {
      currentPath: resolvedPath,
      path: resolvedPath,
      parentPath,
      entries,
    };
  }

  /**
   * Get formatted directory tree for a project path.
   */
  async getDirectoryTree(targetPath: string, maxDepth = 3): Promise<string> {
    const result = (await listDirTool.execute(
      listDirTool.inputSchema.parse({
        path: targetPath,
        maxDepth,
        recursive: true,
      }),
    )) as { content: Array<{ text: string }> };

    return result.content[0]?.text ?? "";
  }

  /**
   * Read file content with line range support.
   */
  async readFileContent(filePath: string, startLine?: number, endLine?: number): Promise<string> {
    const raw = await fs.readFile(filePath, "utf-8");
    if (startLine === undefined && endLine === undefined) {
      return raw;
    }
    const lines = raw.split("\n");
    const start = startLine ? Math.max(1, startLine) - 1 : 0;
    const end = endLine ? Math.min(lines.length, endLine) : lines.length;
    return lines.slice(start, end).join("\n");
  }

  /**
   * Write or overwrite file content.
   */
  async writeFileContent(filePath: string, content: string): Promise<string> {
    await fs.mkdir(path.dirname(filePath), { recursive: true });
    await fs.writeFile(filePath, content, "utf-8");
    return "File written successfully.";
  }

  /**
   * Delete a file from disk.
   */
  async deleteFile(filePath: string): Promise<boolean> {
    await fs.unlink(filePath);
    return true;
  }

  /**
   * Create a new directory.
   */
  async createDirectory(dirPath: string): Promise<boolean> {
    await fs.mkdir(dirPath, { recursive: true });
    return true;
  }

  /**
   * Delete a directory recursively.
   */
  async deleteDirectory(dirPath: string): Promise<boolean> {
    await fs.rm(dirPath, { recursive: true, force: true });
    return true;
  }
}
