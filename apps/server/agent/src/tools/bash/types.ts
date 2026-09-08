export type BashJobStatus = "running" | "exited" | "failed" | "killed" | "expired";

export interface BashJobSnapshot {
  jobId: string;
  command: string;
  cwd: string;
  status: BashJobStatus;
  exitCode?: number;
  signal?: string;
  startedAt: string;
  finishedAt?: string;
  timedOut: boolean;
  aborted: boolean;
}

export interface StartJobOptions {
  command: string;
  cwd: string;
  env: NodeJS.ProcessEnv;
  timeoutMs: number;
  ownerSessionId?: string;
}

export interface JobOutputOptions {
  cursor?: number;
  limit?: number;
  ownerSessionId?: string;
}

export interface JobOutput {
  snapshot: BashJobSnapshot;
  stdout: string;
  stderr: string;
  nextCursor: number;
  truncated: boolean;
}

/** Internal record — only manager.ts touches fields beyond the snapshot. */
export interface JobRecord extends BashJobSnapshot {
  proc: Bun.Subprocess<any, any, any>;
  stdout: string;
  stderr: string;
  /** Total chars ever written (including dropped). */
  stdoutTotal: number;
  stderrTotal: number;
  /** Chars dropped from the head due to cap. */
  stdoutDropped: number;
  stderrDropped: number;
  stdoutDone: boolean;
  stderrDone: boolean;
  exited: boolean;
  rawExit?: number | null;
  timer?: ReturnType<typeof setTimeout>;
  escalation?: ReturnType<typeof setTimeout>;
  retentionTimer?: ReturnType<typeof setTimeout>;
  waiters: Array<() => void>;
  ownerSessionId: string;
}

export const BG_DEFAULT_TIMEOUT_MS = 10 * 60 * 1000;
export const BG_MAX_TIMEOUT_MS = 20 * 60 * 1000;
export const MAX_OUTPUT_CHARS_PER_STREAM = 256 * 1024;
export const MAX_RUNNING_JOBS = 10;
export const MAX_RETAINED_JOBS = 50;
export const RETENTION_MS = 30 * 60 * 1000;
export const MAX_WAIT_MS = 120 * 1000;
