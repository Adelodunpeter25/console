/**
 * CLI Types - Type definitions for daemon management
 */

export interface DaemonStatus {
  running: boolean;
  pid?: number;
  uptime?: string;
  port?: string;
  host?: string;
}

export interface DaemonConfig {
  port: string;
  host: string;
  logLevel: string;
}

export interface StartOptions {
  port: string;
  host: string;
  daemon: boolean;
}

export interface LogsOptions {
  follow: boolean;
  lines: string;
}