/**
 * Terminal PTY Manager.
 *
 * Owns the lifecycle of every terminal session on the server, backed by
 * Bun.Terminal (native PTY built into the Bun runtime). A PTY is identified
 * by a random `terminalId`; the WebSocket route creates instances here and
 * streams their output/exit events back to the client.
 *
 * All instances are tracked so a server shutdown (or daemon restart) can kill
 * lingering shells deterministically instead of leaking processes.
 */
import { randomUUID } from "node:crypto";
import { existsSync, statSync } from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import type {
  TerminalId,
  TerminalOutputEvent,
  TerminalSpawnParams,
  TerminalSpawnedEvent,
} from "@console/types";

/** Callback the route registers to receive pty events for a session. */
export interface PtyCallbacks {
  onData: (event: TerminalOutputEvent) => void;
  onExit: (code: number | null) => void;
  onError: (message: string) => void;
}

/** Minimal shape of a Bun-spawned subprocess attached to a terminal. */
interface PtyProcess {
  pid: number;
  exited: Promise<number>;
  kill(code?: number): void;
}

/** Cap for output buffered while paused, so a flooding program can't balloon memory. */
const PAUSED_BUFFER_LIMIT_BYTES = 8 * 1024 * 1024;

/** Coalescing window: PTY data events inside one window ship as a single frame. */
const OUTPUT_FLUSH_MS = 8;
/** Flush early once the queued window reaches this size. */
const OUTPUT_FLUSH_BYTES = 4 * 1024;
/** Max bytes per WebSocket frame when flushing a large backlog. */
const OUTPUT_FRAME_BYTES = 64 * 1024;

interface PtySession {
  id: TerminalId;
  terminal: Bun.Terminal;
  /** null until the shell subprocess actually starts (see startShell). */
  proc: PtyProcess | null;
  shell: string;
  cwd: string;
  cols: number;
  rows: number;
  callbacks?: PtyCallbacks;
  pending: string[];
  killed: boolean;
  paused: boolean;
  pausedBuffer: string[];
  pausedBufferBytes: number;
  /** Output chunks waiting for the coalescing flush (see OUTPUT_FLUSH_MS). */
  outputQueue: string[];
  outputQueueBytes: number;
  flushTimer?: ReturnType<typeof setTimeout>;
  decoder: TextDecoder;
  /** Whether the shell subprocess has been started. */
  shellStarted: boolean;
  /** Input received before the shell started, flushed on start. */
  pendingInput: string[];
  resolveStart?: (event: TerminalSpawnedEvent) => void;
  rejectStart?: (cause: unknown) => void;
}

// --- Hardening: PTY spawn allowlist + rate limits ---

const ALLOWED_SHELLS = new Set<string>([
  "/bin/bash",
  "/bin/zsh",
  "/bin/sh",
  "/usr/bin/bash",
  "/usr/bin/zsh",
  "/usr/local/bin/bash",
  "/usr/local/bin/zsh",
  "/bin/fish",
  "/usr/bin/fish",
  "powershell.exe",
  "pwsh.exe",
  "cmd.exe",
]);

const MAX_CONCURRENT_TERMINALS = 20;
const MAX_SPAWNS_PER_MINUTE = 20;

function isAllowedShell(shell: string): boolean {
  if (ALLOWED_SHELLS.has(shell)) return true;
  const hostShell = process.env.SHELL;
  if (hostShell && shell === hostShell) return true;
  const base = path.basename(shell);
  for (const allowed of ALLOWED_SHELLS) {
    if (path.basename(allowed) === base) return true;
  }
  return false;
}

export class TerminalPtyManager {
  private sessions = new Map<TerminalId, PtySession>();
  private spawnTimestamps: number[] = [];

