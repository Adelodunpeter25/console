import type { TerminalRecord } from "../types/terminal";
import type { TerminalTabConfig, WorkspaceNode, WorkspaceTabConfig } from "./types";

export const LAYOUT_VERSION = 1 as const;

export interface PersistedTerminal {
  terminalId: string;
  projectId: string;
  cwd: string;
  title?: string;
  shell?: string;
  cols: number;
  rows: number;
}

export interface LayoutSnapshot {
  version: typeof LAYOUT_VERSION;
  workspace: {
    rootNode: WorkspaceNode;
    activePaneId: string;
  };
  selection: {
    selectedProjectId: string | null;
    selectedSessionId: string | null;
  };
  ui: {
    sidebarOpen: boolean;
    rightSidebarOpen: boolean;
    sidebarWidth: number;
    rightSidebarWidth: number;
    rightSidebarPanelSizes: [number, number];
  };
  dockedTerminals: {
    terminals: TerminalTabConfig[];
    activeTerminalId: string | null;
  };
  terminals: PersistedTerminal[];
}

function collectTerminalIds(node: WorkspaceNode, ids: Set<string>): void {
  if (node.type === "leaf") {
    node.tabs.forEach((tab) => {
      if (tab.type === "terminal") ids.add(tab.terminalId);
    });
    return;
  }
  collectTerminalIds(node.children[0], ids);
  collectTerminalIds(node.children[1], ids);
}

function collectTerminalConfigs(node: WorkspaceNode, configs: Map<string, TerminalTabConfig>): void {
  if (node.type === "leaf") {
    node.tabs.forEach((tab) => {
      if (tab.type === "terminal") configs.set(tab.terminalId, tab);
    });
    return;
  }
  collectTerminalConfigs(node.children[0], configs);
  collectTerminalConfigs(node.children[1], configs);
}

