/**
 * CLI Types - Type definitions for daemon management
 */

export interface DaemonStatus {
  running: boolean;
  pid?: number;
  uptime?: string;
  port?: string;
  host?: string;
  /** Storage mode of the running daemon (`~/.console` vs `~/.console-dev`). */
  mode?: "dev" | "production";
}

export interface DaemonConfig {
  port: string;
  host: string;
  logLevel: string;
}

export interface StartOptions {
  port?: string;
  host?: string;
  daemon: boolean;
  /** Use dev storage (~/.console-dev) instead of production (~/.console). */
  dev?: boolean;
}

export interface RestartOptions {
  port?: string;
  host?: string;
  /** Use dev storage (~/.console-dev) instead of production (~/.console). */
  dev?: boolean;
}

export interface LogsOptions {
  follow: boolean;
  lines: string;
}
