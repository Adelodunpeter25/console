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
import { existsSync } from "node:fs";
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
  /** Whether the shell subprocess has been started. */
  shellStarted: boolean;
  /** Fallback that starts the shell at the default size if no resize arrives. */
  shellStartTimer?: ReturnType<typeof setTimeout>;
  /** Input received before the shell started, flushed on start. */
  pendingInput: string[];
  resolveStart?: (event: TerminalSpawnedEvent) => void;
  rejectStart?: (cause: unknown) => void;
}

/** How long to wait for the client's first resize before starting the shell
    at the default size. Real clients resize within ~100-300ms of connecting;
    waiting for it means zsh draws its first prompt at the true grid width and
    its partial-line erase dance self-cleans instead of leaving standout `%`
    artifacts in scrollback. */
const SHELL_START_FALLBACK_MS = 500;

export class TerminalPtyManager {
  private sessions = new Map<TerminalId, PtySession>();

  /**
   * Spawn a new shell PTY in the given working directory.
   * Throws if `cwd` does not exist so clients get a clear spawn error.
   *
   * The PTY is created immediately at the default size, but the shell itself
   * starts on the client's first resize (or after SHELL_START_FALLBACK_MS) so
   * the first prompt is drawn at the client's true grid width.
   */
  spawn(params: TerminalSpawnParams): {
    id: TerminalId;
    ready: Promise<TerminalSpawnedEvent>;
  } {
    const cwd = path.resolve(params.cwd);
    if (!existsSync(cwd)) {
      throw new Error(`Cannot spawn terminal: working directory does not exist: ${cwd}`);
    }

    const shell = params.shell || process.env.SHELL || this.defaultShell();
    const cols = params.cols ?? 80;
    const rows = params.rows ?? 24;

    const id: TerminalId = randomUUID();
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
      shellStarted: false,
      pendingInput: [],
    };
    this.sessions.set(id, session);

    session.terminal = new Bun.Terminal({
      name: "xterm-256color",
      cols,
      rows,
      // Buffer output that arrives before the WebSocket route has attached
      // callbacks so the initial shell prompt is never dropped.
      data: (_terminal, data) => {
        this.handleOutput(session, new TextDecoder().decode(data));
      },
    });

    const ready = new Promise<TerminalSpawnedEvent>((resolve, reject) => {
      session.resolveStart = resolve;
      session.rejectStart = reject;
    });
    session.shellStartTimer = setTimeout(() => {
      if (!session.shellStarted && !session.killed) this.startShell(session);
    }, SHELL_START_FALLBACK_MS);

    return { id, ready };
  }

  /** Start the shell subprocess and resolve the spawn promise. */
  private startShell(session: PtySession): void {
    if (session.shellStarted || session.killed) return;
    session.shellStarted = true;
    if (session.shellStartTimer) {
      clearTimeout(session.shellStartTimer);
      session.shellStartTimer = undefined;
    }

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
      const queued = [...session.pendingInput];
      session.pendingInput.length = 0;
      for (const data of queued) {
        try {
          session.terminal.write(data);
        } catch {
          // Terminal closed mid-flush — nothing more to do.
        }
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

  /** Route PTY output to callbacks, honoring attach buffering and pause state. */
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
    if (session.callbacks) {
      session.callbacks.onData({ type: "output", data });
    } else {
      session.pending.push(data);
      // Cap buffered early output to avoid unbounded growth if attach never happens
      if (session.pending.length > 100) {
        session.pending.shift();
      }
    }
  }

  /** Attach a WebSocket-backed callback set to an existing session. */
  attach(id: TerminalId, callbacks: PtyCallbacks): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`No terminal session found for id: ${id}`);
    session.callbacks = callbacks;
    // Flush any output that arrived between spawn and attach (e.g. shell prompt)
    if (session.pending.length > 0) {
      const queued = [...session.pending];
      session.pending.length = 0;
      for (const data of queued) {
        callbacks.onData({ type: "output", data });
      }
    }
  }

  /** Write raw bytes into the PTY (keystrokes, pasted text). */
  write(id: TerminalId, data: string): boolean {
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
    const session = this.sessions.get(id);
    if (!session || session.killed) return false;
    try {
      session.terminal.resize(cols, rows);
      session.cols = cols;
      session.rows = rows;
      // The first resize carries the client's true grid size: start the shell
      // now so its first prompt is drawn at the correct width.
      if (!session.shellStarted) this.startShell(session);
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
    if (session.shellStartTimer) {
      clearTimeout(session.shellStartTimer);
      session.shellStartTimer = undefined;
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
      const buffered = session.pausedBuffer;
      session.pausedBuffer = [];
      session.pausedBufferBytes = 0;
      for (const data of buffered) {
        this.handleOutput(session, data);
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
