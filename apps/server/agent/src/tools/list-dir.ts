import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { pathString } from "@/agent/src/service/tool-input.js";

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
): Promise<TreeEntry[]> {
  let entries;
  try {
    entries = await fs.readdir(dirPath, { withFileTypes: true });
  } catch (err: unknown) {
    const error = err as NodeJS.ErrnoException;
    throw new Error(`Cannot read directory "${dirPath}": ${error.message}`);
  }

  const filtered = showHidden ? entries : entries.filter((e) => !e.name.startsWith("."));
  filtered.sort((a, b) => {
    // Directories first, then files, both alphabetical
    if (a.isDirectory() !== b.isDirectory()) {
      return a.isDirectory() ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });

  const result: TreeEntry[] = [];
  for (const entry of filtered) {
    if (entry.isDirectory()) {
      const children =
        recursive && currentDepth < maxDepth
          ? await buildTree(
              path.join(dirPath, entry.name),
              currentDepth + 1,
              maxDepth,
              recursive,
              showHidden,
            )
          : undefined;
      result.push({ name: entry.name, isDir: true, children });
    } else if (entry.isFile() || entry.isSymbolicLink()) {
      let size: number | undefined;
      try {
        const stat = await fs.stat(path.join(dirPath, entry.name));
        size = stat.size;
      } catch {
        // ignore stat errors
      }
      result.push({ name: entry.name, isDir: false, size });
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
Use recursive: true to explore subdirectories (up to maxDepth levels deep).`,
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
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

    let tree: TreeEntry[];
    try {
      tree = await buildTree(dirPath, 1, args.maxDepth, args.recursive, args.showHidden);
    } catch (err: unknown) {
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
    const body = lines.length > 0 ? lines.join("\n") : "(empty directory)";

    return {
      content: [{ type: "text", text: header + body }],
    };
  },
};
