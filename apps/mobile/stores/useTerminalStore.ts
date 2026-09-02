import { batch, observable } from "@legendapp/state";
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

/**
 * Live terminal metadata keyed by terminal id.
 * See docs/legend-state-and-list-migration.md.
 */
export const terminals$ = observable<Record<string, TerminalRecord>>({});

/** Raw PTY output (incl. ANSI escapes) per terminal id. Memory only, not persisted. */
export const terminalBuffers$ = observable<Record<string, string>>({});

/** In-flight spawn promises so concurrent open calls spawn only one PTY per terminal. */
const openingPromises = new Map<string, Promise<TerminalSpawnedEvent>>();

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

/** Queue raw output for `terminalId`; flushed to `terminalBuffers$` on the shared interval. */
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
  batch(() => {
    for (const [id, chunks] of Object.entries(pendingAppends)) {
      terminalBuffers$[id].set((prev) => (prev ?? "") + chunks.join(""));
    }
  });
  for (const id of Object.keys(pendingAppends)) delete pendingAppends[id];
}

function dropBuffer(id: string): void {
  delete pendingAppends[id];
  terminalBuffers$[id].delete();
}

function ensureTerminal(record: TerminalRecord): void {
  if (!terminals$[record.id].peek()) {
    terminals$[record.id].set(record);
  }
}

export async function openTerminal(opts: {
  projectId: string;
  cwd: string;
  cols?: number;
  rows?: number;
  label?: string;
  shell?: string;
}): Promise<TerminalSpawnedEvent> {
  // Reuse an in-flight spawn for the same project+cwd so double-clicks don't
  // create two PTYs pointing at the same tab target.
  const cacheKey = `${opts.projectId}::${opts.cwd}`;
  const existing = openingPromises.get(cacheKey);
  if (existing) return existing;

  const cols = opts.cols ?? 80;
  const rows = opts.rows ?? 24;

  const promise = (async () => {
    // Buffer output that arrives before we know the terminal id (should not happen
    // for well-ordered servers, but guards against the spawned→output race where
    // the shell prompt arrives before the spawned handler has run).
    const earlyOutput: string[] = [];

    const sink = connectTerminal({
      baseUrl: getBaseUrl(),
      params: { cwd: opts.cwd, cols, rows, label: opts.label, shell: opts.shell },
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
            ensureTerminal({
              id,
              projectId: opts.projectId,
              status: "running",
              pid: message.pid,
              shell: message.shell,
              cwd: message.cwd,
              cols: message.cols,
              rows: message.rows,
              revision: 0,
            });
            openingPromises.delete(cacheKey);
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
          markStatus(terminalId, "exited");
          emit(terminalId, message);
        } else if (message.type === "error") {
          markStatus(terminalId, "error", { error: message.message });
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
          const term = terminals$[knownId].peek();
          // If we still think it's running/spawning, the socket died
          // unexpectedly — surface as exited so the UI can respawn.
          if (term && (term.status === "running" || term.status === "spawning")) {
            markStatus(knownId, "exited");
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

    ensureTerminal({
      id: spawned.id,
      projectId: opts.projectId,
      status: "running",
      pid: spawned.pid,
      shell: spawned.shell,
      cwd: spawned.cwd,
      cols: spawned.cols,
      rows: spawned.rows,
      revision: 0,
    });
    openingPromises.delete(cacheKey);
    return spawned;
  })().catch((err) => {
    openingPromises.delete(cacheKey);
    throw err;
  });

  openingPromises.set(cacheKey, promise);
  return promise;
}

export function write(id: string, data: string): void {
  const sink = terminalSinks[id];
  if (sink) sink.input(data);
}

export function resize(id: string, cols: number, rows: number): void {
  const term = terminals$[id].peek();
  if (term) {
    terminals$[id].set({ ...term, cols, rows, revision: term.revision + 1 });
  }
  const sink = terminalSinks[id];
  if (sink) sink.resize(cols, rows);
}

export async function kill(id: string): Promise<void> {
  const sink = terminalSinks[id];
  if (sink) {
    sink.kill();
    sink.close();
    delete terminalSinks[id];
  }
  dropBuffer(id);
  end(id);
}

export function markStatus(id: string, status: TerminalStatus, extra?: Partial<TerminalRecord>): void {
  const term = terminals$[id].peek();
  if (!term) return;
  terminals$[id].set({
    ...term,
    ...extra,
    status,
    revision: term.revision + 1,
  });
}

export function end(id: string): void {
  const sink = terminalSinks[id];
  if (sink) {
    sink.close();
    delete terminalSinks[id];
  }
  dropBuffer(id);
  terminals$[id].delete();
}

/** Find a live (spawning/running) terminal for a project, preferring one with matching cwd. */
export function findLiveTerminal(projectId: string, cwd?: string): string | undefined {
  const candidates = Object.values(terminals$.peek()).filter(
    (t) => t.projectId === projectId && (t.status === "spawning" || t.status === "running"),
  );
  if (cwd) {
    const match = candidates.find((t) => t.cwd === cwd);
    if (match) return match.id;
  }
  return candidates[0]?.id;
}

/** Subscribe to a terminal's server events (output/exit/error). Returns an unsubscribe fn. */
export function subscribe(
  id: string,
  listener: (message: TerminalServerMessage) => void,
): () => void {
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
}

/** Track which terminal id a sink belongs to (assigned on the spawned frame). */
const sinkIdMap = new WeakMap<TerminalSink, string>();

function sinkTerminalId(sink: TerminalSink, id?: string): string | undefined {
  if (id) {
    sinkIdMap.set(sink, id);
    return id;
  }
  return sinkIdMap.get(sink);
}

export function clearAllTerminals(): void {
  // Close live sockets and drop pending buffers so old server PTYs don't leak.
  for (const sink of Object.values(terminalSinks)) {
    try {
      sink.close();
    } catch {}
  }
  for (const key of Object.keys(terminalSinks)) delete terminalSinks[key];
  for (const key of Object.keys(listeners)) delete listeners[key];
  for (const key of Object.keys(pendingAppends)) delete pendingAppends[key];
  if (flushTimer) {
    clearInterval(flushTimer);
    flushTimer = null;
  }
  openingPromises.clear();
  batch(() => {
    terminals$.set({} as Record<string, TerminalRecord>);
    terminalBuffers$.set({} as Record<string, string>);
  });
}