  /**
   * Spawn a new shell PTY in the given working directory.
   * Throws if `cwd` does not exist so clients get a clean error.
   *
   * The shell starts immediately at the requested size — clients already send
   * their true grid in the spawn params, so waiting for a first resize only
   * adds latency to every open.
   */
  spawn(params: TerminalSpawnParams): {
    id: TerminalId;
    ready: Promise<TerminalSpawnedEvent>;
  } {
    // Rate-limit + concurrency guard before allocating any FD.
    const now = Date.now();
    this.spawnTimestamps = this.spawnTimestamps.filter((t) => now - t < 60_000);
    if (this.spawnTimestamps.length >= MAX_SPAWNS_PER_MINUTE) {
      throw new Error("Too many terminal spawns — rate limited. Try again later.");
    }
    if (this.sessions.size >= MAX_CONCURRENT_TERMINALS) {
      throw new Error("Too many concurrent terminals.");
    }

    const cwd = path.resolve(params.cwd);
    if (!existsSync(cwd)) {
      throw new Error(`Cannot spawn terminal: working directory does not exist: ${cwd}`);
    }
    try {
      const st = statSync(cwd);
      if (!st.isDirectory()) throw new Error(`Not a directory: ${cwd}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.startsWith("Not a directory")) throw e;
      throw new Error(`Cannot spawn terminal: working directory is not a directory: ${cwd}`);
    }

    const rawShell = params.shell || process.env.SHELL || this.defaultShell();
    const shellForCheck = rawShell.includes("/") ? path.resolve(rawShell) : rawShell;
    const candidate = rawShell.includes("/") ? shellForCheck : rawShell;
    if (!isAllowedShell(candidate) && !isAllowedShell(path.basename(candidate))) {
      throw new Error(`Shell not allowed: ${rawShell}`);
    }
    const shell = candidate;
    const cols = params.cols ?? 80;
    const rows = params.rows ?? 24;

    const id: TerminalId = randomUUID();
    const decoder = new TextDecoder("utf-8", { fatal: false });
    const session: PtySession = {
      id,
      // Assigned immediately after construction below.
      terminal: undefined as unknown as Bun.Terminal,
      // Assigned in startShell(); null until then.
      proc: null,
      shell,
      cwd,
      cols,
      rows,
      pending: [],
      killed: false,
      paused: false,
      pausedBuffer: [],
      pausedBufferBytes: 0,
      outputQueue: [],
      outputQueueBytes: 0,
      decoder,
      shellStarted: false,
      pendingInput: [],
    };
    this.sessions.set(id, session);
    this.spawnTimestamps.push(Date.now());

    session.terminal = new Bun.Terminal({
      name: "xterm-256color",
      cols,
      rows,
      // Buffer output that arrives before the WebSocket route has attached
      // callbacks so the initial shell prompt is never dropped.
      data: (_terminal, data) => {
        const text = session.decoder.decode(data, { stream: true });
        if (text.length > 0) {
          this.handleOutput(session, text);
        }
      },
    });

    const ready = new Promise<TerminalSpawnedEvent>((resolve, reject) => {
      session.resolveStart = resolve;
      session.rejectStart = reject;
    });
    // Start the shell right away at the requested grid size.
    this.startShell(session);

    return { id, ready };
  }

  /** Start the shell subprocess and resolve the spawn promise. */
  private startShell(session: PtySession): void {
    if (session.shellStarted || session.killed) return;
    session.shellStarted = true;

    const proc = Bun.spawn([session.shell], {
      terminal: session.terminal,
      cwd: session.cwd,
      env: {
        ...process.env,
        TERM: "xterm-256color",
        CONSOLE_TERMINAL: "true",
      } as Record<string, string>,
    });
    session.proc = {
      pid: proc.pid,
      exited: proc.exited,
      kill: (code?: number) => proc.kill(code),
    };

    proc.exited.then((code) => {
      if (session.killed) return;
      session.killed = true;
      this.sessions.delete(session.id);
      session.callbacks?.onExit(code);
    });

    // Flush any keystrokes that arrived before the shell existed.
    if (session.pendingInput.length > 0) {
      const queued = session.pendingInput.join("");
      session.pendingInput.length = 0;
      try {
        session.terminal.write(queued);
      } catch {
        // Terminal closed mid-flush — nothing more to do.
      }
    }

    session.resolveStart?.({
      type: "spawned",
      id: session.id,
      pid: proc.pid,
      shell: session.shell,
      cwd: session.cwd,
      cols: session.cols,
      rows: session.rows,
    });
  }

  /** Route PTY output to callbacks, coalescing bursts into fewer frames. */
  private handleOutput(session: PtySession, data: string): void {
    if (session.killed) return;
    if (session.paused) {
      // Client send buffer saturated: hold output until resume().
      session.pausedBuffer.push(data);
      session.pausedBufferBytes += data.length;
      if (session.pausedBufferBytes > PAUSED_BUFFER_LIMIT_BYTES) {
        // Drop oldest to stay bounded; the client is already behind anyway.
        const dropped = session.pausedBuffer.shift()!;
        session.pausedBufferBytes -= dropped.length;
      }
      return;
    }
    if (!session.callbacks) {
      session.pending.push(data);
      // Cap buffered early output to avoid unbounded growth if attach never happens
      if (session.pending.length > 100) {
        session.pending.shift();
      }
      return;
    }
    session.outputQueue.push(data);
    session.outputQueueBytes += data.length;
    if (session.outputQueueBytes >= OUTPUT_FLUSH_BYTES) {
      this.flushOutput(session);
    } else if (!session.flushTimer) {
      session.flushTimer = setTimeout(() => this.flushOutput(session), OUTPUT_FLUSH_MS);
    }
  }

  /** Ship the coalesced output window as a single frame. */
  private flushOutput(session: PtySession): void {
    if (session.flushTimer) {
      clearTimeout(session.flushTimer);
      session.flushTimer = undefined;
    }
    if (session.outputQueue.length === 0 || session.killed) return;
    const queued = session.outputQueue;
    session.outputQueue = [];
    session.outputQueueBytes = 0;
    if (session.paused || !session.callbacks) {
      // Paused (or detached) mid-window: hold for resume/attach instead.
      for (const chunk of queued) {
        session.pausedBuffer.push(chunk);
        session.pausedBufferBytes += chunk.length;
      }
      return;
    }
    session.callbacks.onData({ type: "output", data: queued.join("") });
  }

  /** Attach a WebSocket-backed callback set to an existing session. */
  attach(id: TerminalId, callbacks: PtyCallbacks): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`No terminal session found for id: ${id}`);
    session.callbacks = callbacks;
    // Flush any output that arrived between spawn and attach (e.g. shell prompt)
    if (session.pending.length > 0) {
      const queued = session.pending.join("");
      session.pending.length = 0;
      callbacks.onData({ type: "output", data: queued });
    }
  }

  /** Write raw bytes into the PTY (keystrokes, pasted text). */
  write(id: TerminalId, data: string): boolean {
    if (data.length > 256 * 1024) return false;
    const session = this.sessions.get(id);
    if (!session || session.killed) return false;
    // Shell not started yet (waiting for first resize): hold keystrokes.
    if (!session.shellStarted) {
      session.pendingInput.push(data);
      return true;
    }
    try {
      session.terminal.write(data);
      return true;
    } catch {
      return false;
    }
  }

  /** Resize the PTY's viewport. */
  resize(id: TerminalId, cols: number, rows: number): boolean {
    if (!Number.isFinite(cols) || !Number.isFinite(rows) || cols < 1 || cols > 500 || rows < 1 || rows > 200) return false;
    const session = this.sessions.get(id);
    if (!session || session.killed) return false;
    try {
      session.terminal.resize(cols, rows);
      session.cols = cols;
      session.rows = rows;
      return true;
    } catch {
      return false;
    }
  }

  /** Kill a PTY session and remove it from the registry. */
  kill(id: TerminalId): void {
    const session = this.sessions.get(id);
    if (!session || session.killed) return;
    session.killed = true;
    this.sessions.delete(id);
    if (session.flushTimer) {
      clearTimeout(session.flushTimer);
      session.flushTimer = undefined;
    }
    if (!session.shellStarted) {
      session.rejectStart?.(new Error("Terminal killed before the shell started."));
    }
    try {
      // Interactive shells ignore SIGTERM/SIGHUP; SIGKILL is deterministic.
      session.proc?.kill(9);
    } catch {
      // Already dead — fine.
    }
    try {
      session.terminal.close();
    } catch {
      // Already closed — fine.
    }
    session.callbacks?.onExit(null);
  }

  /**
   * Pause a PTY's output. Used when the client socket's send buffer is
   * saturated so a flooding program (`yes`, huge file cats) can't balloon
   * memory; output is held here until resume(). Unlike node-pty's kernel-level
   * flow control this buffers in-process, capped at PAUSED_BUFFER_LIMIT_BYTES.
   */
  pause(id: TerminalId): void {
    const session = this.sessions.get(id);
    if (session && !session.killed) {
      session.paused = true;
    }
  }

  /** Resume a paused PTY's output (the send buffer drained). */
  resume(id: TerminalId): void {
    const session = this.sessions.get(id);
    if (session && !session.killed && session.paused) {
      session.paused = false;
      if (session.pausedBuffer.length === 0) return;
      const joined = session.pausedBuffer.join("");
      session.pausedBuffer = [];
      session.pausedBufferBytes = 0;
      // Split huge backlogs so no single frame balloons memory downstream.
      for (let i = 0; i < joined.length; i += OUTPUT_FRAME_BYTES) {
        this.handleOutput(session, joined.slice(i, i + OUTPUT_FRAME_BYTES));
      }
    }
  }

  /** Kill every tracked session (e.g. on server shutdown). */
  killAll(): void {
    for (const id of [...this.sessions.keys()]) {
      this.kill(id);
    }
  }

  get size(): number {
    return this.sessions.size;
  }

  private defaultShell(): string {
    if (os.platform() === "win32") return "powershell.exe";
    return "/bin/bash";
  }
}

/** Singleton used across the terminal route. */
export const terminalPtyManager = new TerminalPtyManager();
