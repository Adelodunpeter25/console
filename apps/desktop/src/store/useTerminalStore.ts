import { create } from "zustand";
import { tauriApi } from "../lib/tauri-api";
import { useWorkspaceStore } from "../layout/useWorkspaceStore";
import type { TerminalStatus, TerminalRecord } from "../types";
import type { TerminalSpawnedEvent } from "@console/types";

interface TerminalStoreState {
  terminals: Record<string, TerminalRecord>;
  /** Promise cache so concurrent open calls spawn only one PTY per terminal. */
  opening: Record<string, Promise<TerminalSpawnedEvent> | undefined>;
  openTerminal: (opts: {
    projectId: string;
    cwd: string;
    cols?: number;
    rows?: number;
    label?: string;
    shell?: string;
  }) => Promise<TerminalSpawnedEvent>;
  /** Spawn a PTY and open it as a workspace tab in one call. */
  openTerminalTab: (opts: {
    projectId: string;
    cwd: string;
    cols?: number;
    rows?: number;
    label?: string;
    shell?: string;
  }) => Promise<TerminalSpawnedEvent>;
  write: (id: string, data: string) => void;
  resize: (id: string, cols: number, rows: number) => void;
  kill: (id: string) => Promise<void>;
  markStatus: (id: string, status: TerminalStatus, extra?: Partial<TerminalRecord>) => void;
  end: (id: string) => void;
}

export const useTerminalStore = create<TerminalStoreState>((set, get) => ({
  terminals: {},
  opening: {},

  openTerminal: async ({ projectId, cwd, cols = 80, rows = 24, label, shell }) => {
    // Reuse an in-flight spawn for the same project+cwd so double-clicks don't
    // create two PTYs pointing at the same tab target.
    const cacheKey = `${projectId}::${cwd}`;
    const existing = get().opening[cacheKey];
    if (existing) return existing;

    const promise = (async () => {
      const spawned = await tauriApi.terminalOpen(cwd, { cols, rows, label, shell });
      set((state) => ({
        terminals: {
          ...state.terminals,
          [spawned.id]: {
            id: spawned.id,
            projectId,
            status: "running",
            pid: spawned.pid,
            shell: spawned.shell,
            cwd: spawned.cwd,
            cols: spawned.cols,
            rows: spawned.rows,
            revision: 0,
          },
        },
        opening: { ...state.opening, [cacheKey]: undefined },
      }));
      return spawned;
    })().catch((err) => {
      set((state) => ({ opening: { ...state.opening, [cacheKey]: undefined } }));
      throw err;
    });

    set((state) => ({ opening: { ...state.opening, [cacheKey]: promise } }));
    return promise;
  },

  openTerminalTab: async (opts) => {
    const spawned = await get().openTerminal(opts);
    // Open the tab after the PTY exists so the tab always has a live session.
    useWorkspaceStore.getState().openTerminalTab({
      type: "terminal",
      projectId: opts.projectId,
      terminalId: spawned.id,
      title: opts.label ?? "Terminal",
    });
    return spawned;
  },

  write: (id, data) => {
    tauriApi.terminalInput(id, data).catch((err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("no active terminal")) {
        get().markStatus(id, "exited", { error: msg });
      }
    });
  },

  resize: (id, cols, rows) => {
    set((state) => {
      const term = state.terminals[id];
      if (!term) return state;
      return {
        terminals: {
          ...state.terminals,
          [id]: { ...term, cols, rows, revision: term.revision + 1 },
        },
      };
    });
    tauriApi.terminalResize(id, cols, rows).catch((err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("no active terminal")) {
        get().markStatus(id, "exited", { error: msg });
      }
    });
  },

  kill: async (id) => {
    try {
      await tauriApi.terminalKill(id);
    } catch {
      // The PTY may already have exited; treat as already gone.
    }
    get().end(id);
  },

  markStatus: (id, status, extra) => {
    set((state) => {
      const term = state.terminals[id];
      if (!term) return state;
      return {
        terminals: {
          ...state.terminals,
          [id]: {
            ...term,
            ...extra,
            status,
            revision: term.revision + 1,
          },
        },
      };
    });
  },

  end: (id) => {
    set((state) => {
      const next = { ...state.terminals };
      delete next[id];
      return { terminals: next };
    });
  },
}));