import React from "react";
import { useAppStore } from "../store/useAppStore";
import { useServerStore } from "../store/useServerStore";
import { useTerminalStore } from "../store/useTerminalStore";
import { useWorkspaceStore } from "./useWorkspaceStore";
import {
  createLayoutSnapshot,
  flushLayoutSnapshot,
  loadLayoutSnapshot,
  remapDockedTerminals,
  remapTerminalIds,
  saveLayoutSnapshot,
} from "./layout-persistence";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;

/** Restores and persists the complete local Electron workspace layout. */
export function LayoutPersistence() {
  const selectedProjectId = useAppStore((state) => state.selectedProjectId);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const rightSidebarOpen = useAppStore((state) => state.rightSidebarOpen);
  const sidebarWidth = useAppStore((state) => state.sidebarWidth);
  const rightSidebarWidth = useAppStore((state) => state.rightSidebarWidth);
  const rightSidebarPanelSizes = useAppStore((state) => state.rightSidebarPanelSizes);
  const dockedTerminals = useAppStore((state) => state.dockedTerminals);
  const activeDockedTerminalId = useAppStore((state) => state.activeDockedTerminalId);
  const initServer = useServerStore((state) => state.init);
  const rootNode = useWorkspaceStore((state) => state.rootNode);
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const terminalRecords = useTerminalStore((state) => state.terminals);
  const openTerminal = useTerminalStore((state) => state.openTerminal);

  const hydratedRef = React.useRef(false);
  const restoreStartedRef = React.useRef(false);
  const latestSnapshotRef = React.useRef<ReturnType<typeof createLayoutSnapshot> | null>(null);

  React.useEffect(() => {
    if (restoreStartedRef.current) return;
    restoreStartedRef.current = true;

    const restore = async () => {
      initServer();
      const snapshot = await loadLayoutSnapshot().catch(() => null);
      if (!snapshot) {
        hydratedRef.current = true;
        return;
      }

      const terminalIds = new Map<string, string>();
      for (const terminal of snapshot.terminals) {
        try {
          const spawned = await openTerminal({
            projectId: terminal.projectId,
            cwd: terminal.cwd,
            cols: terminal.cols,
            rows: terminal.rows,
            shell: terminal.shell,
          });
          terminalIds.set(terminal.terminalId, spawned.id);
        } catch {
          // Keep the rest of the layout if an old terminal cannot be recreated.
        }
      }

      const appState = useAppStore.getState();
      appState.setSelectedProjectId(snapshot.selection.selectedProjectId);
      appState.setSelectedSessionId(snapshot.selection.selectedSessionId);
      appState.setSidebarOpen(snapshot.ui.sidebarOpen);
      appState.setRightSidebarOpen(snapshot.ui.rightSidebarOpen);
      appState.setSidebarWidth(Math.min(Math.max(snapshot.ui.sidebarWidth, SIDEBAR_MIN), SIDEBAR_MAX));
      appState.setRightSidebarWidth(
        Math.min(Math.max(snapshot.ui.rightSidebarWidth, SIDEBAR_MIN), SIDEBAR_MAX),
      );
      appState.setRightSidebarPanelSizes(snapshot.ui.rightSidebarPanelSizes);
      appState.restoreDockedTerminals(
        remapDockedTerminals(snapshot.dockedTerminals.terminals, terminalIds),
        terminalIds.get(snapshot.dockedTerminals.activeTerminalId ?? "") ?? null,
      );

      useWorkspaceStore.getState().restoreLayout(
        remapTerminalIds(snapshot.workspace.rootNode, terminalIds),
        snapshot.workspace.activePaneId,
      );
      hydratedRef.current = true;
    };

    void restore();
  }, [initServer, openTerminal]);

  const snapshot = React.useMemo(
    () =>
      createLayoutSnapshot({
        rootNode,
        activePaneId,
        selectedProjectId,
        selectedSessionId,
        sidebarOpen,
        rightSidebarOpen,
        sidebarWidth,
        rightSidebarWidth,
        rightSidebarPanelSizes,
        dockedTerminals,
        activeDockedTerminalId,
        terminalRecords,
      }),
    [
      rootNode,
      activePaneId,
      selectedProjectId,
      selectedSessionId,
      sidebarOpen,
      rightSidebarOpen,
      sidebarWidth,
      rightSidebarWidth,
      rightSidebarPanelSizes,
      dockedTerminals,
      activeDockedTerminalId,
      terminalRecords,
    ],
  );

  latestSnapshotRef.current = snapshot;

  React.useEffect(() => {
    if (!hydratedRef.current) return;
    const timer = setTimeout(() => saveLayoutSnapshot(snapshot), 350);
    return () => clearTimeout(timer);
  }, [snapshot]);

  React.useEffect(() => {
    const flush = () => {
      if (hydratedRef.current && latestSnapshotRef.current) {
        flushLayoutSnapshot(latestSnapshotRef.current);
      }
    };
    window.addEventListener("beforeunload", flush);
    return () => window.removeEventListener("beforeunload", flush);
  }, []);

  return null;
}
