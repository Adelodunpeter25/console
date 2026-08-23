import type { FsTreeEntry } from "@console/types";

export interface FileTreeNode {
  readonly path: string; // relative to project root (e.g. "src/index.ts")
  readonly absolutePath: string; // original absolute path
  readonly name: string;
  readonly isDir: boolean;
  readonly children: ReadonlyArray<FileTreeNode>;
  readonly searchSegments: ReadonlyArray<string>;
  readonly searchWords: ReadonlyArray<string>;
  readonly gitStatus?: string;
  readonly size?: number;
}

export interface VisibleFileTreeNode {
  readonly node: FileTreeNode;
  readonly depth: number;
}

interface MutableFileTreeNode {
  path: string;
  absolutePath: string;
  name: string;
  isDir: boolean;
  children: Map<string, MutableFileTreeNode>;
  gitStatus?: string;
  size?: number;
}

function createMutableNode(
  path: string,
  absolutePath: string,
  name: string,
  isDir: boolean,
  gitStatus?: string,
  size?: number,
): MutableFileTreeNode {
  return {
    path,
    absolutePath,
    name,
    isDir,
    children: new Map(),
    gitStatus,
    size,
  };
}

function splitSearchWords(value: string): ReadonlyArray<string> {
  return value
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map((word) => word.toLowerCase());
}

function buildNodeSearchTerms(path: string): {
  readonly segments: ReadonlyArray<string>;
  readonly words: ReadonlyArray<string>;
} {
  const segments: string[] = [];
  const words: string[] = [];

  for (const segment of path.split("/")) {
    if (!segment) continue;
    segments.push(segment.toLowerCase());
    words.push(...splitSearchWords(segment));
  }

  return { segments, words };
}

function freezeNode(node: MutableFileTreeNode): FileTreeNode {
  const searchTerms = buildNodeSearchTerms(node.path);
  return {
    path: node.path,
    absolutePath: node.absolutePath,
    name: node.name,
    isDir: node.isDir,
    children: [...node.children.values()].sort(compareNodes).map(freezeNode),
    searchSegments: searchTerms.segments,
    searchWords: searchTerms.words,
    gitStatus: node.gitStatus,
    size: node.size,
  };
}

function compareNodes(
  left: Pick<FileTreeNode, "isDir" | "name">,
  right: Pick<FileTreeNode, "isDir" | "name">,
): number {
  if (left.isDir !== right.isDir) {
    return left.isDir ? -1 : 1;
  }
  return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: "base" });
}

function toRelativePath(absolutePath: string, projectRoot: string): string {
  const normalizedRoot = projectRoot.replace(/\/$/, "");
  const normalizedPath = absolutePath.replace(/\/$/, "");
  if (normalizedPath === normalizedRoot) return "";
  if (normalizedPath.startsWith(normalizedRoot + "/")) {
    return normalizedPath.slice(normalizedRoot.length + 1);
  }
  // Fallback: strip leading slash and treat as relative
  return normalizedPath.replace(/^\//, "");
}

/**
 * Build a hierarchical file tree from a flat FsTreeEntry list.
 * Adapted from t3code's fileTree.ts but works with console's FsTreeEntry
 * (isDir + absolute paths) and normalizes to project-relative paths.
 */
export function buildFileTree(
  entries: ReadonlyArray<FsTreeEntry>,
  projectRoot: string,
): ReadonlyArray<FileTreeNode> {
  const root = createMutableNode("", projectRoot, "", true);

  for (const entry of entries) {
    const relativePath = toRelativePath(entry.path, projectRoot);
    if (!relativePath) continue;

    const parts = relativePath.split("/").filter(Boolean);
    if (parts.length === 0) continue;

    let current = root;
    for (let index = 0; index < parts.length; index += 1) {
      const part = parts[index];
      if (!part) continue;

      const isLeaf = index === parts.length - 1;
      const isDir = isLeaf ? entry.isDir : true;
      const currentRelativePath = parts.slice(0, index + 1).join("/");
      const currentAbsolutePath = isLeaf
        ? entry.path
        : `${projectRoot.replace(/\/$/, "")}/${currentRelativePath}`;

      let child = current.children.get(part);
      if (!child) {
        child = createMutableNode(
          currentRelativePath,
          currentAbsolutePath,
          part,
          isDir,
          isLeaf ? entry.gitStatus : undefined,
          isLeaf ? entry.size : undefined,
        );
        current.children.set(part, child);
      } else if (isLeaf) {
        // Leaf already exists as intermediate dir — update to reflect actual type
        child.isDir = entry.isDir;
        child.gitStatus = entry.gitStatus;
        child.size = entry.size;
        child.absolutePath = entry.path;
      }
      current = child;
    }
  }

  return [...root.children.values()].sort(compareNodes).map(freezeNode);
}

export function countFileNodes(nodes: ReadonlyArray<FileTreeNode>): number {
  let count = 0;
  for (const node of nodes) {
    if (!node.isDir) {
      count += 1;
    } else {
      count += countFileNodes(node.children);
    }
  }
  return count;
}

export function defaultExpandedTreePaths(nodes: ReadonlyArray<FileTreeNode>): ReadonlySet<string> {
  const expanded = new Set<string>();
  for (const node of nodes) {
    if (node.isDir) {
      expanded.add(node.path);
    }
  }
  return expanded;
}

function normalizeSearchQuery(query: string): string {
  return query.trim().toLowerCase();
}

function nodeMatchesSearch(node: FileTreeNode, tokens: ReadonlyArray<string>): boolean {
  return tokens.every(
    (token) =>
      node.searchSegments.some((segment) => segment.includes(token)) ||
      node.searchWords.some((word) => word.includes(token)) ||
      node.path.toLowerCase().includes(token),
  );
}

function flattenNode(
  output: VisibleFileTreeNode[],
  node: FileTreeNode,
  depth: number,
  expanded: ReadonlySet<string>,
  searchTokens: ReadonlyArray<string>,
): boolean {
  const isSearching = searchTokens.length > 0;
  const matches = isSearching && nodeMatchesSearch(node, searchTokens);
  let descendantMatches = false;
  const childOutput: VisibleFileTreeNode[] = [];

  if (node.isDir && (expanded.has(node.path) || isSearching)) {
    for (const child of node.children) {
      if (flattenNode(childOutput, child, depth + 1, expanded, searchTokens)) {
        descendantMatches = true;
      }
    }
  }

  const visible = !isSearching || matches || descendantMatches;
  if (!visible) {
    return false;
  }

  output.push({ node, depth });
  output.push(...childOutput);
  return matches || descendantMatches;
}

export function flattenFileTree(input: {
  readonly nodes: ReadonlyArray<FileTreeNode>;
  readonly expanded: ReadonlySet<string>;
  readonly searchQuery?: string;
}): ReadonlyArray<VisibleFileTreeNode> {
  const output: VisibleFileTreeNode[] = [];
  const normalizedSearch = normalizeSearchQuery(input.searchQuery ?? "");
  const searchTokens = normalizedSearch.split(/[\s/\\._-]+/).filter(Boolean);
  for (const node of input.nodes) {
    flattenNode(output, node, 0, input.expanded, searchTokens);
  }
  return output;
}

export function firstFilePath(nodes: ReadonlyArray<FileTreeNode>): string | null {
  for (const node of nodes) {
    if (!node.isDir) {
      return node.path;
    }
    const child = firstFilePath(node.children);
    if (child !== null) {
      return child;
    }
  }
  return null;
}