export function createLayoutSnapshot(input: {
  rootNode: WorkspaceNode;
  activePaneId: string;
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  sidebarOpen: boolean;
  rightSidebarOpen: boolean;
  sidebarWidth: number;
  rightSidebarWidth: number;
  rightSidebarPanelSizes: [number, number];
  dockedTerminals: TerminalTabConfig[];
  activeDockedTerminalId: string | null;
  terminalRecords: Record<string, TerminalRecord>;
}): LayoutSnapshot {
  const terminalIds = new Set<string>();
  const terminalConfigs = new Map<string, TerminalTabConfig>();
  collectTerminalIds(input.rootNode, terminalIds);
  collectTerminalConfigs(input.rootNode, terminalConfigs);
  input.dockedTerminals.forEach((terminal) => terminalIds.add(terminal.terminalId));
  input.dockedTerminals.forEach((terminal) => terminalConfigs.set(terminal.terminalId, terminal));

  const terminals = [...terminalIds].flatMap((terminalId) => {
    const record = input.terminalRecords[terminalId];
    if (!record?.cwd) return [];

    const config = terminalConfigs.get(terminalId);
    return [
      {
        terminalId,
        projectId: config?.projectId ?? record.projectId,
        cwd: record.cwd,
        title: config?.title,
        shell: record.shell,
        cols: record.cols,
        rows: record.rows,
      },
    ];
  });

  return {
    version: LAYOUT_VERSION,
    workspace: {
      rootNode: input.rootNode,
      activePaneId: input.activePaneId,
    },
    selection: {
      selectedProjectId: input.selectedProjectId,
      selectedSessionId: input.selectedSessionId,
    },
    ui: {
      sidebarOpen: input.sidebarOpen,
      rightSidebarOpen: input.rightSidebarOpen,
      sidebarWidth: input.sidebarWidth,
      rightSidebarWidth: input.rightSidebarWidth,
      rightSidebarPanelSizes: input.rightSidebarPanelSizes,
    },
    dockedTerminals: {
      terminals: input.dockedTerminals,
      activeTerminalId: input.activeDockedTerminalId,
    },
    terminals,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object";
}

function isWorkspaceTab(value: unknown): value is WorkspaceTabConfig {
  if (!isRecord(value) || typeof value.type !== "string") return false;
  if (typeof value.projectId !== "string") return false;

  switch (value.type) {
    case "chat":
      return typeof value.sessionId === "string";
    case "terminal":
      return typeof value.terminalId === "string";
    case "file":
    case "diff":
      return typeof value.path === "string";
    default:
      return false;
  }
}

function sanitizeNode(value: unknown): WorkspaceNode | null {
  if (!isRecord(value) || typeof value.type !== "string") return null;

  if (value.type === "leaf") {
    if (typeof value.id !== "string" || !Array.isArray(value.tabs)) return null;
    const tabs = value.tabs.filter(isWorkspaceTab);
    const savedActive = typeof value.activeTabId === "string" ? value.activeTabId : null;
    const activeTabId = tabs.some((tab) => getPersistedTabId(tab) === savedActive)
      ? savedActive
      : (tabs[0] ? getPersistedTabId(tabs[0]) : null);
    return { type: "leaf", id: value.id, tabs, activeTabId };
  }

  if (
    value.type !== "split" ||
    typeof value.id !== "string" ||
    (value.direction !== "horizontal" && value.direction !== "vertical") ||
    !Array.isArray(value.children) ||
    value.children.length !== 2 ||
    !Array.isArray(value.sizes) ||
    value.sizes.length !== 2
  ) {
    return null;
  }

  const left = sanitizeNode(value.children[0]);
  const right = sanitizeNode(value.children[1]);
  if (!left || !right) return null;
  const sizes: [number, number] = [
    typeof value.sizes[0] === "number" ? value.sizes[0] : 50,
    typeof value.sizes[1] === "number" ? value.sizes[1] : 50,
  ];
  return { type: "split", id: value.id, direction: value.direction, sizes, children: [left, right] };
}

function getPersistedTabId(tab: WorkspaceTabConfig): string {
  if (tab.type === "chat") return `chat:${tab.sessionId}`;
  if (tab.type === "terminal") return `terminal:${tab.terminalId}`;
  if (tab.type === "file") return `file:${tab.path}`;
  return `diff:${tab.path}`;
}

function normalizeDockedTerminals(value: unknown): LayoutSnapshot["dockedTerminals"] {
  if (!isRecord(value) || !Array.isArray(value.terminals)) {
    return { terminals: [], activeTerminalId: null };
  }
  const terminals = value.terminals.filter(
    (terminal): terminal is TerminalTabConfig => terminal && isWorkspaceTab(terminal) && terminal.type === "terminal",
  );
  const activeTerminalId = typeof value.activeTerminalId === "string" ? value.activeTerminalId : null;
  return { terminals: terminals.slice(0, 3), activeTerminalId };
}

export function normalizeLayoutSnapshot(value: unknown): LayoutSnapshot | null {
  if (!isRecord(value) || value.version !== LAYOUT_VERSION) return null;
  const workspace = isRecord(value.workspace) ? value.workspace : null;
  const selection = isRecord(value.selection) ? value.selection : null;
  const ui = isRecord(value.ui) ? value.ui : null;
  if (!workspace || !selection || !ui) return null;

  const rootNode = sanitizeNode(workspace.rootNode);
  if (!rootNode) return null;
  const terminals = Array.isArray(value.terminals)
    ? value.terminals.filter(
        (terminal): terminal is PersistedTerminal =>
          isRecord(terminal) &&
          typeof terminal.terminalId === "string" &&
          typeof terminal.projectId === "string" &&
          typeof terminal.cwd === "string" &&
          typeof terminal.cols === "number" &&
          typeof terminal.rows === "number",
      )
    : [];

  return {
    version: LAYOUT_VERSION,
    workspace: {
      rootNode,
      activePaneId: typeof workspace.activePaneId === "string" ? workspace.activePaneId : rootNode.id,
    },
    selection: {
      selectedProjectId: typeof selection.selectedProjectId === "string" ? selection.selectedProjectId : null,
      selectedSessionId: typeof selection.selectedSessionId === "string" ? selection.selectedSessionId : null,
    },
    ui: {
      sidebarOpen: ui.sidebarOpen !== false,
      rightSidebarOpen: ui.rightSidebarOpen !== false,
      sidebarWidth: typeof ui.sidebarWidth === "number" ? ui.sidebarWidth : 288,
      rightSidebarWidth: typeof ui.rightSidebarWidth === "number" ? ui.rightSidebarWidth : 288,
      rightSidebarPanelSizes:
        Array.isArray(ui.rightSidebarPanelSizes) &&
        typeof ui.rightSidebarPanelSizes[0] === "number" &&
        typeof ui.rightSidebarPanelSizes[1] === "number"
          ? [ui.rightSidebarPanelSizes[0], ui.rightSidebarPanelSizes[1]]
          : [55, 45],
    },
    dockedTerminals: normalizeDockedTerminals(value.dockedTerminals),
    terminals,
  };
}

export function remapTerminalIds(node: WorkspaceNode, ids: Map<string, string>): WorkspaceNode {
  if (node.type === "leaf") {
    const tabs = node.tabs.flatMap((tab) => {
      if (tab.type !== "terminal") return [tab];
      const terminalId = ids.get(tab.terminalId);
      return terminalId ? [{ ...tab, terminalId }] : [];
    });
    const activeTabId = tabs.some((tab) => getPersistedTabId(tab) === node.activeTabId)
      ? node.activeTabId
      : (tabs[0] ? getPersistedTabId(tabs[0]) : null);
    return { ...node, tabs, activeTabId };
  }

  return {
    ...node,
    children: [remapTerminalIds(node.children[0], ids), remapTerminalIds(node.children[1], ids)],
  };
}

export function remapDockedTerminals(
  terminals: TerminalTabConfig[],
  ids: Map<string, string>,
): TerminalTabConfig[] {
  return terminals.flatMap((terminal) => {
    const terminalId = ids.get(terminal.terminalId);
    return terminalId ? [{ ...terminal, terminalId }] : [];
  });
}

export async function loadLayoutSnapshot(): Promise<LayoutSnapshot | null> {
  const raw = await window.electronApi?.loadWorkspaceLayout();
  return normalizeLayoutSnapshot(raw);
}

export function saveLayoutSnapshot(snapshot: LayoutSnapshot): void {
  const save = window.electronApi?.saveWorkspaceLayout(snapshot);
  void save?.catch(() => {});
}

export function flushLayoutSnapshot(snapshot: LayoutSnapshot): void {
  try {
    window.electronApi?.saveWorkspaceLayoutSync(snapshot);
  } catch {
    // The app may already be shutting down.
  }
}
