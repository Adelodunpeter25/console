import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { pathString } from "@/agent/src/service/tool-input.js";

// Reuse the same deny-list as the file watcher / API utils to avoid walking
// massive ignored trees (node_modules, .git, dist, etc.). Keep local to avoid
// cross-layer import coupling; must stay in sync with api/src/utils/ignored.ts.
const IGNORED_SEGMENTS = new Set([
  "node_modules",
  ".git",
  ".hg",
  ".svn",
  "dist",
  "build",
  ".next",
  ".turbo",
  ".vite",
  ".cache",
  "coverage",
  ".DS_Store",
  "thumbs.db",
  ".gemini",
  "target",
  "tmp",
]);

function isIgnoredName(name: string): boolean {
  return IGNORED_SEGMENTS.has(name.toLowerCase());
}

const inputSchema = z.object({
  path: pathString('Required filesystem directory path to list. Use "." for the current project directory.'),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for relative paths. Defaults to process.cwd()."),
  recursive: z.boolean().optional().default(false).describe("Recursively list subdirectories"),
  maxDepth: z
    .number()
    .int()
    .min(1)
    .max(10)
    .optional()
    .default(3)
    .describe("Maximum depth for recursive listing (1–10)"),
  showHidden: z
    .boolean()
    .optional()
    .default(false)
    .describe("Include hidden files and directories (starting with '.')"),
  maxEntries: z
    .number()
    .int()
    .min(100)
    .max(10000)
    .optional()
    .default(3000)
    .describe("Maximum total entries to return (truncates with notice). Default 3000."),
});

type Input = z.infer<typeof inputSchema>;

interface TreeEntry {
  name: string;
  isDir: boolean;
  size?: number;
  children?: TreeEntry[];
}

async function buildTree(
  dirPath: string,
  currentDepth: number,
  maxDepth: number,
  recursive: boolean,
  showHidden: boolean,
  signal?: AbortSignal,
  counter?: { count: number; max: number; truncated: boolean },
): Promise<TreeEntry[]> {
  if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
  if (counter && counter.count >= counter.max) {
    counter.truncated = true;
    return [];
  }

  let entries;
  try {
    entries = await fs.readdir(dirPath, { withFileTypes: true });
  } catch (err: unknown) {
    const error = err as NodeJS.ErrnoException;
    throw new Error(`Cannot read directory "${dirPath}": ${error.message}`);
  }

  let filtered = showHidden ? entries : entries.filter((e) => !e.name.startsWith("."));
  // Skip massive ignored trees when recursing (node_modules, .git, etc.)
  if (recursive) {
    filtered = filtered.filter((e) => !isIgnoredName(e.name));
  }
  filtered.sort((a, b) => {
    if (a.isDirectory() !== b.isDirectory()) {
      return a.isDirectory() ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });

  // Bounded concurrency: process entries in parallel batches to avoid sequential stat.
  const CONCURRENCY = 32;
  const result: TreeEntry[] = [];

  for (let i = 0; i < filtered.length; i += CONCURRENCY) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    if (counter && counter.count >= counter.max) {
      counter.truncated = true;
      break;
    }
    const batch = filtered.slice(i, i + CONCURRENCY);
    const batchResults = await Promise.all(
      batch.map(async (entry): Promise<TreeEntry | null> => {
        if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
        if (counter && counter.count >= counter.max) {
          counter.truncated = true;
          return null;
        }
        if (entry.isDirectory()) {
          const children =
            recursive && currentDepth < maxDepth
              ? await buildTree(
                  path.join(dirPath, entry.name),
                  currentDepth + 1,
                  maxDepth,
                  recursive,
                  showHidden,
                  signal,
                  counter,
                )
              : undefined;
          if (counter) counter.count++;
          return { name: entry.name, isDir: true, children };
        }
        if (entry.isFile() || entry.isSymbolicLink()) {
          let size: number | undefined;
          try {
            // lstat avoids following symlink loops; for regular files it's same as stat
            const stat = await fs.lstat(path.join(dirPath, entry.name));
            // Only use size for regular files; for symlink dirs we already handled
            if (stat.isFile() || stat.isSymbolicLink()) size = stat.size;
          } catch {
            // ignore stat errors
          }
          if (counter) counter.count++;
          return { name: entry.name, isDir: false, size };
        }
        return null;
      }),
    );
    for (const item of batchResults) {
      if (item) result.push(item);
    }
  }

  return result;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)}GB`;
}

function renderTree(entries: TreeEntry[], prefix = "", _isLast = false): string[] {
  const lines: string[] = [];

  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i]!;
    const isLastEntry = i === entries.length - 1;
    const connector = isLastEntry ? "└── " : "├── ";
    const childPrefix = isLastEntry ? "    " : "│   ";

    if (entry.isDir) {
      lines.push(`${prefix}${connector}${entry.name}/`);
      if (entry.children && entry.children.length > 0) {
        lines.push(...renderTree(entry.children, prefix + childPrefix));
      } else if (entry.children) {
        lines.push(`${prefix}${childPrefix}(empty)`);
      }
    } else {
      const sizeStr = entry.size !== undefined ? `  [${formatSize(entry.size)}]` : "";
      lines.push(`${prefix}${connector}${entry.name}${sizeStr}`);
    }
  }

  return lines;
}

export const listDirTool: AgentTool<typeof inputSchema> = {
  name: "listDir",
  description: `List files and directories at a given path.
Returns a tree-like structure showing names, sizes, and nesting.
Use this to understand project structure before reading specific files.
Use recursive: true to explore subdirectories (up to maxDepth levels deep).
Skips ignored trees (node_modules, .git, dist, etc.) when recursive, truncates at maxEntries.`,
  inputSchema,
  execute: async (args: Input, signal?: AbortSignal): Promise<unknown> => {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const dirPath = path.resolve(args.cwd ?? process.cwd(), args.path);

    // Verify it's actually a directory
    try {
      const stat = await fs.stat(dirPath);
      if (!stat.isDirectory()) {
        return {
          content: [
            {
              type: "text",
              text: `Error: "${dirPath}" is not a directory. Use readFile to read files.`,
            },
          ],
          isError: true,
        };
      }
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      if (error.code === "ENOENT") {
        return {
          content: [{ type: "text", text: `Error: Directory not found: ${dirPath}` }],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error: ${error.message}` }],
        isError: true,
      };
    }

    const counter = { count: 0, max: args.maxEntries, truncated: false };
    let tree: TreeEntry[];
    try {
      tree = await buildTree(dirPath, 1, args.maxDepth, args.recursive, args.showHidden, signal, counter);
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === "AbortError") throw err;
      return {
        content: [{ type: "text", text: String(err) }],
        isError: true,
      };
    }

    const lines = renderTree(tree);
    const depthNote = args.recursive
      ? ` (recursive, max depth ${args.maxDepth})`
      : " (top-level only — use recursive: true to expand)";

    const header = `Directory: ${dirPath}${depthNote}\n`;
    let body = lines.length > 0 ? lines.join("\n") : "(empty directory)";
    if (counter.truncated) {
      body += `\n\n… truncated at ${counter.max} entries (use maxEntries to see more or narrow path). Ignored: node_modules/.git/dist etc.`;
    }

    return {
      content: [{ type: "text", text: header + body }],
    };
  },
};
