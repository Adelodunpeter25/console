/**
 * Filesystem & File Browser Service.
 * Implements business logic for browsing system directories, reading/writing files, creating/deleting folders.
 */
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { listDirTool, readFileTool, writeFileTool } from "@/agent/src/tools/index.js";
import {
  IMAGE_MAX_BYTES,
  MAX_FILE_PREVIEW_BYTES,
  formatBytes,
  getFilePreviewBlock,
  imageMimeForExtension,
  isLockFileName,
  isPreviewableImageName,
  isSvgFileName,
} from "@console/types";
import type { FilePreviewBlockedCode } from "@console/types";
import type { FsTreeEntry } from "@console/types";
import { isPathIgnored } from "@/api/src/utils/ignored.js";
import { isHiddenName, safeFileSize, toSortedEntries } from "@/agent/src/tools/fs-common.js";

/**
 * Raised when a file may not be returned as a text preview (lockfile, binary,
 * or over the size cap). Routes translate this into a structured HTTP error.
 */
export class FilePreviewBlockedError extends Error {
  readonly code: FilePreviewBlockedCode;
  readonly status: 413 | 415;
  /** Extra structured fields for the JSON error body. */
  readonly detail?: Record<string, number>;

  constructor(
    code: FilePreviewBlockedCode,
    message: string,
    options?: { status?: 413 | 415; detail?: Record<string, number> },
  ) {
    super(message);
    this.name = "FilePreviewBlockedError";
    this.code = code;
    this.status = options?.status ?? 415;
    this.detail = options?.detail;
  }
}

/** Bytes inspected for NUL bytes before trusting that a file is text. */
const BINARY_SNIFF_BYTES = 8192;

/** Weak ETag from size + mtime so repeated previews get 304s. */
export function buildFileETag(sizeBytes: number, mtimeMs: number): string {
  return `W/"${sizeBytes.toString(36)}-${Math.round(mtimeMs).toString(36)}"`;
}

/**
 * Slice already-decoded text to [startLine, endLine] (1-based, inclusive)
 * with a single scan — avoids splitting the whole file into an array.
 * Matches the previous split/slice/join semantics exactly (including
 * trailing-newline handling).
 */
function sliceLines(text: string, startLine?: number, endLine?: number): string {
  if (startLine === undefined && endLine === undefined) return text;
  // Line count as split("\n").length without materialising the array.
  let count = 1;
  for (let i = 0; i < text.length; i++) if (text.charCodeAt(i) === 10) count++;
  const start0 = Math.max(1, startLine ?? 1) - 1;
  const endExcl = Math.min(endLine ?? count, count);
  if (start0 >= count || endExcl <= start0) return "";
  // Locate the offset where line `idx` (0-based) starts.
  const offsetOf = (idx: number): number => {
    if (idx <= 0) return 0;
    let line = 0;
    for (let i = 0; i < text.length; i++) {
      if (text.charCodeAt(i) === 10) {
        line++;
        if (line === idx) return i + 1;
      }
    }
    return text.length;
  };
  const sliceStart = offsetOf(start0);
  if (endExcl >= count) return text.slice(sliceStart);
  return text.slice(sliceStart, offsetOf(endExcl) - 1);
}

export class FsService {
  /**
   * Browse a system directory for the mobile/desktop file picker UI.
   * Defaults to user's home directory ($HOME) when no path is provided.
   */
  async browseDirectory(
    targetPath?: string,
    showHidden = false,
  ): Promise<{ currentPath: string; parentPath: string | null; entries: FsTreeEntry[] }> {
    const home = os.homedir();
    const resolvedPath = targetPath ? path.resolve(targetPath) : home;
    const parentPath =
      path.dirname(resolvedPath) !== resolvedPath ? path.dirname(resolvedPath) : null;

    const dirEntries = await fs.readdir(resolvedPath, { withFileTypes: true });
    const entries = await toSortedEntries(resolvedPath, dirEntries, { showHidden });

    return {
      currentPath: resolvedPath,
      parentPath,
      entries,
    };
  }

  /**
   * Get formatted directory tree for a project path.
   */
  async getDirectoryTree(targetPath: string, maxDepth = 3, showHidden = false): Promise<string> {
    const result = (await listDirTool.execute(
      listDirTool.inputSchema.parse({
        path: targetPath,
        maxDepth,
        recursive: true,
        showHidden,
      }),
    )) as { content: Array<{ text: string }> };

    return result.content[0]?.text ?? "";
  }

