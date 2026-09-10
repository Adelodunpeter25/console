/**
 * Daemon Manager - Handles PID file management and process control
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as os from "node:os";
import { existsSync } from "node:fs";
import { exec } from "node:child_process";
import { promisify } from "node:util";
import type { DaemonStatus, DaemonConfig } from "./types.js";

const execAsync = promisify(exec);

// Directory structure
const isDev = process.env.NODE_ENV === "development" || process.env.CONSOLE_ENV === "dev";
const homeDir = os.homedir();
const folderName = isDev ? ".console-dev" : ".console";
export const CONSOLE_DIR = path.join(homeDir, folderName);
const PID_FILE = path.join(CONSOLE_DIR, "daemon.pid");
export const LOGS_DIR = path.join(CONSOLE_DIR, "logs");
const CONFIG_FILE = path.join(CONSOLE_DIR, "config.json");

/**
 * Per-machine secret env file (~/.console/env, ~/.console-dev/env in dev).
 * KEY=VALUE lines (see parseEnvFile) loaded into the daemon environment on
 * `console start`, so provider keys (e.g. FIRECRAWL_API_KEY) work for every
 * current and future daemon without shell exports. Explicit process env
 * always wins over the file. Managed by `console env`, mode 0600.
 */
export function getEnvFilePath(): string {
  return path.join(CONSOLE_DIR, "env");
}

export function parseEnvFile(contents: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const rawLine of contents.split("\n")) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const eq = line.indexOf("=");
    if (eq <= 0) continue;
    const key = line.slice(0, eq).trim();
    let value = line.slice(eq + 1).trim();
    if (
      value.length >= 2 &&
      ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'")))
    ) {
      value = value.slice(1, -1);
    }
    if (key) out[key] = value;
  }
  return out;
}

export async function loadEnvFile(): Promise<Record<string, string>> {
  try {
    return parseEnvFile(await fs.readFile(getEnvFilePath(), "utf-8"));
  } catch {
    return {};
  }
}

/**
 * Insert or replace entries, leaving every other line (comments, blank
 * lines, other keys) byte-identical. Creates the file at mode 0600.
 */
export async function upsertEnvValues(values: Record<string, string>): Promise<void> {
  await ensureConsoleDir();
  let raw = "";
  try {
    raw = await fs.readFile(getEnvFilePath(), "utf-8");
  } catch {
    raw = "";
  }
  const pending = { ...values };
  const out: string[] = [];
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    const eq = trimmed.indexOf("=");
    const key = eq > 0 ? trimmed.slice(0, eq).trim() : "";
    if (key && Object.hasOwn(pending, key)) {
      out.push(`${key}="${pending[key]}"`);
      delete pending[key];
    } else {
      out.push(line);
    }
  }
  for (const [key, value] of Object.entries(pending)) {
    out.push(`${key}="${value}"`);
  }
  const text = `${out.join("\n").replace(/\n+$/, "")}\n`;
  await fs.writeFile(getEnvFilePath(), text, { mode: 0o600 });
  await fs.chmod(getEnvFilePath(), 0o600);
}

/**
 * Ensure console directory structure exists
 */
export async function ensureConsoleDir(): Promise<void> {
  try {
    await fs.mkdir(CONSOLE_DIR, { recursive: true });
    await fs.mkdir(LOGS_DIR, { recursive: true });
  } catch (error) {
    throw new Error(`Failed to create console directory: ${error}`);
  }
}

/**
 * Get the default daemon config
 */
export function getDefaultConfig(): DaemonConfig {
  return {
    port: process.env.PORT || "3000",
    host: process.env.HOST || "0.0.0.0",
    logLevel: process.env.LOG_LEVEL || "info",
  };
}

/**
 * Resolve effective port/host: CLI flag > env (PORT/HOST) > saved config > default.
 * Pure (no I/O) so it can be unit-tested. `saved` is the result of loadConfig().
 */
export function resolvePortHost(
  overrides: { port?: string; host?: string },
  saved: DaemonConfig,
  env: NodeJS.ProcessEnv = process.env,
): { port: string; host: string } {
  return {
    port: overrides.port ?? env.PORT ?? saved.port ?? "3000",
    host: overrides.host ?? env.HOST ?? saved.host ?? "0.0.0.0",
  };
}

/**
 * Save daemon config
 */
export async function saveConfig(config: Partial<DaemonConfig>): Promise<void> {
  await ensureConsoleDir();
  const currentConfig = getDefaultConfig();
  const mergedConfig = { ...currentConfig, ...config };
  await fs.writeFile(CONFIG_FILE, JSON.stringify(mergedConfig, null, 2));
}

/**
 * Load daemon config
 */
export async function loadConfig(): Promise<DaemonConfig> {
  try {
    if (existsSync(CONFIG_FILE)) {
      const content = await fs.readFile(CONFIG_FILE, "utf-8");
      return JSON.parse(content) as DaemonConfig;
    }
  } catch (error) {
    console.warn(`Failed to load config, using defaults: ${error}`);
  }
  return getDefaultConfig();
}

/**
 * Write PID file
 */
export async function writePidFile(pid: number): Promise<void> {
  await ensureConsoleDir();
  await fs.writeFile(PID_FILE, pid.toString());
}

/**
 * Read PID file
 */
export async function readPidFile(): Promise<number | null> {
  try {
    if (existsSync(PID_FILE)) {
      const content = await fs.readFile(PID_FILE, "utf-8");
      return parseInt(content.trim(), 10);
    }
  } catch (error) {
    console.warn(`Failed to read PID file: ${error}`);
  }
  return null;
}

/**
 * Remove PID file
 */
export async function removePidFile(): Promise<void> {
  try {
    if (existsSync(PID_FILE)) {
      await fs.unlink(PID_FILE);
    }
  } catch (error) {
    console.warn(`Failed to remove PID file: ${error}`);
  }
}

/**
 * Check if a process is running by PID
 */
export async function isProcessRunning(pid: number): Promise<boolean> {
  try {
    // On Unix-like systems, use kill -0 to check if process exists
    process.kill(pid, 0);
    return true;
  } catch {
    // Process doesn't exist or we don't have permission
    return false;
  }
}

/**
 * Get daemon status
 */
export async function getDaemonStatus(): Promise<DaemonStatus> {
  const pid = await readPidFile();

  if (!pid) {
    return { running: false };
  }

  const running = await isProcessRunning(pid);

  if (!running) {
    // Clean up stale PID file
    await removePidFile();
    return { running: false };
  }

  // Get process info (uptime)
  let uptime: string | undefined;
  try {
    const { stdout } = await execAsync(`ps -p ${pid} -o etime=`).catch(() => ({ stdout: "" }));
    uptime = stdout.trim() || undefined;
  } catch {
    // Process might have died between checks
    return { running: false };
  }

  const config = await loadConfig();

  return {
    running: true,
    pid,
    uptime,
    port: config.port,
    host: config.host,
  };
}

/**
 * Kill daemon process
 */
export async function killDaemon(pid: number): Promise<void> {
  try {
    process.kill(pid, "SIGTERM");

    // Wait up to 5 seconds for graceful shutdown
    for (let i = 0; i < 50; i++) {
      await new Promise((resolve) => setTimeout(resolve, 100));
      if (!(await isProcessRunning(pid))) {
        await removePidFile();
        return;
      }
    }

    // Force kill if still running
    process.kill(pid, "SIGKILL");
    await removePidFile();
  } catch (error) {
    console.error(`Failed to kill daemon: ${error}`);
    throw error;
  }
}
