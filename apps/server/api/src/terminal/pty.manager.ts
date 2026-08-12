/**
 * Terminal PTY Manager.
 *
 * Owns the lifecycle of every node-pty session on the server. A PTY is
 * identified by a random `terminalId`; the WebSocket route creates instances
 * here and streams their output/exit events back to the client.
 *
 * All instances are tracked so a server shutdown (or daemon restart) can kill
 * lingering shells deterministically instead of leaking processes.
 */
import { randomUUID } from "node:crypto";
import { existsSync } from "node:fs";
import { chmodSync } from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import pty from "node-pty";
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

interface PtySession {
  id: TerminalId;
  pty: pty.IPty;
  shell: string;
  cwd: string;
  cols: number;
  rows: number;
  callbacks?: PtyCallbacks;
  killed: boolean;
}

export class TerminalPtyManager {
  private sessions = new Map<TerminalId, PtySession>();

  /**
   * Ensure node-pty's prebuilt native binaries are executable. npm can strip
   * exec bits on install (the bundled spawn-helper must be +x or posix_spawn
   * fails with "posix_spawnp failed").
   */
  private ensureExecutable(): void {
    try {
      for (const arch of [process.platform, `${process.platform}-${process.arch}`]) {
        const dir = path.resolve(
          path.dirname(require.resolve("node-pty/package.json")),
          "prebuilds",
          arch,
        );
        if (!existsSync(dir)) continue;
        for (const bin of ["spawn-helper", "pty.node"]) {
          const file = path.join(dir, bin);
          if (existsSync(file)) chmodSync(file, 0o755);
        }
      }
    } catch {
      // Best-effort: ignore failures and let the spawn error surface instead.
    }
  }

  /**
   * Spawn a new shell PTY in the given working directory.
   * Throws if `cwd` does not exist so clients get a clear spawn error.
   */
  spawn(params: TerminalSpawnParams): TerminalSpawnedEvent {
    this.ensureExecutable();
    const cwd = path.resolve(params.cwd);
    if (!existsSync(cwd)) {
      throw new Error(`Cannot spawn terminal: working directory does not exist: ${cwd}`);
    }

    const shell = params.shell || process.env.SHELL || this.defaultShell();
    const cols = params.cols ?? 80;
    const rows = params.rows ?? 24;

    const id: TerminalId = randomUUID();
    const instance = pty.spawn(shell, [], {
      name: "xterm-256color",
      cols,
      rows,
      cwd,
      env: {
        ...process.env,
        TERM: "xterm-256color",
        CONSOLE_TERMINAL: "true",
      } as Record<string, string>,
    });

    const session: PtySession = {
      id,
      pty: instance,
      shell,
      cwd,
      cols,
      rows,
      killed: false,
    };
    this.sessions.set(id, session);

    // Forward process events once the route has attached callbacks. Output
    // always begins flowing immediately; a session with no callbacks yet just
    // buffers nothing and drops early output (acceptable: the spawn event is
    // what matters; the shell prompt arrives after the client binds).
    instance.onData((data) => {
      session.callbacks?.onData({ type: "output", data });
    });
    instance.onExit(({ exitCode }) => {
      if (session.killed) return;
      session.killed = true;
      this.sessions.delete(id);
      session.callbacks?.onExit(exitCode);
    });

    return { type: "spawned", id, pid: instance.pid, shell, cwd, cols, rows };
  }

  /** Attach a WebSocket-backed callback set to an existing session. */
  attach(id: TerminalId, callbacks: PtyCallbacks): void {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`No terminal session found for id: ${id}`);
    session.callbacks = callbacks;
  }

  /** Write raw bytes into the PTY (keystrokes, pasted text). */
  write(id: TerminalId, data: string): boolean {
    const session = this.sessions.get(id);
    if (!session || session.killed) return false;
    try {
      session.pty.write(data);
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
      session.pty.resize(cols, rows);
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
    try {
      session.pty.kill();
    } catch {
      // Already dead — fine.
    }
    session.callbacks?.onExit(null);
  }

  /**
   * Pause a PTY's output. Used when the client socket's send buffer is
   * saturated so a flooding program (`yes`, huge file cats) can't balloon
   * memory; the child blocks on write until resume().
   */
  pause(id: TerminalId): void {
    const session = this.sessions.get(id);
    if (session && !session.killed) {
      try {
        session.pty.pause();
      } catch {
        // Already paused or dying — fine.
      }
    }
  }

  /** Resume a paused PTY's output (the send buffer drained). */
  resume(id: TerminalId): void {
    const session = this.sessions.get(id);
    if (session && !session.killed) {
      try {
        session.pty.resume();
      } catch {
        // Already resumed or dying — fine.
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