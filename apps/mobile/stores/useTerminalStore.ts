import { create } from "zustand";
import type { TerminalServerMessage, TerminalSpawnedEvent } from "@console/types";
import { connectTerminal } from "@console/api";
import { app$ } from "./useAppStore";

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
  /** Raw PTY output (incl. ANSI escapes) per terminal id. Memory only, not persisted. */
  buffers: Record<string, string>;
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
  /** Find a live (spawning/running) terminal for a project, preferring one with matching cwd. */
  findLiveTerminal: (projectId: string, cwd?: string) => string | undefined;
  markStatus: (id: string, status: TerminalStatus, extra?: Partial<TerminalRecord>) => void;
  end: (id: string) => void;
  /** Subscribe to a terminal's server events (output/exit/error). Returns an unsubscribe fn. */
  subscribe: (id: string, listener: (message: TerminalServerMessage) => void) => () => void;
}

/** Live registry of open terminal sinks keyed by terminal id. */
const terminalSinks: Record<string, TerminalSink> = {};

/** Per-terminal event listener registry. */
const listeners: Record<string, Set<(message: TerminalServerMessage) => void>> = {};

/**
 * Coalesces output appends so high-throughput PTYs don't ship the whole buffer
 * across the RN bridge on every frame. Flushed at most every FLUSH_INTERVAL_MS.
 */
const FLUSH_INTERVAL_MS = 80;
const pendingAppends: Record<string, string[]> = {};
let flushTimer: ReturnType<typeof setInterval> | null = null;

function getBaseUrl(): string {
  return app$.backendUrl.peek() ?? "http://localhost:3000";
}

/** Emit a server message to a specific terminal's listeners. */
function emit(terminalId: string, message: TerminalServerMessage): void {
  const set = listeners[terminalId];
  if (set) {
    for (const listener of set) listener(message);
  }
}

/** Queue raw output for `terminalId`; flushed to `buffers` on the shared interval. */
function appendOutput(terminalId: string, data: string): void {
  (pendingAppends[terminalId] ??= []).push(data);
  if (!flushTimer) {
    flushTimer = setInterval(flushBuffers, FLUSH_INTERVAL_MS);
  }
}

function flushBuffers(): void {
  if (Object.keys(pendingAppends).length === 0) {
    // Idle: stop ticking until the next append re-arms the interval.
    if (flushTimer) {
      clearInterval(flushTimer);
      flushTimer = null;
    }
    return;
  }
  useTerminalStore.setState((state) => {
    const next = { ...state.buffers };
    for (const [id, chunks] of Object.entries(pendingAppends)) {
      next[id] = (next[id] ?? "") + chunks.join("");
    }
    return { buffers: next };
  });
  for (const id of Object.keys(pendingAppends)) delete pendingAppends[id];
}

function dropBuffer(id: string): void {
  delete pendingAppends[id];
  useTerminalStore.setState((state) => {
    if (!(id in state.buffers)) return state;
    const next = { ...state.buffers };
    delete next[id];
    return { buffers: next };
  });
}

export const useTerminalStore = create<TerminalStoreState>((set, get) => ({
  terminals: {},
  buffers: {},
  opening: {},

  openTerminal: async ({ projectId, cwd, cols = 80, rows = 24, label, shell }) => {
    // Reuse an in-flight spawn for the same project+cwd so double-clicks don't
    // create two PTYs pointing at the same tab target.
    const cacheKey = `${projectId}::${cwd}`;
    const existing = get().opening[cacheKey];
    if (existing) return existing;

    const promise = (async () => {
      // Buffer output that arrives before we know the terminal id (should not happen
      // for well-ordered servers, but guards against the spawned→output race where
      // the shell prompt arrives before the spawned handler has run).
      const earlyOutput: string[] = [];

      const sink = connectTerminal({
        baseUrl: getBaseUrl(),
        params: { cwd, cols, rows, label, shell },
        onEvent: (message) => {
          // Spawned must be handled first: it defines the terminal id for every
          // subsequent output/exit/error frame from this sink.
          if (message.type === "spawned") {
            const id = message.id;
            if (!sinkTerminalId(sink)) {
              sinkTerminalId(sink, id);
              terminalSinks[id] = sink;
              // Flush any early output that slipped in before the spawned frame
              // was processed (defensive).
              if (earlyOutput.length > 0) {
                for (const data of earlyOutput) appendOutput(id, data);
                earlyOutput.length = 0;
              }
              set((state) => {
                if (state.terminals[id]) return state;
                return {
                  terminals: {
                    ...state.terminals,
                    [id]: {
                      id,
                      projectId,
                      status: "running",
                      pid: message.pid,
                      shell: message.shell,
                      cwd: message.cwd,
                      cols: message.cols,
                      rows: message.rows,
                      revision: 0,
                    },
                  },
                  buffers: { ...state.buffers, [id]: state.buffers[id] ?? "" },
                  opening: { ...state.opening, [cacheKey]: undefined },
                };
              });
            }
            emit(id, message);
            return;
          }

          const terminalId = sinkTerminalId(sink);
          if (!terminalId) {
            if (message.type === "output") earlyOutput.push(message.data);
            return;
          }

          if (message.type === "output") {
            appendOutput(terminalId, message.data);
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
          let closedId: string | undefined;
          for (const key of Object.keys(terminalSinks)) {
            if (terminalSinks[key] === sink) {
              closedId = key;
              delete terminalSinks[key];
            }
          }
          const knownId = closedId ?? sinkTerminalId(sink);
          if (knownId) {
            const term = get().terminals[knownId];
            // If we still think it's running/spawning, the socket died
            // unexpectedly — surface as exited so the UI can respawn.
            if (term && (term.status === "running" || term.status === "spawning")) {
              get().markStatus(knownId, "exited");
              emit(knownId, { type: "exit", code: null });
            }
          }
        },
      });

      const spawned = await sink.open();
      // Idempotent: onEvent already registered the terminal on the spawned frame.
      // This flush covers any earlyOutput that arrived between spawn and the
      // onEvent microtask, and handles the case where onEvent fired after await.
      if (!sinkTerminalId(sink)) {
        sinkTerminalId(sink, spawned.id);
        terminalSinks[spawned.id] = sink;
      }
      if (earlyOutput.length > 0) {
        for (const data of earlyOutput) appendOutput(spawned.id, data);
        earlyOutput.length = 0;
      }

      set((state) => {
        if (state.terminals[spawned.id]) {
          // Already created in onEvent — just clear the opening flag.
          if (state.opening[cacheKey] !== undefined) {
            return { opening: { ...state.opening, [cacheKey]: undefined } };
          }
          return state;
        }
        return {
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
          buffers: { ...state.buffers, [spawned.id]: state.buffers[spawned.id] ?? "" },
          opening: { ...state.opening, [cacheKey]: undefined },
        };
      });
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
    dropBuffer(id);
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
    dropBuffer(id);
    set((state) => {
      const next = { ...state.terminals };
      delete next[id];
      return { terminals: next };
    });
  },

  findLiveTerminal: (projectId, cwd) => {
    const candidates = Object.values(get().terminals).filter(
      (t) => t.projectId === projectId && (t.status === "spawning" || t.status === "running"),
    );
    if (cwd) {
      const match = candidates.find((t) => t.cwd === cwd);
      if (match) return match.id;
    }
    return candidates[0]?.id;
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