  /**
   * List all file and directory entries under a project path recursively.
   * Returns a flat list (like T3's ProjectEntry[]) for building a client-side
   * file tree. Used by the mobile file browser for search and full-tree views.
   *
   * Parallelised: stats for a directory resolve together and subdirectories
   * are walked concurrently (bounded), instead of one stat + one awaited
   * recursion at a time. `withSizes: false` skips all stats for the fastest
   * tree-only scan; `maxEntries` caps runaway trees.
   */
  async listAllEntries(
    targetPath: string,
    maxDepth = 25,
    showHidden = false,
    options?: { withSizes?: boolean; maxEntries?: number },
  ): Promise<FsTreeEntry[]> {
    const resolvedRoot = path.resolve(targetPath);
    const withSizes = options?.withSizes ?? true;
    const maxEntries = options?.maxEntries ?? 30000;
    const counter = { count: 0 };
    const truncated = { value: false };

    const walk = async (currentPath: string, depth: number): Promise<FsTreeEntry[]> => {
      if (depth > maxDepth || truncated.value) return [];
      let dirEntries: import("node:fs").Dirent[];
      try {
        dirEntries = await fs.readdir(currentPath, { withFileTypes: true });
      } catch {
        return [];
      }
      const visible = dirEntries.filter((e) => showHidden || !isHiddenName(e.name));
      // Directories first, then files, alphabetically (numeric-aware).
      visible.sort((a, b) => {
        if (a.isDirectory() !== b.isDirectory()) return a.isDirectory() ? -1 : 1;
        return a.name.localeCompare(b.name, undefined, { numeric: true });
      });

      // Stats for the whole directory resolve together, not one by one.
      const sizes = withSizes
        ? await Promise.all(
            visible.map((entry) => {
              if (!showHidden && isPathIgnored(entry.name)) return Promise.resolve(undefined);
              const entryPath = path.join(currentPath, entry.name);
              return safeFileSize(entryPath, entry.isDirectory());
            }),
          )
        : visible.map(() => undefined);

      // Collect subdirectories to walk concurrently (bounded), then
      // interleave children DFS-style to preserve the previous ordering:
      // each directory immediately followed by its subtree.
      const subdirPaths: string[] = [];
      for (const entry of visible) {
        if (!showHidden && isPathIgnored(entry.name)) continue;
        if (entry.isDirectory()) subdirPaths.push(path.join(currentPath, entry.name));
      }
      const childrenByPath = new Map<string, FsTreeEntry[]>();
      const CONCURRENCY = 8;
      for (let i = 0; i < subdirPaths.length; i += CONCURRENCY) {
        if (truncated.value) break;
        const batch = subdirPaths.slice(i, i + CONCURRENCY);
        const children = await Promise.all(batch.map((p) => walk(p, depth + 1)));
        batch.forEach((p, j) => childrenByPath.set(p, children[j]!));
      }

      const local: FsTreeEntry[] = [];
      for (let i = 0; i < visible.length; i++) {
        if (truncated.value || counter.count >= maxEntries) {
          truncated.value = true;
          break;
        }
        const entry = visible[i]!;
        if (!showHidden && isPathIgnored(entry.name)) {
          const dirPath = path.join(currentPath, entry.name);
          local.push({ name: entry.name, path: dirPath, isDir: true });
          counter.count++;
          continue;
        }

        const entryPath = path.join(currentPath, entry.name);
        const isDir = entry.isDirectory();
        local.push({ name: entry.name, path: entryPath, isDir, size: sizes[i] });
        counter.count++;
        if (isDir) {
          // Children already counted inside the recursive walk.
          local.push(...(childrenByPath.get(entryPath) ?? []));
        }
      }
      return local;
    };

    return walk(resolvedRoot, 1);
  }

