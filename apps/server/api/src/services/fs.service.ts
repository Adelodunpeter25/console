/**
 * Filesystem & File Browser Service.
 * Implements business logic for browsing system directories, reading/writing files, creating/deleting folders.
 */
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { listDirTool, readFileTool, writeFileTool } from "@/agent/src/tools/index.js";
import type { FsTreeEntry } from "@console/types";

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
   * List all file and directory entries under a project path recursively.
   * Returns a flat list (like T3's ProjectEntry[]) for building a client-side
   * file tree. Used by the mobile file browser for search and full-tree views.
   */
  async listAllEntries(
    targetPath: string,
    maxDepth = 6,
    showHidden = false,
  ): Promise<FsTreeEntry[]> {
    const resolvedRoot = path.resolve(targetPath);
    const result: FsTreeEntry[] = [];

    async function walk(currentPath: string, depth: number): Promise<void> {
      if (depth > maxDepth) return;
      let dirEntries: import("node:fs").Dirent[];
      try {
        dirEntries = await fs.readdir(currentPath, { withFileTypes: true });
      } catch {
        return;
      }

      for (const entry of dirEntries) {
        if (!showHidden && entry.name.startsWith(".")) continue;
        // Skip massive ignored dirs for performance (like T3 does via searchRanking)
        if (!showHidden && (entry.name === "node_modules" || entry.name === ".git")) {
          // Still record the directory itself so it appears in the tree
          const dirPath = path.join(currentPath, entry.name);
          result.push({
            name: entry.name,
            path: dirPath,
            isDir: true,
          });
          continue;
        }

        const entryPath = path.join(currentPath, entry.name);
        const isDir = entry.isDirectory();

        let size: number | undefined;
        if (!isDir) {
          try {
            const stat = await fs.stat(entryPath);
            size = stat.size;
          } catch {
            // ignore
          }
        }

        result.push({
          name: entry.name,
          path: entryPath,
          isDir,
          size,
        });

        if (isDir) {
          await walk(entryPath, depth + 1);
        }
      }
    }

    await walk(resolvedRoot, 1);
    return result;
  }

  /**
   * Read file content with line range support.
   */
  async readFileContent(filePath: string, startLine?: number, endLine?: number): Promise<string> {
    const raw = await Bun.file(filePath).text();
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
    await Bun.write(filePath, content);
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
