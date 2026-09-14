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
function isCompiledBinary(): boolean {
  const execBase = path.basename(process.execPath);
  return !["bun", "bun-debug", "node"].includes(execBase);
}

export type ConsoleMode = "dev" | "production";

/**
 * Explicit storage mode. CONSOLE_ENV wins authoritatively; NODE_ENV only
 * counts for non-compiled runtimes — compiled Bun binaries default NODE_ENV
 * to "development", so trusting it there sends production installs to dev
 * storage. Mirrors server apppaths.resolveConsoleMode — keep in sync.
 */
export function resolveConsoleMode(env: NodeJS.ProcessEnv = process.env): ConsoleMode {
  if (env.CONSOLE_ENV === "dev") return "dev";
  if (env.CONSOLE_ENV === "production") return "production";
  if (!isCompiledBinary() && env.NODE_ENV === "development") return "dev";
  return "production";
}

export function getConsoleDir(): string {
  return path.join(os.homedir(), resolveConsoleMode() === "dev" ? ".console-dev" : ".console");
}

export function getLogsDir(): string {
  return path.join(getConsoleDir(), "logs");
}

function pidFilePath(dir: string = getConsoleDir()): string {
  return path.join(dir, "daemon.pid");
}

function configFilePath(dir: string = getConsoleDir()): string {
  return path.join(dir, "config.json");
}

function alternateConsoleDir(): string {
  return path.join(os.homedir(), resolveConsoleMode() === "dev" ? ".console" : ".console-dev");
}

/**
 * Per-machine secret env file (~/.console/env, ~/.console-dev/env in dev).
 * KEY=VALUE lines (see parseEnvFile) loaded into the daemon environment on
 * `console start`, so provider keys (e.g. FIRECRAWL_API_KEY) work for every
 * current and future daemon without shell exports. Explicit process env
 * always wins over the file. Managed by `console env`, mode 0600.
 */
export function getEnvFilePath(): string {
  return path.join(getConsoleDir(), "env");
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
    await fs.mkdir(getConsoleDir(), { recursive: true });
    await fs.mkdir(getLogsDir(), { recursive: true });
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
  await fs.writeFile(configFilePath(), JSON.stringify(mergedConfig, null, 2));
}

/**
 * Load daemon config
 */
export async function loadConfig(dir: string = getConsoleDir()): Promise<DaemonConfig> {
  try {
    if (existsSync(configFilePath(dir))) {
      const content = await fs.readFile(configFilePath(dir), "utf-8");
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
  await fs.writeFile(pidFilePath(), pid.toString());
}

/**
 * Read PID file
 */
export async function readPidFile(dir: string = getConsoleDir()): Promise<number | null> {
  try {
    if (existsSync(pidFilePath(dir))) {
      const content = await fs.readFile(pidFilePath(dir), "utf-8");
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
export async function removePidFile(dir: string = getConsoleDir()): Promise<void> {
  try {
    if (existsSync(pidFilePath(dir))) {
      await fs.unlink(pidFilePath(dir));
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
 * Get daemon status. When no pid exists in the resolved dir, falls back to
 * the alternate dir so `stop`/`status` keep working regardless of which mode
 * the running daemon was started with.
 */
export async function getDaemonStatus(): Promise<DaemonStatus> {
  for (const dir of [getConsoleDir(), alternateConsoleDir()]) {
    const pid = await readPidFile(dir);
    if (!pid) continue;
    if (!(await isProcessRunning(pid))) {
      // Stale pid here — clear just this dir and keep looking.
      await removePidFile(dir);
      continue;
    }
    return await buildRunningStatus(pid, dir);
  }
  return { running: false };
}

async function buildRunningStatus(pid: number, dir: string): Promise<DaemonStatus> {
  // Get process info (uptime)
  let uptime: string | undefined;
  try {
    const { stdout } = await execAsync(`ps -p ${pid} -o etime=`).catch(() => ({ stdout: "" }));
    uptime = stdout.trim() || undefined;
  } catch {
    // Process might have died between checks
    return { running: false };
  }

  const config = await loadConfig(dir);
  const mode: ConsoleMode = dir === getConsoleDir() ? resolveConsoleMode() : alternateMode();

  return {
    running: true,
    pid,
    uptime,
    port: config.port,
    host: config.host,
    mode,
  };
}

function alternateMode(): ConsoleMode {
  return resolveConsoleMode() === "dev" ? "production" : "dev";
}

/**
 * Remove the PID file holding `pid`, whichever storage dir it lives in.
 */
async function removePidFileFor(pid: number): Promise<void> {
  for (const dir of [getConsoleDir(), alternateConsoleDir()]) {
    try {
      if (existsSync(pidFilePath(dir))) {
        const content = await fs.readFile(pidFilePath(dir), "utf-8");
        if (parseInt(content.trim(), 10) === pid) {
          await fs.unlink(pidFilePath(dir));
        }
      }
    } catch (error) {
      console.warn(`Failed to remove PID file: ${error}`);
    }
  }
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
        await removePidFileFor(pid);
        return;
      }
    }

    // Force kill if still running
    process.kill(pid, "SIGKILL");
    await removePidFileFor(pid);
  } catch (error) {
    console.error(`Failed to kill daemon: ${error}`);
    throw error;
  }
}
