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
  // Session ids are globally unique; keeping projectId in config (rather than
  // the node id) lets an existing chat move projects without duplicating its tab.
  return `chat:${sessionId}`;
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
