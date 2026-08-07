import type { IJsonTabNode } from "flexlayout-react";

export const MAIN_WORKSPACE_TABSET_ID = "workspace-main";

/** Every view that can eventually live in the center workspace. */
export type WorkspaceTabType = "chat" | "terminal" | "file" | "diff";

export interface ChatTabConfig {
  type: "chat";
  projectId: string;
  sessionId: string;
}

export interface TerminalTabConfig {
  type: "terminal";
  projectId: string;
  terminalId: string;
}

export interface FileTabConfig {
  type: "file";
  projectId: string;
  path: string;
}

export interface DiffTabConfig {
  type: "diff";
  projectId: string;
  path: string;
  baseRevision?: string;
}

export type WorkspaceTabConfig =
  | ChatTabConfig
  | TerminalTabConfig
  | FileTabConfig
  | DiffTabConfig;

export interface OpenChatTabInput extends ChatTabConfig {
  title: string;
}

export interface OpenFileTabInput extends FileTabConfig {
  title?: string;
}

export function isWorkspaceTabConfig(value: unknown): value is WorkspaceTabConfig {
  return Boolean(
    value &&
      typeof value === "object" &&
      "type" in value &&
      "projectId" in value &&
      typeof value.projectId === "string",
  );
}

export function chatTabId(_projectId: string, sessionId: string): string {
  return `chat:${sessionId}`;
}

export function fileTabId(_projectId: string, filePath: string): string {
  return `file:${filePath}`;
}

export function createChatTab({ projectId, sessionId, title }: OpenChatTabInput): IJsonTabNode {
  return {
    type: "tab",
    id: chatTabId(projectId, sessionId),
    name: title || "Untitled Chat",
    component: "chat",
    config: { type: "chat", projectId, sessionId } satisfies ChatTabConfig,
    enableClose: true,
    enableRename: false,
  };
}

export function createFileTab({ projectId, path: filePath, title }: OpenFileTabInput): IJsonTabNode {
  const tabTitle = title ?? basename(filePath);
  return {
    type: "tab",
    id: fileTabId(projectId, filePath),
    name: tabTitle,
    component: "file",
    config: { type: "file", projectId, path: filePath } satisfies FileTabConfig,
    enableClose: true,
    enableRename: false,
  };
}

export function basename(filePath: string): string {
  const normalized = filePath.replace(/\\/g, "/");
  const idx = normalized.lastIndexOf("/");
  return idx >= 0 ? normalized.slice(idx + 1) : normalized;
}
