/**
 * Terminal protocol types shared across server, desktop (Rust relay), and mobile.
 *
 * The terminal runs **on the Node server** via node-pty. Clients never spawn
 * shells directly — the desktop/mobile app connects to the server over
 * WebSocket at `GET /api/terminals?cwd=...&cols=...&rows=...` (query params
 * are the spawn options; all subsequent traffic is JSON frames).
 */

/** Unique id for a running PTY session on the server. */
export type TerminalId = string;

/** Query params used to spawn a new PTY when the WebSocket connects. */
export interface TerminalSpawnParams {
  /** Working directory the shell starts in (usually a project root). */
  cwd: string;
  /** Shell binary. Defaults to process.env.SHELL or the platform shell. */
  shell?: string;
  /** Initial columns. Default 80. */
  cols?: number;
  /** Initial rows. Default 24. */
  rows?: number;
  /** Optional friendly label (e.g. the project name) for tooling/debugging. */
  label?: string;
}

/** Confirmation sent from the server once the PTY has spawned. */
export interface TerminalSpawnedEvent {
  type: "spawned";
  id: TerminalId;
  pid: number;
  shell: string;
  cwd: string;
  cols: number;
  rows: number;
}

/** PTY output pushed from the server to the client. */
export interface TerminalOutputEvent {
  type: "output";
  data: string;
}

/** PTY exited (shell process terminated). */
export interface TerminalExitEvent {
  type: "exit";
  code: number | null;
}

/** Terminal-level failure (spawn error, kill error, unknown frame). */
export interface TerminalErrorEvent {
  type: "error";
  message: string;
}

/** Union of all messages the server may send to the client. */
export type TerminalServerMessage =
  | TerminalSpawnedEvent
  | TerminalOutputEvent
  | TerminalExitEvent
  | TerminalErrorEvent;

/** Client → server: keystrokes / pasted input to feed into the PTY. */
export interface TerminalInputMessage {
  type: "input";
  data: string;
}

/** Client → server: terminal viewport resize. */
export interface TerminalResizeMessage {
  type: "resize";
  cols: number;
  rows: number;
}

/** Client → server: kill/close the PTY. */
export interface TerminalKillMessage {
  type: "kill";
}

/** Union of all messages the client may send to the server. */
export type TerminalClientMessage =
  | TerminalInputMessage
  | TerminalResizeMessage
  | TerminalKillMessage;