export type WorkspaceTabType = "chat" | "terminal" | "file" | "diff";

export interface ChatTabConfig {
  type: "chat";
  projectId: string;
  sessionId: string;
  title?: string;
}

export interface TerminalTabConfig {
  type: "terminal";
  projectId: string;
  terminalId: string;
  title?: string;
}

export interface FileTabConfig {
  type: "file";
  projectId: string;
  path: string;
  title?: string;
}

export interface DiffTabConfig {
  type: "diff";
  projectId: string;
  path: string;
  title?: string;
  baseRevision?: string;
}

export type WorkspaceTabConfig =
  | ChatTabConfig
  | TerminalTabConfig
  | FileTabConfig
  | DiffTabConfig;

export type OpenChatTabInput = ChatTabConfig & { title?: string };
export type OpenTerminalTabInput = TerminalTabConfig & { title?: string };
export type OpenFileTabInput = FileTabConfig & { title?: string };

export type SplitDirection = "horizontal" | "vertical";

export interface LeafPaneNode {
  type: "leaf";
  id: string;
  tabs: WorkspaceTabConfig[];
  activeTabId: string | null;
}

export interface SplitPaneNode {
  type: "split";
  id: string;
  direction: SplitDirection;
  sizes: [number, number];
  children: [WorkspaceNode, WorkspaceNode];
}

export type WorkspaceNode = LeafPaneNode | SplitPaneNode;

export function basename(filePath: string): string {
  const normalized = filePath.replace(/\\/g, "/");
  const lastSlash = normalized.lastIndexOf("/");
  return lastSlash >= 0 ? normalized.slice(lastSlash + 1) : normalized;
}

export function getTabId(config: WorkspaceTabConfig): string {
  switch (config.type) {
    case "chat":
      return `chat:${config.sessionId}`;
    case "terminal":
      return `terminal:${config.terminalId}`;
    case "file":
      return `file:${config.path}`;
    case "diff":
      return `diff:${config.path}`;
  }
}

export function getTabTitle(config: WorkspaceTabConfig): string {
  if (config.title) return config.title;
  switch (config.type) {
    case "chat":
      return "Untitled Chat";
    case "terminal":
      return "Terminal";
    case "file":
      return basename(config.path);
    case "diff":
      return `Diff: ${basename(config.path)}`;
  }
}
