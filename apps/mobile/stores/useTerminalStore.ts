import { create } from "zustand";
import type { TerminalServerMessage, TerminalSpawnedEvent } from "@console/types";
import { connectTerminal } from "@console/api";
import { useAppStore } from "./useAppStore";

export type TerminalStatus = "spawning" | "running" | "exited" | "error";

export interface TerminalRecord {
  id: string;
  projectId: string;
  status: TerminalStatus;
  pid?: number;
  shell?: string;
  cwd?: string;
  cols: number;
  rows: number;
  error?: string;
  /** Nonce bumped on every event so subscribers (e.g. exit banner) re-render. */
  revision: number;
}

/** Handle returned by connectTerminal (input/resize/kill/close). */
export type TerminalSink = ReturnType<typeof connectTerminal>;

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
  write: (id: string, data: string) => void;
  resize: (id: string, cols: number, rows: number) => void;
  kill: (id: string) => Promise<void>;
  markStatus: (id: string, status: TerminalStatus, extra?: Partial<TerminalRecord>) => void;
  end: (id: string) => void;
  /** Subscribe to a terminal's server events (output/exit/error). Returns an unsubscribe fn. */
  subscribe: (id: string, listener: (message: TerminalServerMessage) => void) => () => void;
}

/** Live registry of open terminal sinks keyed by terminal id. */
const terminalSinks: Record<string, TerminalSink> = {};

/** Per-terminal event listener registry. */
const listeners: Record<string, Set<(message: TerminalServerMessage) => void>> = {};

function getBaseUrl(): string {
  return useAppStore.getState().backendUrl ?? "http://localhost:3000";
}

/** Emit a server message to a specific terminal's listeners. */
function emit(terminalId: string, message: TerminalServerMessage): void {
  const set = listeners[terminalId];
  if (set) {
    for (const listener of set) listener(message);
  }
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
      const sink = connectTerminal({
        baseUrl: getBaseUrl(),
        params: { cwd, cols, rows, label, shell },
        onEvent: (message) => {
          // Route messages to the correct terminal via the sink's own id
          // (assigned on the "spawned" frame).
          const terminalId = sinkTerminalId(sink);
          if (!terminalId) return;

          if (message.type === "output") {
            emit(terminalId, message);
          } else if (message.type === "exit") {
            get().markStatus(terminalId, "exited");
            emit(terminalId, message);
          } else if (message.type === "error") {
            get().markStatus(terminalId, "error", { error: message.message });
            emit(terminalId, message);
          }
        },
        onClose: () => {
          for (const key of Object.keys(terminalSinks)) {
            if (terminalSinks[key] === sink) delete terminalSinks[key];
          }
        },
      });

      const spawned = await sink.open();
      terminalSinks[spawned.id] = sink;
      sinkTerminalId(sink, spawned.id);

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

  write: (id, data) => {
    const sink = terminalSinks[id];
    if (sink) sink.input(data);
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
    const sink = terminalSinks[id];
    if (sink) sink.resize(cols, rows);
  },

  kill: async (id) => {
    const sink = terminalSinks[id];
    if (sink) {
      sink.kill();
      sink.close();
      delete terminalSinks[id];
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
    const sink = terminalSinks[id];
    if (sink) {
      sink.close();
      delete terminalSinks[id];
    }
    set((state) => {
      const next = { ...state.terminals };
      delete next[id];
      return { terminals: next };
    });
  },

  subscribe: (id, listener) => {
    let set = listeners[id];
    if (!set) {
      set = new Set();
      listeners[id] = set;
    }
    set.add(listener);
    return () => {
      set!.delete(listener);
      if (set!.size === 0) delete listeners[id];
    };
  },
}));

/** Track which terminal id a sink belongs to (assigned on the spawned frame). */
const sinkIdMap = new WeakMap<TerminalSink, string>();

function sinkTerminalId(sink: TerminalSink, id?: string): string | undefined {
  if (id) {
    sinkIdMap.set(sink, id);
    return id;
  }
  return sinkIdMap.get(sink);
}
