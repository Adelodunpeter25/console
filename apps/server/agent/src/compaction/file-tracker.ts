import type { AgentMessage } from "@/agent/src/types/index.js";

export interface FileOperations {
  read: Set<string>;
  written: Set<string>;
  edited: Set<string>;
}

export function createFileOps(): FileOperations {
  return {
    read: new Set(),
    written: new Set(),
    edited: new Set(),
  };
}

/** Strip line range selectors like `:1-50`, `:50`, `:raw` from paths. */
export function stripReadSelector(path: string): string {
  const colon = path.lastIndexOf(":");
  if (colon <= 0) return path;
  const candidate = path.slice(colon + 1);
  if (/^(?:L?\d+(?:(?:[-+]|\.\.)L?\d+|-|\.\.)?|raw|conflicts)$/i.test(candidate)) {
    return path.slice(0, colon);
  }
  return path;
}

/** Check if path has a url scheme like http:// or artifact:// */
export function isUrlScheme(path: string): boolean {
  return /^[a-z][a-z0-9+.-]*:\/\//i.test(path);
}

/** Extract clean file paths touched by tool calls in messages. */
export function extractFileOps(messages: AgentMessage[]): FileOperations {
  const ops = createFileOps();

  for (const msg of messages) {
    if (msg.role !== "assistant") continue;

    for (const part of msg.content) {
      if (part.type !== "toolCall") continue;
      const call = part.call;
      const args = (call.arguments ?? {}) as Record<string, unknown>;
      const rawPath =
        (args.path as string) ||
        (args.filePath as string) ||
        (args.targetFile as string) ||
        (args.TargetFile as string) ||
        (args.absolutePath as string) ||
        (args.AbsolutePath as string);

      if (!rawPath || typeof rawPath !== "string" || isUrlScheme(rawPath)) {
        continue;
      }

      const cleanPath = stripReadSelector(rawPath.trim());
      if (!cleanPath) continue;

      const name = call.name.toLowerCase();
      if (name.includes("read")) {
        ops.read.add(cleanPath);
      } else if (name.includes("write")) {
        ops.written.add(cleanPath);
      } else if (name.includes("edit") || name.includes("replace") || name.includes("patch")) {
        ops.edited.add(cleanPath);
      }
    }
  }

  return ops;
}

const MAX_FILES_IN_TREE = 25;

/** Format cumulative file operations into a clean prefix tree with Read/Write/RW markers. */
export function formatFileTree(ops: FileOperations): string {
  const readSet = ops.read;
  const modifiedSet = new Set<string>([...ops.written, ...ops.edited]);

  if (readSet.size === 0 && modifiedSet.size === 0) {
    return "";
  }

  const mode = new Map<string, "Read" | "Write" | "RW">();
  for (const file of readSet) {
    mode.set(file, "Read");
  }
  for (const file of modifiedSet) {
    mode.set(file, readSet.has(file) ? "RW" : "Write");
  }

  const allFiles = [...mode.keys()].sort();
  const displayFiles = allFiles.slice(0, MAX_FILES_IN_TREE);

  // Group by directory
  const dirGroups = new Map<string, string[]>();
  for (const file of displayFiles) {
    const lastSlash = file.lastIndexOf("/");
    const dir = lastSlash > 0 ? file.slice(0, lastSlash + 1) : "";
    const name = lastSlash > 0 ? file.slice(lastSlash + 1) : file;
    const list = dirGroups.get(dir) || [];
    list.push(`${name} (${mode.get(file)})`);
    dirGroups.set(dir, list);
  }

  const lines: string[] = [];
  for (const [dir, files] of dirGroups.entries()) {
    if (dir) {
      lines.push(`# ${dir}`);
    }
    for (const f of files) {
      lines.push(f);
    }
  }

  if (allFiles.length > MAX_FILES_IN_TREE) {
    lines.push(`[…${allFiles.length - MAX_FILES_IN_TREE} files elided…]`);
  }

  return `<files>\n${lines.join("\n")}\n</files>`;
}