  /**
   * Read file content with line range support.
   *
   * Enforces the file-preview gate (docs/notes/file-preview-gating.md):
   * lockfiles, binary files (extension or NUL-byte sniff), and files over
   * MAX_FILE_PREVIEW_BYTES are rejected with FilePreviewBlockedError before
   * the body is read. Single open/read: stat + sniff + body share one
   * file handle instead of stat + open/sniff/close + reopen.
   */
  async readFileContentWithMeta(
    filePath: string,
    startLine?: number,
    endLine?: number,
  ): Promise<{ content: string; sizeBytes: number; mtimeMs: number }> {
    const fileName = path.basename(filePath);

    const nameBlock = getFilePreviewBlock(fileName);
    if (nameBlock?.kind === "LOCKFILE_BLOCKED") {
      throw new FilePreviewBlockedError("LOCKFILE_BLOCKED", nameBlock.message);
    }
    if (nameBlock?.kind === "BINARY_FILE") {
      throw new FilePreviewBlockedError("BINARY_FILE", nameBlock.message);
    }

    const handle = await fs.open(filePath, "r");
    try {
      const stat = await handle.stat();
      if (!stat.isFile()) {
        throw new Error(`${filePath} is not a regular file.`);
      }
      if (stat.size > MAX_FILE_PREVIEW_BYTES) {
        throw new FilePreviewBlockedError(
          "FILE_TOO_LARGE",
          `"${fileName}" is ${formatBytes(stat.size)} — previews are capped at ${formatBytes(MAX_FILE_PREVIEW_BYTES)}.`,
          { status: 413, detail: { sizeBytes: stat.size, maxBytes: MAX_FILE_PREVIEW_BYTES } },
        );
      }

      // Single read: sniff the head of the same buffer for NUL bytes.
      // Extensions lie, NUL bytes don't — catches binaries whose extension
      // isn't on the denylist.
      const buffer = Buffer.alloc(stat.size);
      await handle.read(buffer, 0, stat.size, 0);
      const headLen = Math.min(BINARY_SNIFF_BYTES, buffer.length);
      for (let i = 0; i < headLen; i++) {
        if (buffer[i] === 0) {
          throw new FilePreviewBlockedError(
            "BINARY_FILE",
            `"${fileName}" doesn't look like a text file.`,
          );
        }
      }

      const raw = buffer.toString("utf-8");
      return { content: sliceLines(raw, startLine, endLine), sizeBytes: stat.size, mtimeMs: stat.mtimeMs };
    } finally {
      await handle.close();
    }
  }

  async readFileContent(filePath: string, startLine?: number, endLine?: number): Promise<string> {
    return (await this.readFileContentWithMeta(filePath, startLine, endLine)).content;
  }

  /**
   * Validate an image/SVG preview request and return its metadata.
   * Lets the route stream `Bun.file()` (zero-copy sendfile) instead of
   * buffering the whole file through Node.
   */
  async getImageMeta(
    filePath: string,
  ): Promise<{ mimeType: string; sizeBytes: number; mtimeMs: number }> {
    const fileName = path.basename(filePath);

    if (isLockFileName(fileName)) {
      throw new FilePreviewBlockedError(
        "LOCKFILE_BLOCKED",
        `"${fileName}" is a generated lockfile — open it on your machine instead.`,
      );
    }

    if (!isPreviewableImageName(fileName) && !isSvgFileName(fileName)) {
      throw new FilePreviewBlockedError(
        "BINARY_FILE",
        `"${fileName}" is not a supported image format.`,
      );
    }

    const stat = await fs.stat(filePath);
    if (!stat.isFile()) {
      throw new Error(`${filePath} is not a regular file.`);
    }

    if (stat.size > IMAGE_MAX_BYTES) {
      throw new FilePreviewBlockedError(
        "FILE_TOO_LARGE",
        `"${fileName}" is ${formatBytes(stat.size)} — image previews are capped at ${formatBytes(IMAGE_MAX_BYTES)}.`,
        { status: 413, detail: { sizeBytes: stat.size, maxBytes: IMAGE_MAX_BYTES } },
      );
    }

    const mimeType = imageMimeForExtension(path.extname(filePath)) ?? "application/octet-stream";
    return { mimeType, sizeBytes: stat.size, mtimeMs: stat.mtimeMs };
  }

  /**
   * Read raw image / SVG bytes for binary preview.
   *
   * Only allows supported raster image extensions and SVG.
   * Lockfiles and non-image binaries/text are blocked.
   * Enforces IMAGE_MAX_BYTES.
   */
  async readFileBytes(
    filePath: string,
  ): Promise<{ bytes: Buffer; mimeType: string; sizeBytes: number; mtimeMs: number }> {
    const { mimeType, sizeBytes, mtimeMs } = await this.getImageMeta(filePath);
    const bytes = await fs.readFile(filePath);

    return { bytes, mimeType, sizeBytes, mtimeMs };
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
