/**
 * Desktop-side terminal UI types.
 *
 * Note: protocol-level types shared with the server (TerminalSpawnParams,
 * TerminalServerMessage, TerminalClientMessage, etc.) live in `@console/types`;
 * these describe the *local* UI state of a terminal tab and its lifecycle.
 */

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